package dispatch

// Purpose: what a tick does with one job: admit its placement through Admit
// and settle the result, or explain why it waits.
// Inputs:  a job, its pipeline's world and placement.
// Outputs: the waiter resolved on admission, a failure or E618; the stored
// reason of a job that stays queued.
// Constraints: the entry is claimed before Admit, so a caller that gives up
// waits for the commit and keeps the lease it would otherwise orphan; a
// delivery is registered with the WaitGroup before the waiter is released, so
// Close cannot return while a carrier is about to start.

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// admit sends one placement through the chokepoint and settles the job.
func (d *Dispatcher) admit(ctx context.Context, runs map[string]*pipelineRun, pr *pipelineRun, q Queued, p placement, dec *Decision) {
	r, ok := runnerOf(&pr.world, p.RunnerID)
	local := ok && r.Location == "local"
	if !ok || (!local && !d.knownTransport(r)) {
		dec.Outcome, dec.Reason = OutcomeUnknownKind, model.NodeProtocol
		d.remember(ctx, q, model.NodeProtocol, "no transport driver for the runner's transport kind")
		return
	}
	e := d.claim(q.Seq)
	if e == nil {
		dec.Outcome = OutcomeFailed // the caller left, or the dispatcher closed
		return
	}
	defer d.release(e)
	cpu, mem := capacity(r, q.Job, local)
	slots, known := slotsOf(r)
	host := d.cfg.Host
	if !local {
		host = r.ID
	}
	req := Request{AttemptID: q.Job.AttemptID, Placement: p, Runner: r, Job: q.Job.Sched, Local: local, Host: host, CapacityCPU: cpu, CapacityMemMB: mem,
		Limits: pr.world.Policy.Limits, RunnerSlots: slots, SlotsKnown: known, WorldDigest: dec.WorldDigest, Incarnation: d.incarnation(), LeaseTTL: d.cfg.LeaseTTL}
	if d.cfg.ProviderLimit != nil {
		req.ProviderLimit = d.cfg.ProviderLimit(r.Provider)
	}
	a, err := Admit(ctx, d.cfg.Store, req)
	var ref *Refusal
	switch {
	case err == nil:
		for _, o := range runs {
			o.world.Apply(p)
		}
		dec.Outcome = OutcomeAdmitted
		if !local {
			d.wg.Add(1)
		}
		d.finish(q.Seq, a, nil)
		if !local {
			d.deliver(q, r, a)
		}
	case errors.Is(err, ErrConflict):
		dec.Outcome = OutcomeConflict
		d.healthy(q)
	case errors.As(err, &ref):
		dec.Outcome, dec.Reason = OutcomeRefused, ref.Reason
		d.note(q, ref.Reason) // Admit stored the reason
		d.healthy(q)
	default:
		d.retry(&dec.Outcome, q, err)
	}
}

// waiting settles a job Place did not place.
func (d *Dispatcher) waiting(ctx context.Context, q Queued, dec *Decision) {
	x := dec.Explanation
	switch x.Status {
	case "unplaceable":
		dec.Outcome = OutcomeUnplaceable
		if q.Job.Foreground {
			d.finish(q.Seq, Admitted{}, fail(CodeUnplaceable, "%s", unplaceableLine(q.Job, x)))
			return
		}
	case "awaiting-approval":
		dec.Outcome = OutcomeAwaitingAppr
	default:
		dec.Outcome = OutcomeWaiting
	}
	dec.Reason = waitReason(x)
	if dec.Reason != "" {
		d.remember(ctx, q, dec.Reason, "")
	}
}

// note remembers the reason a job waits and reports whether it changed.
func (d *Dispatcher) note(q Queued, reason model.PlacementReason) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range d.queue {
		if c.queued.Seq == q.Seq && c.reason != reason {
			c.reason = reason
			return true
		}
	}
	return false
}

// remember stores the reason a job is still queued, once per change. The
// in-memory copy is kept only when the write succeeded, so a failed write is
// retried on the next tick.
func (d *Dispatcher) remember(ctx context.Context, q Queued, reason model.PlacementReason, detail string) {
	d.mu.Lock()
	var e *entry
	for _, c := range d.queue {
		if c.queued.Seq == q.Seq {
			e = c
		}
	}
	changed := e != nil && e.reason != reason
	d.mu.Unlock()
	if !changed {
		return
	}
	if err := d.cfg.Store.RecordRefusal(context.WithoutCancel(ctx), q.Job.AttemptID, string(reason), detail); err == nil {
		d.note(q, reason)
	}
}

// healthy clears a job's run of store errors: the store answered.
func (d *Dispatcher) healthy(q Queued) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range d.queue {
		if c.queued.Seq == q.Seq {
			c.fails = 0
		}
	}
}

// retryLimit is how many consecutive store errors fail a job; a busy
// database is retried on the next tick.
const retryLimit = 3

// retry counts a store error against the job and fails it at the limit.
func (d *Dispatcher) retry(outcome *string, q Queued, err error) {
	d.mu.Lock()
	n := 0
	for _, c := range d.queue {
		if c.queued.Seq == q.Seq {
			c.fails++
			n = c.fails
		}
	}
	d.mu.Unlock()
	*outcome = OutcomeRetry
	if n >= retryLimit {
		*outcome = OutcomeFailed
		d.finish(q.Seq, Admitted{}, err)
	}
}
