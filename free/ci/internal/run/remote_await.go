package run

// Purpose: the run loop's await of an attempt that was placed on a node it did not
// start (P7-AUTO-28, D-1041). The dispatcher delivered the lease; the node's
// carrier reports to the run's remoteSink. This file waits for that report,
// ends the attempt in the same states the local path uses (CI-37), and cancels
// the remote job when the run is cancelled.
// Inputs:  an admitted remote attempt (Admitted.Local false).
// Outputs: the filled JobOutcome; the attempt passed, failed, cancelled, or lost
// and re-queued for the CI-40 infra retry (park).
// Constraints: no polling; the wait ends on the carrier's result, the carrier's
// end without one (lost) or the context. A cancel moves the attempt to cancelled
// first (the lease row is gone, so a late result is fenced), then stops the
// carrier and waits for it, so the node's job process group is killed before
// the run exits. An infra failure reported by a node is not retried (it is
// finalizing, which has no path back to the queue); a lost session is.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/diagnose"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// remoteCancelWait bounds how long a cancelled run waits for the carrier to return.
const remoteCancelWait = 15 * time.Second

// runRemote awaits the remote attempt adm and settles it.
func (rs *runState) runRemote(ctx context.Context, pj plan.PlannedJob, job model.Job, adm Admitted, o JobOutcome, n int) JobOutcome {
	id := adm.AttemptID
	var ra *remoteAttempt
	var rd *runDispatch
	if v, ok := runDispatchers.Load(rs.sess.RunID); ok {
		rd = v.(*runDispatch)
		ra = rd.sink.attempt(id)
	}
	if ra == nil {
		_, _ = rs.cancelAttempt(id)
		return failed(o, fail(CodeOptions, "attempt %s was placed on a remote runner; this run has no way to await it", id))
	}
	info := rs.info(pj, job, id, n)
	lg := rs.openLog(ctx, info)
	defer closeLog(lg)
	var t tail
	ra.attach(func(b []byte) {
		t.write(b)
		writeLog(lg, b)
		if rs.c.Opts.Stream {
			rs.render.Event(Event{Type: "log", Job: pj.ID, Text: string(b)})
		}
	})
	rs.render.Event(Event{Type: "started", Job: pj.ID})
	if lg != nil {
		o.LogRef = rs.logRef(info)
	}
	started := time.Now()
	select {
	case ev := <-ra.result:
		return rs.remoteResult(ctx, pj, id, ev, o, n, info, &t, time.Since(started))
	case cause := <-ra.lost:
		select {
		case ev := <-ra.result: // the result won the race with the carrier's end
			return rs.remoteResult(ctx, pj, id, ev, o, n, info, &t, time.Since(started))
		default:
		}
		return rs.remoteLost(ctx, pj, job, rd, id, cause, o, n, &t)
	case <-ctx.Done():
		return rs.remoteCancelled(pj, rd, ra, id, o)
	}
}

// cancelAttempt moves the attempt to cancelled from whichever live state it is in.
func (rs *runState) cancelAttempt(id string) (model.JobState, error) {
	bg, done := background(context.Background())
	defer done()
	state, epoch, err := rs.sess.Store.AttemptStateEpoch(bg, id)
	if err != nil {
		return "", err
	}
	switch state {
	case "leased", "running", "finalizing":
		return state, rs.sess.Store.Transition(bg, id, state, "cancelled", store.TransitionDetail{Epoch: epoch, FailureClass: "cancelled"})
	}
	return state, nil
}

// remoteCancelled ends an attempt whose run was cancelled: cancelled in the store, then the
// remote job is stopped and the carrier is waited for.
func (rs *runState) remoteCancelled(pj plan.PlannedJob, rd *runDispatch, ra *remoteAttempt, id string, o JobOutcome) JobOutcome {
	_, _ = rs.cancelAttempt(id)
	if rd != nil {
		rd.sink.cancelCarrier(id)
	}
	select {
	case <-ra.ended:
	case <-time.After(remoteCancelWait):
	}
	return rs.stopped(o, pj.ID)
}

// remoteLost ends an attempt whose carrier ended without a result (a lost session, a node that
// never answered, a fenced lease). The dispatcher already marked it lost; the CI-40 retry follows.
func (rs *runState) remoteLost(ctx context.Context, pj plan.PlannedJob, job model.Job, rd *runDispatch, id string, cause error, o JobOutcome, n int, t *tail) JobOutcome {
	bg, done := background(context.Background())
	defer done()
	if state, _, err := rs.sess.Store.AttemptStateEpoch(bg, id); err == nil && state == "cancelled" {
		return rs.stopped(o, pj.ID) // a cancel intent won
	}
	if runner := rd.runnerOf(bg, id); runner != "" {
		_, _ = rd.sink.eng.Breaker(bg, runner, true) // best effort: three infra failures in a row put the node in maintenance
	}
	o = failed(o, cause)
	o.Tail = t.String()
	d := diagnose.Classify(diagnose.Input{Lost: true})
	o.FailureClass, o.Diagnosis, o.Remediation = "infra", d.Signature, d.Remediation
	o.retryable = true // the node, not the code, failed to deliver a result: a re-run elsewhere is the proof
	if rs.park(ctx, pj, job, id, &o, n) {
		return o
	}
	_, _ = rs.settle(id, "lost", "needs-operator", "infra")
	return o
}

// runnerOf is the runner a lost attempt was on ("" when unknown).
func (rd *runDispatch) runnerOf(ctx context.Context, attemptID string) string {
	runner, err := rd.sess.Store.LeasePreviousRunner(ctx, attemptID) // recorded when the attempt became lost
	if err != nil {
		return ""
	}
	return runner
}

// remoteResult settles a finalizing attempt from the node's evidence.
func (rs *runState) remoteResult(ctx context.Context, pj plan.PlannedJob, id string, ev model.Evidence, o JobOutcome, n int, info AttemptInfo, t *tail, took time.Duration) JobOutcome {
	o.Duration, o.Tail = took, t.String()
	if ev.Timings.DurationMS != nil {
		o.Duration = time.Duration(*ev.Timings.DurationMS) * time.Millisecond
	}
	switch ev.Result {
	case "pass":
		return rs.finalizePassed(id, o, info)
	case "cancelled":
		if _, err := rs.settle(id, "finalizing", "cancelled", "cancelled"); err != nil {
			return failed(o, err)
		}
		return rs.stopped(o, pj.ID)
	}
	class := model.FailureClass("code")
	exit := 1
	if ev.Result == "error" {
		class = "infra"
	}
	for _, c := range ev.Checks {
		if c.FailureClass != "" {
			class = c.FailureClass
		}
		if c.Exit != nil {
			exit = *c.Exit
		}
	}
	o.ExitCode, o.FailureClass = exit, class
	d := diagnose.Classify(diagnose.Input{ExecClass: class, ExitCode: exit, Tail: o.Tail})
	o.Diagnosis, o.Remediation = d.Signature, d.Remediation
	if class != "infra" {
		o.FailureClass = d.Class
	}
	o.Exit = &exit
	to, err := rs.settle(id, "finalizing", "failed", o.FailureClass)
	if err != nil {
		return failed(o, err)
	}
	if to == "cancelled" {
		return cancelled(o, "", "cancelled by the operator")
	}
	o.State = StateFailed
	return o
}

// finalizePassed ends a finalizing attempt whose job passed: the finalize hooks run
// (results before artifacts), then it becomes passed. The local path and a remote
// result share it.
func (rs *runState) finalizePassed(id string, o JobOutcome, info AttemptInfo) JobOutcome {
	if ferr := rs.attemptFinalize(info); ferr != nil { // a hook refused: the attempt cannot pass
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

// toLost makes sure the attempt is lost: a remote attempt the dispatcher already marked lost
// stays so; a running one is moved.
func (rs *runState) toLost(id string) bool {
	bg, done := background(context.Background())
	defer done()
	if state, _, err := rs.sess.Store.AttemptStateEpoch(bg, id); err == nil && state == "lost" {
		return true
	}
	return rs.trans(id, "running", "lost", "infra") == nil
}
