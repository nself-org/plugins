package run

// Purpose: the run loop. One goroutine per planned job waits for its
// dependencies, then creates and runs its attempt through CreateAttempt;
// admission (the host reservation) bounds how many run at once. A failed
// required job applies the plan's fail-fast scope: dependants are skipped,
// or with pipeline scope every other job is cancelled, running ones included.
// Inputs:  the plan, the persisted pipeline and the Session.
// Outputs: one JobOutcome per planned job.
// Constraints: a job that did not run always has a reason; cancellation of
// the run context cancels every running attempt; this file never touches the
// store state machine directly (jobrun.go does).

import (
	"context"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

type runState struct {
	c      *Coordinator
	sess   *Session
	hooks  Hooks
	pol    PolicyResult // the resolved policy of this run
	pl     *plan.Plan
	mp     *model.Pipeline
	rev    Revision
	begin  any               // the evidence builder's start state (read before any job runs)
	rows   map[string]string // planned job id -> job row id
	subst  func(plan.PlannedJob) bool
	render Renderer
	subset map[string]bool

	mu         sync.Mutex
	out        map[string]*JobOutcome
	done       map[string]chan struct{}
	marks      map[string]model.Reason
	cancels    map[string]context.CancelFunc
	attempts   map[string]string // attempt id -> planned job id
	userCancel bool
	cancelRun  context.CancelFunc // ends the whole run (a cancel intent from another process)
}

func (rs *runState) outcome(id string) JobOutcome {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if o := rs.out[id]; o != nil {
		return *o
	}
	return JobOutcome{ID: id}
}

func (rs *runState) marked(id string) model.Reason {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.marks[id]
}

func (rs *runState) setCancel(id string, f context.CancelFunc) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.cancels[id] = f
}

// execute runs every planned job to an outcome.
func (rs *runState) execute(ctx context.Context) {
	for _, pj := range rs.pl.Jobs {
		rs.done[pj.ID] = make(chan struct{})
	}
	var wg sync.WaitGroup
	for i := range rs.pl.Jobs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pj := rs.pl.Jobs[i]
			o := rs.runOne(ctx, i, pj)
			rs.finish(pj, o)
		}(i)
	}
	wg.Wait()
}

func (rs *runState) base(pj plan.PlannedJob) JobOutcome {
	o := JobOutcome{ID: pj.ID, Name: pj.Name, Check: pj.Check, Layer: pj.Layer, Kind: pj.Job.Kind,
		AllowFailure: pj.Job.AllowFailure, Substantive: rs.subst(pj), Tool: ""}
	o.Required = pj.Job.Required != nil && *pj.Job.Required
	return o
}

func skipped(o JobOutcome, why model.Reason, detail string) JobOutcome {
	o.State, o.Reason, o.Detail = StateSkipped, why, detail
	return o
}

func cancelled(o JobOutcome, why model.Reason, detail string) JobOutcome {
	o.State, o.Reason, o.Detail = StateCancelled, why, detail
	return o
}

// depOK reports whether a finished dependency lets its waiters run. A failed
// job always blocks them, allow_failure or not (plan/dag.go: a failing
// allow_failure job still blocks everything that waits on it). A skipped job
// lets them run only when it was scoped out by design (selection, when.*,
// pipeline); one that never ran for want of a place, a tool or admission, or
// that was cancelled, blocks them like a failure.
func depOK(d JobOutcome) bool {
	switch d.State {
	case StatePassed:
		return true
	case StateSkipped:
		r := string(d.Reason)
		return r != "dep.failed" && r != "fail_fast.cancelled" &&
			!strings.HasPrefix(r, "unplaceable.") && !strings.HasPrefix(r, "admission.") && !strings.HasPrefix(r, "advisory.")
	}
	return false
}

func (rs *runState) runOne(ctx context.Context, i int, pj plan.PlannedJob) JobOutcome {
	o := rs.base(pj)
	switch {
	case rs.subset != nil && !rs.subset[pj.ID]:
		return skipped(o, "pipeline.excluded", "not selected by --job")
	case pj.State == plan.StateSkip:
		return skipped(o, firstReason(pj.Blocked, pj.Reason), pj.Detail)
	case pj.State == plan.StateUnplaceable:
		return skipped(o, pj.Blocked, pj.Detail)
	}
	for _, d := range pj.Deps {
		if ch := rs.done[d]; ch != nil {
			select {
			case <-ch:
			case <-ctx.Done():
				return cancelled(o, "", "cancelled while waiting for "+d)
			}
		}
		if dep := rs.outcome(d); !depOK(dep) {
			return skipped(o, "dep.failed", d+" did not pass")
		}
	}
	if m := rs.marked(pj.ID); m != "" {
		return rs.markOutcome(o, m)
	}
	if ctx.Err() != nil {
		return cancelled(o, "", "run cancelled")
	}
	return rs.runAttempt(ctx, i, pj, o)
}

func firstReason(a, b model.Reason) model.Reason {
	if a != "" {
		return a
	}
	return b
}

func (rs *runState) markOutcome(o JobOutcome, m model.Reason) JobOutcome {
	if m == "fail_fast.cancelled" {
		return cancelled(o, m, "another job failed (fail-fast)")
	}
	return skipped(o, m, "a dependency failed")
}

// finish records an outcome and applies fail-fast when a required job failed.
func (rs *runState) finish(pj plan.PlannedJob, o JobOutcome) {
	rs.mu.Lock()
	rs.out[pj.ID] = &o
	delete(rs.cancels, pj.ID)
	var stop []context.CancelFunc
	if o.State == StateFailed && o.Required && !o.AllowFailure {
		ids, why := rs.pl.CancelSet(pj.ID)
		for _, id := range ids {
			if rs.marks[id] == "" {
				rs.marks[id] = why
			}
			if f := rs.cancels[id]; f != nil && why == "fail_fast.cancelled" {
				stop = append(stop, f)
			}
		}
	}
	rs.mu.Unlock()
	for _, f := range stop {
		f()
	}
	rs.render.Event(Event{Type: "finished", Job: pj.ID, Out: &o})
	if rows := rs.rows[pj.ID]; rows != "" {
		bg, done := background(context.Background())
		defer done()
		status := o.State
		_ = rs.sess.Store.SetJobStatus(bg, rows, status)
	}
	close(rs.done[pj.ID])
}

// outcomes returns every outcome in plan order.
func (rs *runState) outcomes() []JobOutcome {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make([]JobOutcome, 0, len(rs.pl.Jobs))
	for _, pj := range rs.pl.Jobs {
		if o := rs.out[pj.ID]; o != nil {
			out = append(out, *o)
		}
	}
	return out
}
