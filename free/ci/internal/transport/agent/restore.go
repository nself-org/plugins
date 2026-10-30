package agent

// Purpose: the coordinator half of a cache restore over carrier B (P7-CACHE-21, Epic CACHE D7): once the
//          node reports the lease running, the run's Sink (transport.Restorer) pushes digest and chunk
//          frames down the node's control stream and reads the node's upstream offset answers.
// Inputs:  the run's Sink, the lease, the first heartbeat naming the lease, offset frames (OffsetHandler).
// Outputs: control-stream frames fenced to the lease and its epoch; Carrier.BlobRestore for the session.
// Constraints: the push starts once, on the first heartbeat of the lease (the node has the lease and its
//          feed by then), and ends with Run (its context is cancelled and the push is awaited); an offset
//          for a lease without a restore, from another node, or a full buffer drops the oldest answer.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

const offsetBuffer = 64

// BlobRestore says the pull carrier delivers the coordinator's digest and chunk frames (session.RestoreCarrier).
func (*Carrier) BlobRestore() bool { return true }

// SetPush installs the function that queues a frame on a node's control stream (agentapi.Server.Push).
func (d *Driver) SetPush(fn func(ctx context.Context, node string, m protocol.Message) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Push = fn
}

type restoreLink struct {
	ctx   context.Context
	push  func(context.Context, string, protocol.Message) error
	rt    *route
	epoch uint64
}

func (l restoreLink) send(typ string, body any) error {
	e := l.epoch
	return l.push(l.ctx, l.rt.node, protocol.Message{Type: typ, LeaseID: l.rt.leaseID, Epoch: &e, Body: body})
}
func (l restoreLink) Digest(d protocol.Digest) error  { return l.send("digest", d) }
func (l restoreLink) Chunk(c protocol.Chunk) error    { return l.send("chunk", c) }
func (l restoreLink) Offsets() <-chan protocol.Offset { return l.rt.offsets }

var _ transport.RestoreLink = restoreLink{}

// armRestore prepares the push of a Restorer sink; the returned func stops it and waits for it.
func (d *Driver) armRestore(ctx context.Context, rt *route, lease protocol.Lease, sink transport.Sink) func() {
	r, ok := sink.(transport.Restorer)
	push := d.pushFn()
	if !ok || push == nil {
		return func() {}
	}
	rt.offsets = make(chan protocol.Offset, offsetBuffer)
	rctx, cancel := context.WithCancel(ctx)
	var once sync.Once
	done := make(chan struct{})
	rt.arm = func() {
		once.Do(func() {
			go func() {
				defer close(done)
				// the Sink audits a refusal; the node runs cold
				if err := r.Restore(rctx, restoreLink{rctx, push, rt, lease.Epoch}); err != nil && rctx.Err() == nil {
					d.logf("agent transport: restore push for lease %s: %v", rt.leaseID, err)
				}
			}()
		})
	}
	return func() {
		cancel()
		once.Do(func() { close(done) })
		<-done
	}
}

func (d *Driver) pushFn() func(context.Context, string, protocol.Message) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Push
}

// noteRunning starts the restore push of a lease the node just reported.
func (d *Driver) noteRunning(leaseID string) {
	d.mu.Lock()
	rt := d.routes[leaseID]
	d.mu.Unlock()
	if rt != nil && rt.arm != nil {
		rt.arm()
	}
}

// Offset implements agentapi.OffsetHandler: the node's answer to the restore push.
func (h blobHandler) Offset(_ context.Context, node string, m protocol.Message) error {
	o, ok := m.Body.(*protocol.Offset)
	if !ok {
		return nil
	}
	h.d.mu.Lock()
	rt := h.d.routes[m.LeaseID]
	h.d.mu.Unlock()
	if rt == nil || rt.node != node || rt.offsets == nil {
		return nil
	}
	for {
		select {
		case rt.offsets <- *o:
			return nil
		default:
		}
		select {
		case <-rt.offsets:
		default:
		}
	}
}

var _ agentapi.OffsetHandler = blobHandler{}
