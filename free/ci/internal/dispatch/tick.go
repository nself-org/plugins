package dispatch

// Purpose: one pipeline's share of a tick: build its world, place its queued
// jobs in order, admit each placement through Admit, and settle the jobs that
// were not admitted (E618 for a foreground job nothing can run, E693 past
// max_queue, a recorded reason otherwise).
// Inputs:  the pipeline's queued jobs in Orderer order.
// Outputs: waiters resolved; decisions sent to the sink; refusal reasons stored
// for `ci explain`.
// Constraints: one world per pipeline, because the policy is per pipeline;
// admission keeps Place's order and each commit is applied to the world copy
// with World.Apply; a placement is re-checked by the admission transaction, so
// a world that went stale between Build and Admit cannot oversubscribe.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func (d *Dispatcher) tickGroup(ctx context.Context, group []Queued, now time.Time) error {
	w, digest, err := d.cfg.World(ctx, group[0].Job.Sched.Pipeline)
	if err != nil {
		for _, q := range group {
			if q.Job.Foreground {
				d.finish(q.Seq, Admitted{}, err)
			}
		}
		return err
	}
	res := place(w, group)
	placed := map[string]sched.Placement{}
	for _, p := range res.Placements {
		placed[p.JobKey] = p
	}
	explained := map[string]sched.Explanation{}
	for _, x := range res.Explanations {
		explained[x.JobKey] = x
	}
	for _, q := range group {
		key := q.Job.Sched.Key
		dec := Decision{WorldDigest: digest, Job: q.Job.Sched, Explanation: explained[key]}
		if p, ok := placed[key]; ok {
			dec.Placement = &p
			d.admit(ctx, &w, q, p, &dec, now)
		} else {
			d.waiting(ctx, q, &dec)
		}
		d.cfg.Sink.Record(ctx, dec)
		d.expire(ctx, q, w.Policy.Overrides.MaxQueueMs, &dec, now)
	}
	return nil
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

// admit sends one placement through the chokepoint and settles the job.
func (d *Dispatcher) admit(ctx context.Context, w *sched.World, q Queued, p sched.Placement, dec *Decision, now time.Time) {
	r, ok := runnerOf(w, p.RunnerID)
	local := ok && r.Location == "local"
	if !ok || (!local && !d.knownTransport(r)) {
		dec.Outcome, dec.Reason = OutcomeUnknownKind, model.NodeProtocol
		d.remember(ctx, q, model.NodeProtocol, "no transport driver for the runner's transport kind")
		return
	}
	cpu, mem := capacity(r, q.Job, local)
	host := d.cfg.Host
	if !local {
		host = r.ID
	}
	req := Request{AttemptID: q.Job.AttemptID, Placement: p, Runner: r, Job: q.Job.Sched, Local: local, Host: host, CapacityCPU: cpu, CapacityMemMB: mem,
		Limits: w.Policy.Limits, RunnerSlots: slotsOf(r), Foreground: q.Job.Foreground, WorldDigest: dec.WorldDigest, Incarnation: d.incarnation(), LeaseTTL: d.cfg.LeaseTTL}
	if d.cfg.ProviderLimit != nil {
		req.ProviderLimit = d.cfg.ProviderLimit(r.Provider)
	}
	a, err := Admit(ctx, d.cfg.Store, req)
	var ref *Refusal
	switch {
	case err == nil:
		w.Apply(p)
		dec.Outcome = OutcomeAdmitted
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

// waitReason is the reason `ci explain` shows for a job that waits: the first
// specific reason of the first runner that has one, else the job-level reason.
func waitReason(x sched.Explanation) model.PlacementReason {
	for _, a := range x.Alternatives {
		for _, r := range a.Reasons {
			if r.Code != string(model.CapacityBusy) {
				return model.PlacementReason(r.Code)
			}
		}
	}
	if len(x.Reasons) > 0 {
		return model.PlacementReason(x.Reasons[0].Code)
	}
	for _, a := range x.Alternatives {
		if len(a.Reasons) > 0 {
			return model.PlacementReason(a.Reasons[0].Code)
		}
	}
	return ""
}

// expire ends a job that waited past max_queue (E693, queue.timeout).
func (d *Dispatcher) expire(ctx context.Context, q Queued, maxQueueMs int64, dec *Decision, now time.Time) {
	if dec.Outcome == OutcomeAdmitted || dec.Outcome == OutcomeFailed {
		return
	}
	limit := maxQueueMs
	if limit <= 0 && q.Job.Foreground {
		limit = defaultForegroundQueueMs
	}
	waited := now.Sub(q.EnqueuedAt)
	if limit <= 0 || waited < time.Duration(limit)*time.Millisecond {
		return
	}
	d.remember(ctx, q, model.QueueTimeout, "")
	dec.Outcome, dec.Reason = OutcomeTimeout, model.QueueTimeout
	d.finish(q.Seq, Admitted{}, fail(CodeQueueTimeout, "job %s waited %s in the queue (max_queue %s); queue.timeout", q.Job.Sched.Name, waited.Round(time.Second), (time.Duration(limit)*time.Millisecond).Round(time.Second)))
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

// remember stores the reason a job is still queued, once per change.
func (d *Dispatcher) remember(ctx context.Context, q Queued, reason model.PlacementReason, detail string) {
	if d.note(q, reason) {
		_ = d.cfg.Store.RecordRefusal(context.WithoutCancel(ctx), q.Job.AttemptID, string(reason), detail)
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

func (d *Dispatcher) incarnation() int64 {
	if d.cfg.Incarnation != nil {
		return d.cfg.Incarnation()
	}
	return 1
}

func runnerOf(w *sched.World, id string) (sched.Runner, bool) {
	for _, r := range w.Runners {
		if r.ID == id {
			return r, true
		}
	}
	for _, r := range w.Hosted {
		if r.ID == id {
			return r, true
		}
	}
	return sched.Runner{}, false
}

func slotsOf(r sched.Runner) int {
	if s := r.Capability.Availability.Slots.Value; s != nil {
		return *s
	}
	return 0
}

// capacity is the host capacity a placement reserves against.
func capacity(r sched.Runner, j Job, local bool) (cpu, mem int) {
	if v := r.Capability.Resources.CPU.Value; v != nil {
		cpu = int(*v)
	}
	if v := r.Capability.Resources.MemMB.Value; v != nil {
		mem = int(*v)
	}
	if local {
		if j.LocalCPU > 0 {
			cpu = j.LocalCPU
		}
		if j.LocalMemMB > 0 {
			mem = j.LocalMemMB
		}
	}
	return cpu, mem
}

// unplaceableLine is the one human reason line of an E618 (X17).
func unplaceableLine(j Job, x sched.Explanation) string {
	var parts []string
	for i, a := range x.Alternatives {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more runners", len(x.Alternatives)-3))
			break
		}
		codes := make([]string, 0, len(a.Reasons))
		for _, r := range a.Reasons {
			codes = append(codes, r.Code)
		}
		parts = append(parts, fmt.Sprintf("%s: %s", a.RunnerID, strings.Join(codes, ",")))
	}
	if len(parts) == 0 {
		for _, r := range x.Reasons {
			parts = append(parts, r.Code)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "no runner is registered")
	}
	return fmt.Sprintf("job %s cannot run on any runner (%s)", j.Sched.Name, strings.Join(parts, "; "))
}
