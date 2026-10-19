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
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/diagnose"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

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
	defer func() { flush(); ra.detach() }()
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

// driveToCancelled moves the attempt to cancelled from whichever state it is in, re-reading after a
// lost compare-and-set (a result, a loss or a cancel intent can land between the read and the move).
// A lost attempt reaches cancelled through retryable and queued, the way a parked attempt is retired.
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
		case "lost":
			to = "retryable"
		case "retryable":
			to = "queued"
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
	o = rs.stopped(o, pj.ID)
	if err != nil {
		o.Detail = strings.TrimSpace(o.Detail + " (the attempt could not be recorded cancelled: " + err.Error() + ")")
	}
	return o
}

// nodeFault says whether a carrier error is the node's doing (it counts against the node's breaker).
// A bundle that cannot be made, a driver nobody configured and a refused secret are ours.
func nodeFault(err error) bool {
	t := err.Error()
	for _, ours := range []string{"transport.ssh_checkout", "transport.ssh_not_configured", "transport.ssh_secrets_refused", "transport.ssh_no_destination", "transport.ssh_blob"} {
		if strings.Contains(t, ours) {
			return false
		}
	}
	return true
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
		_ = rs.driveToCancelled(id)
		return rs.stopped(o, pj.ID)
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

// localOnly says a job must run on this machine because the remote path cannot honour what the
// local one does: outcome hooks that read the output (drill), declared artifacts and caches (no
// intake from a node yet), secrets (no value is sent to a node), services, requirements the world
// cannot verify (labels, tools, an isolation floor, a container: the local runner reports no
// measured facts for them) unless the job is remote-only, and the invocation's own controls
// (--container, --network, --service, --timeout), which the agent never receives.
func (rs *runState) localOnly(job model.Job) bool {
	o := rs.c.Opts
	if o.Container || o.Network != "" || o.Timeout > 0 || len(o.Services) > 0 {
		return true
	}
	if rs.wantsOutput(job) || len(job.Artifacts) > 0 || job.Cache != nil || len(job.Secrets) > 0 || len(job.NeedsServices) > 0 {
		return true
	}
	if job.RemoteOnly {
		return false
	}
	r := job.Requirements
	return r != nil && (len(r.Labels) > 0 || len(r.Tools) > 0 || len(r.Accelerators) > 0) || job.Isolation != "" || job.Container != nil
}
