package dispatch

// Purpose: the dispatcher's queue: entries, enqueue, the claim that keeps a
// lease from losing its owner, and how a waiter gives up.
// Constraints: every field of an entry is guarded by Dispatcher.mu; a result is
// written before done is closed, so a reader that saw done may read it.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type entry struct {
	queued   Queued
	done     chan struct{}
	res      Admitted
	err      error
	finished bool
	reason   model.PlacementReason
	fails    int // consecutive store errors while admitting
	// admitting is non-nil while a tick is committing this entry's lease; a
	// caller that gives up waits for it, so a lease never loses its owner.
	admitting chan struct{}
}

func (d *Dispatcher) enqueue(j Job) (*entry, error) { return d.enqueueCtx(context.Background(), j) }

// enqueueCtx queues j, or refuses it with E694 when its project's queue is full.
func (d *Dispatcher) enqueueCtx(ctx context.Context, j Job) (*entry, error) {
	if j.AttemptID == "" || j.Sched.Key == "" {
		return nil, fail(CodeUnplaceable, "a queued job needs an attempt and a key")
	}
	limits := d.queueLimits(ctx) // read before the lock: it may touch the policy
	e, depth, err := d.push(j, limits.MaxDepth)
	if err != nil {
		return nil, err
	}
	if e == nil { // the project's queue is full: refused before it was queued (E694)
		return nil, d.backpressure(ctx, j.Sched.Project, depth, limits.MaxDepth)
	}
	return e, nil
}

// push queues j unless its project already has max jobs queued (max <= 0 is no
// cap); it then returns a nil entry and the depth.
func (d *Dispatcher) push(j Job, max int64) (*entry, int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, 0, errClosed
	}
	var depth int64
	for _, e := range d.queue {
		if e.finished {
			continue
		}
		if e.queued.Job.Sched.Key == j.Sched.Key {
			return nil, 0, fail(CodeUnplaceable, "job key %q is already queued", j.Sched.Key)
		}
		if e.queued.Job.Sched.Project == j.Sched.Project {
			depth++
		}
	}
	if max > 0 && depth >= max {
		return nil, depth, nil
	}
	d.seq++
	e := &entry{queued: Queued{Job: j, EnqueuedAt: d.cfg.Clock(), Seq: d.seq}, done: make(chan struct{})}
	d.queue = append(d.queue, e)
	return e, depth, nil
}

// Wake makes the waiters tick now instead of at their next poll: the caller
// freed capacity (an attempt ended) that a queued job may be waiting for. On a
// tiny run the poll was a measured share of every hand-off (P7-CI-46).
func (d *Dispatcher) Wake() {
	d.mu.Lock()
	d.lastTick = time.Time{}
	ch := d.wake
	d.wake = nil
	d.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// wakeCh is the channel the next Wake closes.
func (d *Dispatcher) wakeCh() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.wake == nil {
		d.wake = make(chan struct{})
	}
	return d.wake
}

// due reports whether a waiter should tick: something was enqueued since the
// last tick, or Poll has passed.
func (d *Dispatcher) due() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.seq != d.ticked || d.cfg.Clock().Sub(d.lastTick) >= d.cfg.Poll
}

// drop removes an unfinished entry (its caller gave up). An entry a tick is
// committing, or committed just before the caller gave up, keeps its result:
// the lease exists and the caller must see it to cancel it.
func (d *Dispatcher) drop(e *entry, cause error) (Admitted, error) {
	for {
		d.mu.Lock()
		if e.finished {
			res, err := e.res, e.err
			d.compact()
			d.mu.Unlock()
			return res, err
		}
		if ch := e.admitting; ch != nil {
			d.mu.Unlock()
			<-ch
			continue
		}
		e.finished, e.err = true, cause
		close(e.done)
		d.compact()
		d.mu.Unlock()
		return Admitted{}, cause
	}
}

// claim marks the entry with sequence seq as being admitted; nil when its
// caller already left or the dispatcher is closed.
func (d *Dispatcher) claim(seq uint64) *entry {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	for _, e := range d.queue {
		if e.queued.Seq == seq && !e.finished {
			e.admitting = make(chan struct{})
			return e
		}
	}
	return nil
}

// release ends a claim, waking a caller that waits to give up.
func (d *Dispatcher) release(e *entry) {
	d.mu.Lock()
	ch := e.admitting
	e.admitting = nil
	d.mu.Unlock()
	close(ch)
}

// finish resolves the entry with sequence seq once.
func (d *Dispatcher) finish(seq uint64, a Admitted, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.queue {
		if e.queued.Seq == seq && !e.finished {
			e.finished, e.res, e.err = true, a, err
			close(e.done)
		}
	}
	d.compact()
}

func (d *Dispatcher) compact() {
	live := d.queue[:0]
	for _, e := range d.queue {
		if !e.finished {
			live = append(live, e)
		}
	}
	d.queue = live
}

// pending snapshots the unfinished entries.
func (d *Dispatcher) pending(now time.Time) []Queued {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ticked, d.lastTick = d.seq, now
	out := make([]Queued, 0, len(d.queue))
	for _, e := range d.queue {
		if !e.finished {
			out = append(out, e.queued)
		}
	}
	return out
}
