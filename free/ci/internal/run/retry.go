package run

// Purpose: failure diagnostics, bounded infra retries and explicit flaky
// confirmation for one job (P7-CI-40, cap:ci.diagnostics). diagnose classifies
// a failed attempt with internal/diagnose; runAttempt retries a job only when
// the failure is proven infrastructure, up to retry.infra_max, and never a
// release or idempotent:false job; confirmFlaky reruns a code failure only when
// the operator asked (--confirm-flaky N).
// Inputs:  the planned job, its outcome shell and the run Options.
// Outputs: the final JobOutcome with its Retries, diagnostics and Flaky note.
// Constraints: every extra attempt is made by CreateAttempt (no AdmitAndLease
// here); a deterministic (code) failure is never retried automatically; a
// confirmation rerun never changes the original result, it only records
// counted attempts; cancellation, fail-fast and a cancelled context end the
// loop with the last outcome.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/diagnose"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// confirmingKey marks the context of a --confirm-flaky rerun: park refuses it.
type confirmingKey struct{}

// MaxConfirmFlaky bounds --confirm-flaky.
const MaxConfirmFlaky = 10

// retryable reports whether a job may be run again at all: a release or
// idempotent:false job runs at most once (P7-CI-31, needs-operator).
func retryable(job model.Job) bool {
	return job.Kind != "release" && (job.Idempotent == nil || *job.Idempotent)
}

// infraBudget is how many extra attempts a job may use for infra failures.
func infraBudget(job model.Job) int {
	if !retryable(job) || job.Retry == nil {
		return 0
	}
	return min(max(job.Retry.InfraMax, 0), plan.MaxInfraRetries)
}

// diagnose classifies a failed attempt and records the diagnostics on o.
func (rs *runState) diagnose(o *JobOutcome, res *ciexec.AttemptResult) {
	if res.Attempt.State != "failed" {
		return
	}
	in := diagnose.Input{ExecClass: res.Attempt.FailureClass, ExitCode: res.ExitCode, Reason: res.Reason, Signal: res.Signal, Tail: o.Tail}
	if res.Err != nil {
		in.Err = res.Err.Error()
	}
	d := diagnose.Classify(in)
	res.Attempt.FailureClass = d.Class
	o.FailureClass, o.Diagnosis, o.Remediation, o.retryable = d.Class, d.Signature, d.Remediation, d.Retryable
	if res.Err == nil || res.ExitCode != 0 {
		code := res.ExitCode
		o.Exit = &code
	}
}

// diagnosePrepare records diagnostics for an attempt that failed before the job started.
func (rs *runState) diagnosePrepare(o *JobOutcome, err error) {
	d := diagnose.Classify(diagnose.Input{ExecClass: "infra", Err: err.Error()})
	o.FailureClass, o.Diagnosis, o.Remediation, o.retryable = d.Class, d.Signature, d.Remediation, d.Retryable
}

// logRef names the stored log of an attempt: `ci logs --run <run> --attempt <n>`.
func (rs *runState) logRef(info AttemptInfo) string {
	return "logs/" + info.RunID + "/" + info.AttemptID
}

// stillRunning reports whether the run may start another attempt of pj.
func (rs *runState) stillRunning(ctx context.Context, pj plan.PlannedJob) bool {
	if ctx.Err() != nil || rs.marked(pj.ID) != "" {
		return false
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return !rs.userCancel
}

// park ends a failed attempt that will be retried through the store's retry
// path (running -> lost -> retryable -> queued; the store enforces the bound,
// the idempotent and release rules). It reports whether the attempt was
// parked; when it is, o is a failed infra outcome marked parked and the
// caller must not settle it.
func (rs *runState) park(ctx context.Context, pj plan.PlannedJob, job model.Job, id string, o *JobOutcome, n int) bool {
	if ctx.Value(confirmingKey{}) != nil || !o.retryable || o.FailureClass != "infra" || n > infraBudget(job) || !rs.stillRunning(ctx, pj) {
		return false
	}
	if !rs.toLost(id) { // a cancel intent or an unexpected state: settle normally
		return false
	}
	for _, to := range []model.JobState{"retryable", "queued"} {
		from := map[model.JobState]model.JobState{"retryable": "lost", "queued": "retryable"}[to]
		if err := rs.trans(id, from, to, "infra"); err != nil {
			if CodeOf(err) == codeStoreRef { // a cancel intent won the race: the operator's cancel ends the attempt
				_ = rs.driveToCancelled(id)
				*o = cancelled(*o, "", "cancelled by the operator")
				return true
			}
			*o = failed(*o, err) // left lost: the next recovery sweep decides
			return true
		}
	}
	o.State, o.parked = StateFailed, true
	return true
}

// retire cancels the parked attempt before s.N once its successor exists (or failed to).
func (rs *runState) retire(s AttemptSpec) {
	if s.N < 2 {
		return
	}
	prev := s
	prev.N--
	id, err := AttemptIDFor(WithSession(context.Background(), rs.sess), prev)
	if err != nil {
		return
	}
	bg, done := background(context.Background())
	defer done()
	if state, epoch, e := rs.sess.Store.AttemptStateEpoch(bg, id); e == nil && state == "queued" {
		_ = rs.sess.Store.Transition(bg, id, "queued", "cancelled", store.TransitionDetail{Epoch: epoch, FailureClass: "infra", Reason: "superseded by retry attempt " + strconv.Itoa(s.N)})
	}
}

// runAttempt runs a job: attempt 1, then infra retries, then (on request)
// flaky confirmation reruns. The returned outcome is the last real attempt's,
// except that a confirmation never replaces a failed result.
func (rs *runState) runAttempt(ctx context.Context, i int, pj plan.PlannedJob, o JobOutcome) JobOutcome {
	job := rs.mp.Jobs[i]
	var retries []model.Retry
	n := 1
	out := rs.runAttemptN(ctx, i, pj, o, n)
	for out.parked {
		if !rs.stillRunning(ctx, pj) { // cancelled between the park and the retry
			rs.retire(AttemptSpec{Pipeline: rs.mp, Job: &rs.mp.Jobs[i], N: n + 1})
			out.parked = false
			break
		}
		rs.render.Event(Event{Type: "retrying", Job: pj.ID, Text: fmt.Sprintf("attempt %d failed (%s); retrying as attempt %d of at most %d", n, out.Diagnosis, n+1, infraBudget(job)+1)})
		why := model.Retry{JobID: pj.ID, Attempt: n + 1, Class: out.FailureClass, Diagnosis: out.Diagnosis, Excerpt: diagnose.Excerpt(out.Tail, 1024)}
		n++
		out = rs.runAttemptN(ctx, i, pj, o, n)
		if out.AttemptID != "" { // the retry is recorded only once its attempt exists
			retries = append(retries, why)
		}
	}
	if rs.c.Opts.ConfirmFlaky > 0 && retryable(job) && out.State == StateFailed && out.FailureClass == "code" {
		retries, out.Flaky = rs.confirmFlaky(ctx, i, pj, o, retries, n, out.AttemptID)
	}
	out.Retries = retries
	return out
}

// confirmFlaky reruns a code failure up to Options.ConfirmFlaky times. Each
// rerun is authorized in the store (an audit row naming the failed attempt),
// created by CreateAttempt and recorded as a retry of class code, so evidence
// shows it was a confirmation and never an automatic retry. It returns the
// retries and a Flaky note when at least one rerun passed. The original failed
// outcome is untouched.
func (rs *runState) confirmFlaky(ctx context.Context, i int, pj plan.PlannedJob, o JobOutcome, retries []model.Retry, n int, failedID string) ([]model.Retry, *model.Flaky) {
	passes, fails := 0, 1 // the original failure counts as one failed run
	prev := failedID
	for k := 0; k < min(rs.c.Opts.ConfirmFlaky, MaxConfirmFlaky) && rs.stillRunning(ctx, pj); k++ {
		bg, done := background(ctx)
		err := rs.sess.Store.AuthorizeConfirmation(bg, "coordinator", prev, "confirm-flaky "+strconv.Itoa(k+1)+"/"+strconv.Itoa(rs.c.Opts.ConfirmFlaky))
		done()
		if err != nil {
			break
		}
		n++
		rs.render.Event(Event{Type: "retrying", Job: pj.ID, Text: "confirmation rerun " + strconv.Itoa(k+1) + " of the failed job (attempt " + strconv.Itoa(n) + ")"})
		r := rs.runAttemptN(context.WithValue(ctx, confirmingKey{}, true), i, pj, o, n) // a confirmation rerun is never parked for an infra retry
		if r.AttemptID == "" {
			break // refused or cancelled before an attempt existed: nothing ran, nothing to count
		}
		retries = append(retries, model.Retry{JobID: pj.ID, Attempt: n, Class: "code", Diagnosis: "confirm-flaky"})
		switch r.State {
		case StatePassed:
			passes++
		case StateFailed:
			if r.FailureClass != "code" {
				return retries, flakyNote(passes, fails) // an infra error says nothing about the code: not counted
			}
			fails++
		default:
			return retries, flakyNote(passes, fails)
		}
		if r.State != StateFailed || r.FailureClass != "code" || r.AttemptID == "" {
			break // only a failed code attempt can be confirmed again
		}
		prev = r.AttemptID
	}
	return retries, flakyNote(passes, fails)
}

func flakyNote(passes, fails int) *model.Flaky {
	if passes == 0 {
		return nil // every rerun failed too: a deterministic failure, not a flake
	}
	return &model.Flaky{Source: "confirmation", Passes: passes, Fails: fails}
}
