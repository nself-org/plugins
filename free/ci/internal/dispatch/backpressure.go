package dispatch

// Purpose: backpressure at submission (Epic D9, D17): a project that already has
// sched.queue.max_depth jobs queued is refused with E694, and the refusal says
// how long the wait would be (expected_wait_ms) so the caller can decide.
// Inputs:  the project's queued depth, the store's settled runtime estimators.
// Outputs: a *BackpressureError, or nil.
// Constraints: the wait is jobs ahead x mean runtime / slots, from the settled
// estimators (at least minSamples samples, as in internal/schedtel); with none it
// uses the 60 s stand-in runtime of the scheduler and says the figure is
// "unknown". It is a hint, never a promise. A job is refused before it is
// queued, so refusing never loses a queued job.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/fairq"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	// queueLimitsTimeout bounds the policy read an ordering makes.
	queueLimitsTimeout = 5 * time.Second
	// minSamples is the sample count at which an estimator is settled
	// (schedtel.MinSamples; dispatch cannot import schedtel).
	minSamples = 3
	// standInRuntimeMs is the scheduler's runtime stand-in for an unknown job.
	standInRuntimeMs = int64(60000)
)

// BackpressureError is E694: the project's queue is full.
type BackpressureError struct {
	Err            Error
	Project        string
	Depth, Max     int64
	ExpectedWaitMs int64
	// WaitConfidence is "estimated" (settled estimators) or "unknown" (stand-in runtime).
	WaitConfidence string
}

func (e *BackpressureError) Error() string { return e.Err.Error() }

// Unwrap lets errors.As find the *Error with the code.
func (e *BackpressureError) Unwrap() error { return &e.Err }

// queueLimits is the fairness and depth policy now in force; a policy that
// cannot be read falls back to the registered defaults, never to "no cap".
func (d *Dispatcher) queueLimits(ctx context.Context) QueueLimits {
	if d.cfg.Queue == nil {
		return DefaultQueueLimits()
	}
	l, err := d.cfg.Queue(ctx)
	if err != nil {
		return DefaultQueueLimits()
	}
	return l
}

// backpressure builds the E694 error for a project at depth jobs.
func (d *Dispatcher) backpressure(ctx context.Context, project string, depth, max int64) *BackpressureError {
	wait, conf := d.expectedWait(ctx, project, depth)
	return &BackpressureError{
		Err:     *fail(fairq.CodeBackpressure, "project %q has %d jobs queued (queue.max_depth %d); expected wait about %ds (expected_wait_ms=%d, %s)", project, depth, max, wait/1000, wait, conf),
		Project: project, Depth: depth, Max: max, ExpectedWaitMs: wait, WaitConfidence: conf,
	}
}

// expectedWait is the time a job submitted now would wait behind ahead queued jobs.
func (d *Dispatcher) expectedWait(ctx context.Context, project string, ahead int64) (int64, string) {
	slots := 1
	if d.cfg.Slots != nil {
		slots = d.cfg.Slots()
	}
	return ExpectedWait(ctx, d.cfg.Store, project, ahead, slots)
}

// RuntimeEstimate is the mean settled runtime in ms for project (else for every
// project) and its confidence: "estimated", or "unknown" with the stand-in.
func RuntimeEstimate(ctx context.Context, st *store.Store, project string) (int64, string) {
	if st == nil {
		return standInRuntimeMs, "unknown"
	}
	ests, err := st.TelEstimators(ctx)
	if err != nil {
		return standInRuntimeMs, "unknown"
	}
	var mine, all, nMine, nAll int64
	for _, e := range ests {
		if e.N < minSamples {
			continue
		}
		all, nAll = all+e.RuntimeMs, nAll+1
		if e.Project == project {
			mine, nMine = mine+e.RuntimeMs, nMine+1
		}
	}
	switch {
	case nMine > 0:
		return mine / nMine, "estimated"
	case nAll > 0:
		return all / nAll, "estimated"
	}
	return standInRuntimeMs, "unknown"
}

// ExpectedWait is ahead x mean runtime / slots in ms and the confidence of the
// runtime (see RuntimeEstimate). It is also what `ci runs queue` shows.
func ExpectedWait(ctx context.Context, st *store.Store, project string, ahead int64, slots int) (int64, string) {
	runtime, conf := RuntimeEstimate(ctx, st, project)
	return WaitFor(ahead, runtime, slots), conf
}

// WaitFor is ahead x runtime / slots in ms, saturating.
func WaitFor(ahead, runtimeMs int64, slots int) int64 {
	n, _ := sched.Mul(ahead, runtimeMs)
	return n / int64(max(slots, 1))
}
