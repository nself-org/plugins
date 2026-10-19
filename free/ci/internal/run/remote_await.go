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
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/diagnose"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// endedGrace is how long the run waits for the dispatcher to report a carrier that returned.
const endedGrace = 5 * time.Second

// remoteCancelWait bounds how long a cancelled run waits for the carrier to return.
var remoteCancelWait = 15 * time.Second

// SetRemoteCancelWait changes that bound and returns the function that restores it. A sim test makes it
// huge, so the only way the run can end is the cancel reaching the carrier, whatever the machine's load.
func SetRemoteCancelWait(d time.Duration) (restore func()) {
	was := remoteCancelWait
	remoteCancelWait = d
	return func() { remoteCancelWait = was }
}

// newEmitter returns the function a node's output goes through before the log, the tail and the
// stream see it: the run's redactor (a node is not trusted to have masked its own output), and a
// flush for the tail the redactor holds back.
func newEmitter(red ciexec.Redactor, sink func([]byte)) (emit func([]byte), flush func()) {
	emit = func(b []byte) {
		if red != nil {
			b = red.Redact(b)
		}
		if len(b) > 0 {
			sink(b)
		}
	}
	flush = func() {
		if red != nil {
			if b := red.Flush(); len(b) > 0 {
				sink(b)
			}
		}
	}
	return emit, flush
}

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
		rs.driveToCancelled(id)
		return failed(o, fail(CodeOptions, "attempt %s was placed on a remote runner; this run has no way to await it", id))
	}
	info := rs.info(pj, job, id, n)
	lg := rs.openLog(ctx, info)
	defer closeLog(lg)
	var t tail
	var red ciexec.Redactor
	if rs.hooks.NewRedactor != nil {
		red = rs.hooks.NewRedactor()
	}
	emit, flush := newEmitter(red, func(b []byte) {
		t.write(b)
		writeLog(lg, b)
		if rs.c.Opts.Stream {
			rs.render.Event(Event{Type: "log", Job: pj.ID, Text: string(b)})
		}
	})
	ra.attach(emit)
	var once sync.Once
	finish := func() { once.Do(func() { ra.detach(); flush() }) } // before the tail is read: a redactor may hold the last bytes back
	defer finish()
	rs.render.Event(Event{Type: "started", Job: pj.ID})
	if lg != nil {
		o.LogRef = rs.logRef(info)
	}
	started := time.Now()
	result := func(ev model.Evidence) JobOutcome {
		finish()
		return rs.remoteResult(ctx, pj, id, ev, o, n, info, &t, time.Since(started))
	}
	lost := func(cause error) JobOutcome {
		finish()
		return rs.remoteLost(ctx, pj, job, rd, id, cause, o, n, &t)
	}
	select {
	case ev := <-ra.result:
		return result(ev)
	case cause := <-ra.lost:
		select {
		case ev := <-ra.result: // the result won the race with the carrier's end
			return result(ev)
		default:
		}
		return lost(cause)
	case <-ra.ended: // the carrier returned: its result, or the dispatcher's report of its loss (which follows at once), is next
		select {
		case ev := <-ra.result:
			return result(ev)
		case cause := <-ra.lost:
			return lost(cause)
		case <-time.After(endedGrace):
			return lost(errors.New("the carrier ended without a result"))
		}
	case <-ctx.Done():
		finish()
		return rs.remoteCancelled(pj, rd, ra, id, o)
	}
}

// driveToCancelled moves the attempt to cancelled from whichever state it is in, re-reading after a
// lost compare-and-set (a result, a loss or a cancel intent can land between the read and the move).
// A lost attempt is cancelled directly: a cancel needs no retry authorization (a release job, an
// at-most-once job and an exhausted retry budget can all be cancelled).
func (rs *runState) driveToCancelled(id string) error {
	bg, done := background(context.Background())
	defer done()
	var last error
	for i := 0; i < 8; i++ {
		state, epoch, err := rs.sess.Store.AttemptStateEpoch(bg, id)
		if err != nil {
			return err
		}
		to := model.JobState("cancelled")
		switch state {
		case "cancelled":
			return nil
		case "passed", "failed", "needs-operator":
			return nil // it ended on its own first (needs-operator takes an operator to move)
		}
		d := store.TransitionDetail{Epoch: epoch, FailureClass: "cancelled"}
		if last = rs.sess.Store.Transition(bg, id, state, to, d); last != nil && CodeOf(last) != codeStoreRef {
			return last
		}
	}
	return last
}

// remoteCancelled ends an attempt whose run was cancelled: cancelled in the store, then the
// remote job is stopped and the carrier is waited for.
func (rs *runState) remoteCancelled(pj plan.PlannedJob, rd *runDispatch, ra *remoteAttempt, id string, o JobOutcome) JobOutcome {
	err := rs.driveToCancelled(id)
	if rd != nil {
		rd.sink.cancelCarrier(id)
	}
	select {
	case <-ra.ended:
	case <-time.After(remoteCancelWait):
	}
	return rs.cancelledOutcome(pj, o, err)
}

// cancelledOutcome is the outcome of a cancelled attempt; a store error that left it unsettled is said.
func (rs *runState) cancelledOutcome(pj plan.PlannedJob, o JobOutcome, settleErr error) JobOutcome {
	o = rs.stopped(o, pj.ID)
	if settleErr != nil {
		o.Detail = strings.TrimSpace(o.Detail + " (the attempt could not be recorded cancelled: " + settleErr.Error() + ")")
	}
	return o
}

// remoteLost ends an attempt whose carrier ended without a result (a lost session, a node that
// never answered, a fenced lease). The dispatcher normally marked it lost already; the CI-40 retry follows.
func (rs *runState) remoteLost(ctx context.Context, pj plan.PlannedJob, job model.Job, rd *runDispatch, id string, cause error, o JobOutcome, n int, t *tail) JobOutcome {
	bg, done := background(context.Background())
	defer done()
	state, _, err := rs.sess.Store.AttemptStateEpoch(bg, id)
	if err != nil {
		return failed(o, err)
	}
	if state == "cancelled" {
		return rs.stopped(o, pj.ID) // a cancel intent won
	}
	if state == "leased" || state == "running" { // the dispatcher could not mark it lost: a cancel intent may have won
		to, serr := rs.settle(id, state, "lost", "infra")
		if serr != nil {
			return failed(o, serr)
		}
		if to == "cancelled" {
			return rs.stopped(o, pj.ID)
		}
	}
	if ctx.Err() != nil { // the run was cancelled too (Ctrl-C reaches ssh as well): cancelled, not lost
		return rs.cancelledOutcome(pj, o, rs.driveToCancelled(id))
	}
	if runner := rd.runnerOf(bg, id); runner != "" && nodeFault(cause) {
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
	if _, serr := rs.settle(id, "lost", "needs-operator", "infra"); serr != nil {
		return failed(o, serr)
	}
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
	if ms := ev.Timings.DurationMS; ms != nil && *ms >= 0 && time.Duration(*ms) <= took/time.Millisecond { // a node's claim is never longer than the wait for it
		o.Duration = time.Duration(*ms) * time.Millisecond
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
