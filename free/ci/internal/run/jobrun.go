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
	"github.com/nself-org/plugins/free/ci/internal/trust"
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
	if err = rs.sess.Store.Transition(bg, id, from, to, store.TransitionDetail{Epoch: epoch, FailureClass: class}); err != nil {
		return err
	}
	if to == "passed" || to == "failed" || to == "cancelled" || to == "lost" { // the store freed the host reservation
		dispatchWake(rs.sess.RunID)
	}
	return nil
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

func (rs *runState) info(pj plan.PlannedJob, job model.Job, attemptID string, n int) AttemptInfo {
	source := rs.source
	if cap := rs.c.Opts.SecretsTrust; cap != "" && trust.RankTrust(cap) < trust.RankTrust(source) {
		source = cap
	}
	var pipelineCache *model.CacheConfig
	if rs.c.Plan != nil {
		pipelineCache = rs.c.Plan.Cache
	}
	return AttemptInfo{SourceClass: source, RunID: rs.sess.RunID, JobID: rs.rows[pj.ID], JobName: pj.ID, AttemptID: attemptID, Revision: rs.rev.SHA, N: n,
		Job: job, IdempotencyKey: idempotencyKey(&job, attemptID), Home: rs.sess.Home, Store: rs.sess.Store, Container: rs.c.Opts.Container, PipelineCache: pipelineCache,
		SecretsInfo: SecretsInfo{Root: rs.c.Opts.Root, CoordinatorID: rs.sess.Coordinator.ID, SecretClasses: rs.pol.SecretClasses, HasSecretClasses: rs.pol.HasSecretClasses}}
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

// runAttemptN creates, admits and runs attempt n of one job (retry.go decides whether there is a next).
func (rs *runState) runAttemptN(ctx context.Context, i int, pj plan.PlannedJob, o JobOutcome, n int) JobOutcome {
	jctx, stop := context.WithCancel(ctx)
	defer stop()
	rs.setCancel(pj.ID, stop)
	if m := rs.marked(pj.ID); m != "" {
		rs.retire(AttemptSpec{Pipeline: rs.mp, Job: &rs.mp.Jobs[i], N: n}) // a parked previous attempt is superseded
		return rs.markOutcome(o, m)
	}
	job := &rs.mp.Jobs[i]
	spec := AttemptSpec{Pipeline: rs.mp, Job: job, N: n, Foreground: true}
	if aid, e := AttemptIDFor(WithSession(jctx, rs.sess), spec); e == nil { // a cancel intent finds the attempt while it waits for admission
		rs.mu.Lock()
		rs.attempts[aid] = pj.ID
		rs.mu.Unlock()
	}
	adm, err := CreateAttempt(WithSession(jctx, rs.sess), spec)
	rs.retire(spec) // a parked previous attempt is superseded whether or not this one was admitted
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
	o.AttemptID, o.N = adm.AttemptID, n
	rs.mu.Lock()
	rs.attempts[adm.AttemptID] = pj.ID
	rs.mu.Unlock()
	if !adm.Local {
		return rs.runRemote(jctx, pj, *job, adm, o, n) // a node the dispatcher delivered the lease to (P7-AUTO-28)
	}
	return rs.runLeased(jctx, pj, *job, adm.AttemptID, o, n)
}

func (rs *runState) runLeased(ctx context.Context, pj plan.PlannedJob, job model.Job, id string, o JobOutcome, n int) JobOutcome {
	if err := rs.trans(id, "leased", "running", ""); err != nil {
		return rs.endEarly(ctx, pj, id, o, err)
	}
	info := rs.info(pj, job, id, n)
	_, epoch, err := rs.sess.Store.AttemptStateEpoch(context.WithoutCancel(ctx), id)
	if err != nil && declares(job) { // only a job with fenced writes needs its launch epoch
		return rs.failPrepared(ctx, pj, job, id, o, err, n, info, false)
	}
	info.Epoch = epoch // the capability this incarnation of the attempt launched with
	setup, err := rs.prepare(ctx, &info)
	defer rs.unprepare(info, setup)
	if err != nil {
		return rs.failPrepared(ctx, pj, job, id, o, err, n, info, setup.cacheStarted)
	}
	logRef := rs.logRef(info)
	spec := rs.spec(ctx, pj, setup.job, id)
	spec.Mounts = setup.mounts
	effective := info
	effective.Job = setup.job // hooks see the isolation the job really runs under
	spec.BeforeJob = rs.beforeJob(effective)
	spec.AfterJob = rs.afterJob(effective)
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
	if newSecrets := rs.hooks.secretsFactory(); newSecrets != nil && len(spec.Job.Secrets) > 0 {
		spec.Secrets = newSecrets(info, rs.hooks.NewRedactor)
		if z, ok := spec.Secrets.(interface{ Zero() }); ok {
			defer z.Zero() // values are zeroed on every exit: pass, fail, cancel and a lost attempt
		}
	}
	res := func() ciexec.AttemptResult {
		defer closeLog(lg)              // before the attempt settles: a finished attempt always has its whole log stored
		return rs.runWaiting(ctx, spec) // takes the execution slot and waits out an unavailable isolation level
	}()
	o.Duration, o.ExitCode, o.Tail = res.Finished.Sub(res.Started), res.ExitCode, t.String()
	if lg != nil {
		o.LogRef = logRef
	}
	rs.judge(ctx, info, full, &res, &o)
	rs.diagnose(&o, &res)
	classifyCatalogue(pj.Check, &res, &o)
	oc := Outcome{State: res.Attempt.State, FailureClass: res.Attempt.FailureClass, ExitCode: res.ExitCode}
	defer func() {
		if rs.hooks.Cache != nil {
			bg, done := background(ctx)
			defer done()
			_ = guard("cache after hook", func() error { return rs.hooks.Cache.After(bg, info, oc) })
		}
	}()
	switch res.Attempt.State {
	case "cancelled":
		if _, err := rs.settle(id, "running", "cancelled", "cancelled"); err != nil {
			return failed(o, err)
		}
		return rs.stopped(o, pj.ID)
	case "passed":
		return rs.pass(id, o, info)
	}
	o.FailureClass = res.Attempt.FailureClass
	if res.Err != nil {
		o.Code, o.Detail = CodeOf(res.Err), res.Err.Error()
	}
	if res.Reason != "" {
		o.Detail = strings.TrimSpace(o.Detail + " (" + res.Reason + ": the container may have hit its memory or process limit)")
	}
	return rs.settleFailed(ctx, pj, job, id, o, n)
}

func (rs *runState) pass(id string, o JobOutcome, info AttemptInfo) JobOutcome {
	mid, err := rs.settle(id, "running", "finalizing", "")
	if err != nil {
		return failed(o, err)
	}
	if mid == "cancelled" { // a cancel intent won the move to finalizing
		return cancelled(o, "", "cancelled by the operator")
	}
	return rs.finalizePassed(id, o, info)
}

func (rs *runState) settleFailed(ctx context.Context, pj plan.PlannedJob, job model.Job, id string, o JobOutcome, n int) JobOutcome {
	class := o.FailureClass
	if class == "" {
		class = "code"
	}
	if rs.park(ctx, pj, job, id, &o, n) {
		return o
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
func (rs *runState) failPrepared(ctx context.Context, pj plan.PlannedJob, job model.Job, id string, o JobOutcome, err error, n int, info AttemptInfo, cacheStarted bool) JobOutcome {
	o = failed(o, err)
	if cacheStarted {
		bg, done := background(ctx)
		defer done()
		_ = guard("cache after hook", func() error { return rs.hooks.Cache.After(bg, info, Outcome{State: "failed", FailureClass: "infra"}) })
	}
	if ctx.Err() != nil {
		_, _ = rs.settle(id, "running", "cancelled", "cancelled")
		return rs.stopped(o, pj.ID)
	}
	rs.diagnosePrepare(&o, err)
	if rs.park(ctx, pj, job, id, &o, n) {
		return o
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
