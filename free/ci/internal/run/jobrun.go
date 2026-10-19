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
	"errors"
	"strings"
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
			rs.mu.Lock() // the intent won before the poller saw it: the run is still the operator's cancel
			rs.userCancel = true
			rs.mu.Unlock()
			return "cancelled", nil
		}
	}
	return to, err
}

func (rs *runState) info(pj plan.PlannedJob, job model.Job, attemptID string) AttemptInfo {
	return AttemptInfo{RunID: rs.sess.RunID, JobID: rs.rows[pj.ID], JobName: pj.ID, AttemptID: attemptID, Revision: rs.rev.SHA, N: 1,
		Job: job, IdempotencyKey: idempotencyKey(&job, attemptID), Home: rs.sess.Home, Store: rs.sess.Store}
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
	spec := AttemptSpec{Pipeline: rs.mp, Job: job, N: 1, Foreground: true}
	if aid, e := AttemptIDFor(WithSession(jctx, rs.sess), spec); e == nil { // a cancel intent finds the attempt while it waits for admission
		rs.mu.Lock()
		rs.attempts[aid] = pj.ID
		rs.mu.Unlock()
	}
	adm, err := CreateAttempt(WithSession(jctx, rs.sess), spec)
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
		_, _ = rs.settle(adm.AttemptID, "leased", "cancelled", "cancelled") // never leave a leased attempt a remote runner may still start
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
	spec.Mounts = setup.mounts
	spec.AfterJob = rs.afterJob(info)
	var t tail
	var full *outputBuffer // everything the attempt prints, kept only for jobs an OutcomeHook asks about
	if rs.wantsOutput(setup.job) {
		full = &outputBuffer{}
	}
	lg := rs.openLog(ctx, info)
	spec.Output = func(b []byte) {
		t.write(b)
		writeLog(lg, b)
		if full != nil {
			full.write(b)
		}
		if rs.c.Opts.Stream {
			rs.render.Event(Event{Type: "log", Job: pj.ID, Text: string(b)})
		}
	}
	rs.render.Event(Event{Type: "started", Job: pj.ID})
	spec.Slot = rs.sess.slots.take(slotDir(rs.sess.Home))
	res := func() ciexec.AttemptResult {
		defer rs.sess.slots.put(spec.Slot)
		defer closeLog(lg) // before the attempt settles: a finished attempt always has its whole log stored
		return rs.c.exec.Run(ctx, spec)
	}()
	o.Duration, o.ExitCode, o.Tail = res.Finished.Sub(res.Started), res.ExitCode, t.String()
	rs.judge(ctx, info, full, &res, &o)
	oc := Outcome{State: res.Attempt.State, FailureClass: res.Attempt.FailureClass, ExitCode: res.ExitCode}
	defer func() {
		if rs.hooks.Cache != nil {
			bg, done := background(ctx)
			defer done()
			_ = guard("cache after hook", func() error { return rs.hooks.Cache.After(bg, info, oc) })
		}
	}()
	switch {
	case res.Attempt.State == "cancelled":
		if _, err := rs.settle(id, "running", "cancelled", "cancelled"); err != nil {
			return failed(o, err)
		}
		return rs.stopped(o, pj.ID)
	case res.Attempt.State == "passed":
		return rs.pass(id, o, info)
	}
	o.FailureClass = res.Attempt.FailureClass
	if res.Err != nil {
		o.Code, o.Detail = CodeOf(res.Err), res.Err.Error()
	}
	if res.Reason != "" {
		o.Detail = strings.TrimSpace(o.Detail + " (" + res.Reason + ": the container may have hit its memory or process limit)")
	}
	return rs.settleFailed(id, o)
}

func (rs *runState) pass(id string, o JobOutcome, info AttemptInfo) JobOutcome {
	mid, err := rs.settle(id, "running", "finalizing", "")
	if err != nil {
		return failed(o, err)
	}
	if mid == "cancelled" { // a cancel intent won the move to finalizing
		return cancelled(o, "", "cancelled by the operator")
	}
	if ferr := rs.attemptFinalize(info); ferr != nil { // a hook refused: the attempt cannot pass (results before artifacts)
		return rs.settleFinalizeFailed(id, o, ferr)
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
