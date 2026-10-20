package dispatch

// Purpose: the waiting hook (Epic D12 wiring, D-1329): a registered function sees every job a tick could
// not place and left waiting, with the facts a burst decision needs (how long it queued, how many jobs of
// its pipeline wait ahead of it and in all, and the expected wait). The burst engine registers one from its
// init; with no engine bound it does nothing.
// Inputs:  OnWaiting calls from init(); one Waiting per waiting job per tick.
// Outputs: none to the dispatcher: a hook cannot change a placement.
// Constraints: hooks run inside the tick, in registration order, so a hook must return fast and start any
// slow work in its own goroutine; a panic in a hook is not recovered (a programming error); a job that was
// admitted, refused by a step, timed out or declared unplaceable is not reported.

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Waiting describes one job a tick left waiting.
type Waiting struct {
	Job Job
	// Pipeline is the id of the job's pipeline; Project its project.
	Pipeline, Project string
	EnqueuedAt        time.Time
	// Waited is how long the job has queued at the tick.
	Waited time.Duration
	// Ahead counts jobs of the same pipeline that also wait and come before this one in this tick's
	// order; Demand counts every such waiting job of the pipeline, this one included.
	Ahead, Demand int
	// ExpectedWaitMs is the time this job still expects to wait behind Ahead jobs (ExpectedWait over the
	// settled runtimes); it is never below one runtime share, because the job found no free slot.
	ExpectedWaitMs int64
}

var (
	waitMu    sync.Mutex
	waitHooks []func(context.Context, Waiting)
)

// OnWaiting registers a hook that runs for each waiting job at each tick.
func OnWaiting(f func(context.Context, Waiting)) {
	if f == nil {
		panic("dispatch: nil waiting hook")
	}
	waitMu.Lock()
	defer waitMu.Unlock()
	waitHooks = append(waitHooks, f)
}

// setWaitHooks replaces the hooks (tests) and returns the function that puts the old ones back.
func setWaitHooks(f ...func(context.Context, Waiting)) (restore func()) {
	waitMu.Lock()
	defer waitMu.Unlock()
	old := waitHooks
	waitHooks = f
	return func() {
		waitMu.Lock()
		waitHooks = old
		waitMu.Unlock()
	}
}

// fireWaiting tells every hook that q waits. pr carries the tick's count of waiting jobs of the pipeline.
func (d *Dispatcher) fireWaiting(ctx context.Context, pr *pipelineRun, q Queued) {
	waitMu.Lock()
	hooks := slices.Clone(waitHooks)
	waitMu.Unlock()
	pr.waitSeen++
	if len(hooks) == 0 {
		return
	}
	wait, _ := d.expectedWait(ctx, q.Job.Sched.Project, int64(pr.waitSeen))
	w := Waiting{Job: q.Job, Pipeline: q.Job.Sched.Pipeline.ID, Project: q.Job.Sched.Project, EnqueuedAt: q.EnqueuedAt,
		Waited: d.cfg.Clock().Sub(q.EnqueuedAt), Ahead: pr.waitSeen - 1, Demand: pr.waitTotal, ExpectedWaitMs: wait}
	for _, h := range hooks {
		h(ctx, w)
	}
}
