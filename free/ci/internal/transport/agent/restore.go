package agent

// Purpose: carrier B side of the reverse blob path (Epic CACHE D7, P7-CACHE-21): when the Sink of a
// lease is a transport.Restorer, the node's `offset` requests for planned cache blobs are served
// with digest and chunk frames on the node's control stream.
// Inputs:  the node's offset frames (agentapi has already fenced them to the lease and epoch).
// Outputs: digest and chunk frames through Driver.Push, RestoreEvent calls on the Sink.
// Constraints: a request for another epoch of the lease or after a cancel is refused with an event;
// a driver without a Push function serves no restore (the node falls back to a cold miss); the
// pusher ends with the run.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/transport/restore"
)

// frameUp sends the coordinator's transfer frames for one lease through the node's control stream.
type frameUp struct {
	ctx  context.Context
	push func(context.Context, string, protocol.Message) error
	rt   *route
}

func (u frameUp) frame(kind string, body any) error {
	e := u.rt.epoch
	return u.push(u.ctx, u.rt.node, protocol.Message{Type: kind, LeaseID: u.rt.leaseID, Epoch: &e, Body: body})
}
func (u frameUp) Digest(d protocol.Digest) error { return u.frame("digest", d) }
func (u frameUp) Chunk(c protocol.Chunk) error   { return u.frame("chunk", c) }

// attachRestore binds a pusher to rt when the sink serves a plan; the returned func stops it.
func (d *Driver) attachRestore(ctx context.Context, rt *route, push func(context.Context, string, protocol.Message) error) func() {
	r, ok := rt.sink.(transport.Restorer)
	if !ok || push == nil {
		return func() {}
	}
	rctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	p, err := restore.New(rctx, r, frameUp{ctx: rctx, push: push, rt: rt})
	if err != nil || !p.Planned() {
		cancel()
		return func() {}
	}
	rt.restore = p
	done := make(chan struct{})
	go func() { defer close(done); p.Run(rctx) }()
	return func() {
		p.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

// onOffset routes one offset frame of the node.
func (d *Driver) onOffset(ctx context.Context, rt *route, m protocol.Message, o protocol.Offset) {
	switch {
	case rt.restore == nil:
	case m.Epoch == nil || *m.Epoch != rt.epoch:
		rt.restore.Refuse(ctx, o.SHA256, restore.ReasonStaleEpoch)
	default:
		rt.restore.Offer(ctx, o)
	}
}

// Refused is agentapi.RefusalReporter: a restore request the listener turned away (stale epoch,
// denied token) before it reached Frame still leaves an audit row.
func (h blobHandler) Refused(ctx context.Context, node string, m protocol.Message, reason string) {
	o, ok := m.Body.(*protocol.Offset)
	if !ok {
		return
	}
	rt, late, err := (*hubSink)(h.d).route(m.LeaseID)
	if err != nil || late || rt.node != node || rt.restore == nil {
		return
	}
	why := restore.ReasonDenied
	if reason == agentapi.RefusedStale || m.Epoch == nil || *m.Epoch != rt.epoch { // the gate may name the stale epoch as a wrong job
		why = restore.ReasonStaleEpoch
	}
	rt.restore.Refuse(ctx, o.SHA256, why)
}
