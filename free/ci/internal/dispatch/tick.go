package dispatch

// Purpose: one tick (Epic D7): build each pipeline's world, end the jobs that
// waited past max_queue, place the rest per pipeline, then walk ALL queued jobs
// in the Orderer's order and admit each placement through Admit.
// Inputs:  the queued jobs, in Orderer order.
// Outputs: waiters resolved; one Decision per job sent to the sink; refusal
// reasons stored for `ci explain`.
// Constraints: one world per pipeline, because the policy is per pipeline;
// admission order is the Orderer's order across pipelines (a younger job of one
// pipeline never passes an older job of another); every commit is applied to
// every world copy with World.Apply; a placement is re-checked by the admission
// transaction, so a world that went stale cannot oversubscribe; a job past its
// deadline is ended before it is placed, so it never runs after E693.

import (
	"context"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// pipelineRun is one pipeline's world and placement result for this tick.
type pipelineRun struct {
	world     sched.World
	digest    string
	err       error
	jobs      []Queued
	placed    map[string]sched.Placement
	explained map[string]sched.Explanation
}

// Tick places and admits every queued job once, in the Orderer's order. It
// works for every queued job, so it does not stop when the caller that happened
// to run it gives up: that caller's cancellation must not fail another job.
func (d *Dispatcher) Tick(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.cfg.TickTimeout)
	defer cancel()
	d.tickM.Lock()
	defer d.tickM.Unlock()
	d.mu.Lock()
	closed := d.closed
	d.mu.Unlock()
	if closed || d.quiesced {
		return errClosed
	}
	now := d.cfg.Clock()
	queued := d.pending(now)
	if len(queued) == 0 {
		return nil
	}
	ordered := d.cfg.Orderer.Order(queued)
	if len(ordered) != len(queued) {
		return errors.New("dispatch: the orderer must return exactly the queued jobs")
	}
	runs, first := d.buildWorlds(ctx, ordered)
	now = d.cfg.Clock() // building the worlds takes time
	var live []Queued
	for _, q := range ordered {
		pr := runs[q.Job.Sched.Pipeline.ID]
		if pr.err != nil {
			continue
		}
		if d.pastDeadline(q, pr.world.Policy.Overrides.MaxQueueMs, now) {
			d.timeout(ctx, q, pr.digest, now)
			continue
		}
		pr.jobs = append(pr.jobs, q)
		live = append(live, q)
	}
	for _, pr := range runs {
		if pr.err != nil || len(pr.jobs) == 0 {
			continue
		}
		res := place(pr.world, pr.jobs)
		pr.placed, pr.explained = map[string]sched.Placement{}, map[string]sched.Explanation{}
		for _, p := range res.Placements {
			pr.placed[p.JobKey] = p
		}
		for _, x := range res.Explanations {
			pr.explained[x.JobKey] = x
		}
	}
	d.notePending(ctx, runs)
	for _, q := range live {
		d.settle(ctx, runs, runs[q.Job.Sched.Pipeline.ID], q)
	}
	return first
}

// buildWorlds builds the world of every pipeline that has queued jobs. A
// pipeline whose world cannot be built fails its foreground jobs at once.
func (d *Dispatcher) buildWorlds(ctx context.Context, ordered []Queued) (map[string]*pipelineRun, error) {
	runs := map[string]*pipelineRun{}
	var first error
	for _, q := range ordered {
		id := q.Job.Sched.Pipeline.ID
		if _, ok := runs[id]; ok {
			continue
		}
		pr := &pipelineRun{}
		pr.world, pr.digest, pr.err = d.cfg.World(ctx, q.Job.Sched.Pipeline)
		runs[id] = pr
		if pr.err == nil {
			continue
		}
		if first == nil {
			first = pr.err
		}
		for _, o := range ordered {
			if o.Job.Sched.Pipeline.ID == id && o.Job.Foreground {
				d.finish(o.Seq, Admitted{}, pr.err)
			}
		}
	}
	return runs, first
}

// place hands every queued job, in the given order, to sched.Place.
func place(w sched.World, ordered []Queued) sched.Result {
	jobs := make([]sched.Job, len(ordered))
	for i, q := range ordered {
		jobs[i] = q.Job.Sched
	}
	return sched.Place(w, jobs)
}

// Plan orders the queued jobs with the Orderer and places them against w. It
// is the pure half of a tick (no store, no admission): the placements it
// returns are what a tick would try to admit, in that order.
func (d *Dispatcher) Plan(w sched.World, queued []Queued) ([]Queued, sched.Result) {
	ordered := d.cfg.Orderer.Order(queued)
	return ordered, place(w, ordered)
}

// settle admits one job's placement, or explains why it waits, and records the decision.
func (d *Dispatcher) settle(ctx context.Context, runs map[string]*pipelineRun, pr *pipelineRun, q Queued) {
	dec := Decision{WorldDigest: pr.digest, Job: q.Job.Sched, Explanation: pr.explained[q.Job.Sched.Key], World: &pr.world, EnqueuedAt: q.EnqueuedAt}
	if d.pastDeadline(q, pr.world.Policy.Overrides.MaxQueueMs, d.cfg.Clock()) { // earlier admissions took time too
		d.timeout(ctx, q, pr.digest, d.cfg.Clock())
		return
	}
	if p, ok := pr.placed[q.Job.Sched.Key]; ok {
		dec.Placement = &p
		d.admit(ctx, runs, pr, q, p, &dec)
	} else {
		d.waiting(ctx, q, &dec)
	}
	if !dec.recorded {
		d.cfg.Sink.Record(ctx, dec)
	}
}

// pastDeadline reports whether q waited past max_queue (an interactive foreground
// job has a 10 minute default, R-DX-06).
func (d *Dispatcher) pastDeadline(q Queued, maxQueueMs int64, now time.Time) bool {
	limit := maxQueueMs
	if limit <= 0 && q.Job.Foreground && !q.Job.NoDefaultQueueCap {
		limit = defaultForegroundQueueMs
	}
	return limit > 0 && now.Sub(q.EnqueuedAt) >= time.Duration(limit)*time.Millisecond
}

// timeout ends a job that waited past max_queue with E693, before it is placed.
func (d *Dispatcher) timeout(ctx context.Context, q Queued, digest string, now time.Time) {
	waited := now.Sub(q.EnqueuedAt).Round(time.Second)
	d.remember(ctx, q, model.QueueTimeout, "")
	d.finish(q.Seq, Admitted{}, fail(CodeQueueTimeout, "job %s waited %s in the queue; queue.timeout", q.Job.Sched.Name, waited))
	d.cfg.Sink.Record(ctx, Decision{WorldDigest: digest, Job: q.Job.Sched, Outcome: OutcomeTimeout, Reason: model.QueueTimeout, EnqueuedAt: q.EnqueuedAt})
}
