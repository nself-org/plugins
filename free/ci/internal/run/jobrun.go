package run

// Purpose: one attempt from admission to its final state: lease, start hooks,
// cache and services, the executor, and the state transitions with their
// epochs. A cancel intent written by `ci runs cancel` always wins over a
// result that was not committed first (store CAS).
// Inputs:  a planned job and its outcome shell. Outputs: the filled outcome.
// Constraints: only CreateAttempt makes attempts; every transition reads the
// epoch it commits against; cleanup writes use a context that survives Ctrl-C.

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const tailBytes = 8 << 10

// tail keeps the last bytes of a job's output.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) write(b []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > tailBytes {
		t.buf = t.buf[len(t.buf)-tailBytes:]
	}
}

func (t *tail) String() string { t.mu.Lock(); defer t.mu.Unlock(); return string(t.buf) }

// trans moves an attempt from one state to the next at its current epoch.
func (rs *runState) trans(id string, from, to model.JobState, class model.FailureClass) error {
	bg, done := background(context.Background())
	defer done()
	state, epoch, err := rs.sess.Store.AttemptStateEpoch(bg, id)
	if err != nil {
		return err
	}
	if state != from {
		return fail(CodeRun, "attempt %s is %s, not %s", id, state, from)
	}
	return rs.sess.Store.Transition(bg, id, from, to, store.TransitionDetail{Epoch: epoch, FailureClass: class})
}

// settle ends an attempt that is in state `from`. A committed cancel intent
// turns any non-cancel result into cancelled (E605 from the store).
func (rs *runState) settle(id string, from, to model.JobState, class model.FailureClass) (model.JobState, error) {
	err := rs.trans(id, from, to, class)
	if err != nil && CodeOf(err) == codeStoreRef && to != "cancelled" {
		if err = rs.trans(id, from, "cancelled", "cancelled"); err == nil {
			return "cancelled", nil
		}
	}
	return to, err
}

func (rs *runState) info(pj plan.PlannedJob, job model.Job, attemptID string) AttemptInfo {
	return AttemptInfo{RunID: rs.sess.RunID, JobID: rs.rows[pj.ID], JobName: pj.ID, AttemptID: attemptID, Revision: rs.rev.SHA, N: 1,
		Job: job, IdempotencyKey: idempotencyKey(&job, attemptID)}
}

// failed fills a failure outcome from an error.
func failed(o JobOutcome, err error) JobOutcome {
	o.State, o.Code, o.Detail, o.FailureClass = StateFailed, CodeOf(err), err.Error(), "infra"
	return o
}

// stopped is the outcome of a job whose context ended.
func (rs *runState) stopped(o JobOutcome, id string) JobOutcome {
	if m := rs.marked(id); m != "" {
		return rs.markOutcome(o, m)
	}
	return cancelled(o, "", "cancelled by the operator")
}

func (rs *runState) runAttempt(ctx context.Context, i int, pj plan.PlannedJob, o JobOutcome) JobOutcome {
	jctx, stop := context.WithCancel(ctx)
	defer stop()
	rs.setCancel(pj.ID, stop)
	if m := rs.marked(pj.ID); m != "" {
		return rs.markOutcome(o, m)
	}
	job := &rs.mp.Jobs[i]
	adm, err := CreateAttempt(WithSession(jctx, rs.sess), AttemptSpec{Pipeline: rs.mp, Job: job, N: 1, Foreground: true})
	if err != nil {
		var ar *AdmissionRefused
		switch {
		case jctx.Err() != nil:
			return rs.stopped(o, pj.ID)
		case errors.As(err, &ar) && !o.Required:
			return skipped(o, ar.Reason, "optional job refused by local admission")
		case errors.As(err, &ar):
			o.State, o.Reason, o.Detail, o.FailureClass = StateFailed, ar.Reason, err.Error(), "infra"
			return o
		}
		return failed(o, err)
	}
	o.AttemptID, o.N = adm.AttemptID, 1
	rs.mu.Lock()
	rs.attempts[adm.AttemptID] = pj.ID
	rs.mu.Unlock()
	if !adm.Local {
		return failed(o, fail(CodeOptions, "attempt %s was placed on a remote runner; this coordinator cannot await it", adm.AttemptID))
	}
	return rs.runLeased(jctx, pj, *job, adm.AttemptID, o)
}

func (rs *runState) runLeased(ctx context.Context, pj plan.PlannedJob, job model.Job, id string, o JobOutcome) JobOutcome {
	if err := rs.trans(id, "leased", "running", ""); err != nil {
		return rs.endEarly(ctx, pj, id, o, err)
	}
	info := rs.info(pj, job, id)
	setup, err := rs.prepare(ctx, &info)
	defer rs.unprepare(info, setup)
	if err != nil {
		return rs.failPrepared(ctx, pj, id, o, err)
	}
	spec := rs.spec(ctx, pj, setup.job, id)
	var t tail
	spec.Output = func(b []byte) {
		t.write(b)
		if rs.c.Opts.Stream {
			rs.render.Event(Event{Type: "log", Job: pj.ID, Text: string(b)})
		}
	}
	rs.render.Event(Event{Type: "started", Job: pj.ID})
	res := rs.c.exec.Run(ctx, spec)
	o.Duration, o.ExitCode, o.Tail = res.Finished.Sub(res.Started), res.ExitCode, t.String()
	oc := Outcome{State: res.Attempt.State, FailureClass: res.Attempt.FailureClass, ExitCode: res.ExitCode}
	defer func() {
		if rs.hooks.Cache != nil {
			bg, done := background(ctx)
			defer done()
			_ = rs.hooks.Cache.After(bg, info, oc)
		}
	}()
	switch {
	case res.Attempt.State == "cancelled":
		if _, err := rs.settle(id, "running", "cancelled", "cancelled"); err != nil {
			return failed(o, err)
		}
		return rs.stopped(o, pj.ID)
	case res.Attempt.State == "passed":
		return rs.pass(id, o)
	}
	o.FailureClass = res.Attempt.FailureClass
	if res.Err != nil {
		o.Code, o.Detail = CodeOf(res.Err), res.Err.Error()
	}
	return rs.settleFailed(id, o)
}

func (rs *runState) pass(id string, o JobOutcome) JobOutcome {
	if _, err := rs.settle(id, "running", "finalizing", ""); err != nil {
		return failed(o, err)
	}
	to, err := rs.settle(id, "finalizing", "passed", "")
	if err != nil {
		return failed(o, err)
	}
	bg, done := background(context.Background())
	defer done()
	if state, _, e := rs.sess.Store.AttemptStateEpoch(bg, id); e == nil {
		to = state
	}
	switch to {
	case "passed":
		o.State = StatePassed
	case "cancelled":
		return cancelled(o, "", "cancelled by the operator")
	default:
		o.State, o.FailureClass, o.Detail = StateFailed, "infra", "a declared artifact was not committed"
	}
	return o
}

func (rs *runState) settleFailed(id string, o JobOutcome) JobOutcome {
	class := o.FailureClass
	if class == "" {
		class = "code"
	}
	to, err := rs.settle(id, "running", "failed", class)
	if err != nil {
		return failed(o, err)
	}
	if to == "cancelled" {
		return cancelled(o, "", "cancelled by the operator")
	}
	o.State, o.FailureClass = StateFailed, class
	return o
}

// failPrepared ends a running attempt whose preparation failed.
func (rs *runState) failPrepared(ctx context.Context, pj plan.PlannedJob, id string, o JobOutcome, err error) JobOutcome {
	o = failed(o, err)
	if ctx.Err() != nil {
		_, _ = rs.settle(id, "running", "cancelled", "cancelled")
		return rs.stopped(o, pj.ID)
	}
	_, _ = rs.settle(id, "running", "failed", "infra")
	return o
}

// endEarly handles a failed leased -> running move: usually a cancel intent.
func (rs *runState) endEarly(ctx context.Context, pj plan.PlannedJob, id string, o JobOutcome, err error) JobOutcome {
	if CodeOf(err) == codeStoreRef || ctx.Err() != nil {
		if _, e := rs.settle(id, "leased", "cancelled", "cancelled"); e == nil {
			return rs.stopped(o, pj.ID)
		}
	}
	return failed(o, err)
}

type prepared struct {
	job      model.Job
	services *ServicesRequest
}

// prepare runs the start hooks, restores the cache and starts services.
func (rs *runState) prepare(ctx context.Context, info *AttemptInfo) (prepared, error) {
	p := prepared{job: info.Job}
	env := map[string]string{}
	for k, v := range info.Job.Env {
		env[k] = v
	}
	for _, h := range rs.hooks.AttemptStarts {
		if err := h.AttemptStart(ctx, *info); err != nil {
			return p, err
		}
	}
	if rs.hooks.Cache != nil {
		setup, err := rs.hooks.Cache.Before(ctx, *info)
		if err != nil {
			return p, err
		}
		for k, v := range setup.Env {
			env[k] = v
		}
		if len(setup.Mounts) > 0 {
			b, _ := json.Marshal(setup.Mounts)
			env["NSELF_CI_MOUNTS"] = string(b)
		}
	}
	if names := append(append([]string(nil), info.Job.NeedsServices...), rs.c.Opts.Services...); len(names) > 0 {
		if rs.hooks.Services == nil {
			return p, fail(CodeOptions, "job needs services %v and no services hook is registered", names)
		}
		req := ServicesRequest{RunID: info.RunID, Names: names}
		up, err := rs.hooks.Services.Up(ctx, req)
		if err != nil {
			return p, err
		}
		p.services = &req
		for k, v := range up.Env {
			env[k] = v
		}
	}
	if info.IdempotencyKey != "" {
		env["NSELF_CI_IDEMPOTENCY_KEY"] = info.IdempotencyKey
	}
	env["NSELF_CI_RUN_ID"] = info.RunID
	p.job.Env = env
	if rs.c.Opts.Container {
		img := ""
		if p.job.Container != nil {
			img = p.job.Container.Image
		}
		if img == "" && rs.hooks.Image != nil {
			var err error
			if img, err = rs.hooks.Image.Image(ctx, *info); err != nil {
				return p, err
			}
		}
		if img == "" {
			return p, fail(CodeOptions, "--container needs a pinned toolchain image and none is registered")
		}
		p.job.Isolation, p.job.Container = "container", &model.ContainerConfig{Image: img}
	}
	return p, nil
}

func (rs *runState) unprepare(info AttemptInfo, p prepared) {
	if p.services != nil && rs.hooks.Services != nil {
		bg, done := background(context.Background())
		defer done()
		_ = rs.hooks.Services.Down(bg, *p.services)
	}
}

func (rs *runState) spec(ctx context.Context, pj plan.PlannedJob, job model.Job, id string) ciexec.JobSpec {
	s := ciexec.JobSpec{AttemptID: id, CoordinatorID: safeID(rs.sess.Coordinator.ID), Job: job, Command: job.Run,
		SourceDir: rs.c.Opts.Root, Home: rs.sess.Home, Network: model.NetworkScope(rs.c.Opts.Network)}
	if rs.c.Opts.Timeout > 0 {
		s.Timeout = rs.c.Opts.Timeout
	}
	return s
}
