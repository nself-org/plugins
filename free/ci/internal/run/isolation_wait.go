package run

// Purpose: an attempt whose isolation level is unavailable now waits instead of ending (Epic D7, D-0964).
// Inputs:  the leased attempt's JobSpec, as runLeased built it.
// Outputs: the executor's result; an E678 refusal becomes a wait while the budget for its reason lasts.
// Constraints: only a refusal before anything ran is waited out (AttemptResult.Refused: the executor fails the Isolator's
// Prepare before the workspace exists), so a re-run cannot repeat a side effect; the slot is given back while waiting so another
// attempt can use it; a cancel or the run's end stops the wait at once; a job is never run at a weaker level.

import (
	"context"
	"errors"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The wait budgets by reason. A slot that is not mapped frees when another attempt of this node ends, which can
// take a whole job; a runtime that is missing or failing a probe is retried briefly, then the attempt fails as
// infra with the E678 it got (waiting for a node that is fixed later is the dispatcher's job, not this loop's).
// The waiter gives its slot back while it sleeps, so a newly started attempt can take a freed slot first; the
// budget bounds how long that can go on.
var (
	slotWaitMax    = 30 * time.Minute
	runtimeWaitMax = time.Minute
	waitStepFirst  = 2 * time.Second
	waitStepMax    = 15 * time.Second
	waitNoteEvery  = time.Minute
)

// waitBudget is how long an attempt that holds slot waits for the reason of an E678, 0 for a reason that waiting
// cannot cure. Slot 0 is the first UID the runtime would map: when even that is refused no other attempt can free
// a better one.
func waitBudget(reason string, slot int) time.Duration {
	switch reason {
	case isolation.ReasonSlotUID:
		if slot > 0 {
			return slotWaitMax
		}
	case isolation.ReasonRuntimeMissing, isolation.ReasonProbeFailed, isolation.ReasonLimits:
		return runtimeWaitMax
	}
	return 0
}

// runWaiting runs the attempt through the executor and waits out an E678 the Isolator raised before anything ran
// (AttemptResult.Refused). An E678 from later in the run, such as a planner's, ends the attempt as before: its
// hooks and secrets have already been used. It takes the execution slot itself and gives it back on every return,
// a panic included.
func (rs *runState) runWaiting(ctx context.Context, spec ciexec.JobSpec) (res ciexec.AttemptResult) {
	dir := slotDir(rs.sess.Home)
	spec.Slot = rs.sess.slots.take(dir)
	release := func() { rs.sess.slots.put(spec.Slot); spec.Slot = -1 }
	defer func() {
		if spec.Slot >= 0 {
			release()
		}
	}()
	start, step, lastNote := time.Now(), waitStepFirst, time.Time{}
	for {
		res = rs.c.exec.Run(ctx, spec)
		var ie *isolation.Error
		if ctx.Err() != nil || !res.Refused || !errors.As(res.Err, &ie) || ie.Code != "E678" || time.Since(start)+step > waitBudget(ie.Reason, spec.Slot) {
			return res
		}
		release()
		if spec.Output != nil && time.Since(lastNote) >= waitNoteEvery {
			lastNote = time.Now()
			spec.Output([]byte("nself-ci: waiting for " + string(spec.Job.Isolation) + " isolation (" + ie.Reason + "); the job has not started\n"))
		}
		t := time.NewTimer(step)
		select {
		case <-ctx.Done():
			t.Stop()
			return ciexec.AttemptResult{Attempt: model.Attempt{ID: spec.AttemptID, State: "cancelled", FailureClass: "cancelled"}, Err: ctx.Err(), Started: res.Started, Finished: time.Now()}
		case <-t.C:
		}
		step = min(step*2, waitStepMax)
		spec.Slot = rs.sess.slots.take(dir)
	}
}
