package agent

// Purpose: the delivery bookkeeping of the pull driver: the per-node lease queue read by the long-poll
//          (agentapi.LeaseSource), the route table the sink uses, the liveness check and the cancel path.
// Inputs:  Run (enqueue, release, cancelRun, leaseGone), the long-poll (Next).
// Outputs: leases for Next; routes for the sink; E656 errors.
// Constraints: one mutex guards the tables and no I/O runs under it; a cancel that finds no control stream is
//          kept and retried on the node's next long-poll, so a cancel issued while the agent was
//          disconnected still reaches it (the epoch bump makes the agent stop on reconnect either way); the
//          finished-lease memory is bounded.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// maxFinished bounds the memory of leases whose Run returned (late frames of them are accepted and dropped).
const maxFinished = 4096

// enqueue registers the route and queues the lease for the node.
func (d *Driver) enqueue(rt *route, l protocol.Lease) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.routes == nil {
		d.routes, d.queues, d.finished, d.pending = map[string]*route{}, map[string][]protocol.Lease{}, map[string]bool{}, map[string]map[string]string{}
	}
	if _, dup := d.routes[rt.leaseID]; dup {
		return fail(ReasonDuplicate, "lease %s is already being delivered", rt.leaseID)
	}
	if len(d.queues[rt.node]) >= maxQueued {
		return fail(ReasonQueueFull, "node %s has %d undelivered leases", rt.node, len(d.queues[rt.node]))
	}
	delete(d.finished, rt.leaseID)
	d.routes[rt.leaseID] = rt
	d.queues[rt.node] = append(d.queues[rt.node], l)
	return nil
}

// release forgets a route and remembers the lease as finished.
func (d *Driver) release(rt *route) {
	d.mu.Lock()
	delete(d.routes, rt.leaseID)
	q := d.queues[rt.node][:0:0]
	for _, l := range d.queues[rt.node] {
		if l.LeaseID != rt.leaseID {
			q = append(q, l)
		}
	}
	d.queues[rt.node] = q
	if p := d.pending[rt.node]; p != nil {
		delete(p, rt.leaseID)
	}
	if len(d.order) >= maxFinished {
		delete(d.finished, d.order[0])
		d.order = d.order[1:]
	}
	d.finished[rt.leaseID] = true
	d.order = append(d.order, rt.leaseID)
	lw := rt.lw
	d.mu.Unlock()
	if lw != nil {
		_ = lw.Close()
	}
}

// Next hands the node its oldest queued lease, or nil. It is agentapi.LeaseSource.
func (d *Driver) Next(_ context.Context, nodeID string) (*protocol.Lease, error) {
	d.retryCancels(nodeID)
	d.mu.Lock()
	defer d.mu.Unlock()
	q := d.queues[nodeID]
	if len(q) == 0 {
		return nil, nil
	}
	l := q[0]
	d.queues[nodeID] = q[1:]
	return &l, nil
}

// leaseGone says whether the lease can no longer produce a result: it expired, or its attempt ended.
func (d *Driver) leaseGone(ctx context.Context, rt *route) (bool, string) {
	l, err := d.cfg().leases.LeaseGet(ctx, rt.leaseID)
	switch {
	case isStale(err):
		return true, "unknown to the store"
	case err != nil:
		d.logf("agent transport: lease %s: %v", rt.leaseID, err)
		return false, ""
	case terminal[l.State]:
		return true, "attempt " + string(l.State)
	}
	return false, ""
}

// cancelRun stops the job: it asks the agent to cancel and waits for its result for the grace period.
func (d *Driver) cancelRun(rt *route) error {
	const reason = "cancelled"
	if cancel := d.cfg().cancel; cancel == nil || cancel(rt.node, rt.leaseID, reason) != nil {
		d.mu.Lock()
		if d.pending[rt.node] == nil {
			d.pending[rt.node] = map[string]string{}
		}
		d.pending[rt.node][rt.leaseID] = reason
		d.mu.Unlock()
	}
	timer := time.NewTimer(d.grace())
	defer timer.Stop()
	select {
	case <-rt.done:
		return nil
	case <-timer.C:
		return fail(ReasonCancelLate, "node %s did not report lease %s within %s of the cancel", rt.node, rt.leaseID, d.grace())
	}
}

// retryCancels re-sends the cancels that found no control stream; the node is polling, so it is connected.
func (d *Driver) retryCancels(nodeID string) {
	cancel := d.cfg().cancel
	if cancel == nil {
		return
	}
	d.mu.Lock()
	todo := make(map[string]string, len(d.pending[nodeID]))
	for id, why := range d.pending[nodeID] {
		todo[id] = why
	}
	d.mu.Unlock()
	for id, why := range todo {
		if cancel(nodeID, id, why) != nil { // no control stream yet: try again on the next poll
			continue
		}
		d.mu.Lock()
		delete(d.pending[nodeID], id)
		d.mu.Unlock()
	}
}
