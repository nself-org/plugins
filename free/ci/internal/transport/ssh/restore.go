package ssh

// Purpose: carrier A side of the reverse blob path (Epic CACHE D7, P7-CACHE-21): when the
// Sink of a lease is a transport.Restorer, the node's `offset` requests for planned cache
// blobs are served over the same stdio session with digest and chunk frames.
// Inputs:  the node's offset frames, fenced to this lease and epoch.
// Outputs: digest and chunk frames (stream cache) and RestoreEvent calls on the Sink.
// Constraints: the frames are lease-scoped like every other frame of the session; a request
// from another epoch of this lease or after a cancel is refused with an event; the pusher
// stops with the session.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/transport/restore"
)

// frameUp sends the coordinator's transfer frames on the session.
type frameUp struct{ s *sess }

func (u frameUp) Digest(d protocol.Digest) error { return u.s.send("digest", d, true) }
func (u frameUp) Chunk(c protocol.Chunk) error   { return u.s.send("chunk", c, true) }

// startRestore starts the pusher when the Sink serves a plan; the returned func stops it.
func (s *sess) startRestore(ctx context.Context) func() {
	r, ok := s.sink.(transport.Restorer)
	if !ok {
		return func() {}
	}
	p, err := restore.New(ctx, r, frameUp{s})
	if err != nil || !p.Planned() {
		return func() {}
	}
	s.restore = p
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); p.Run(rctx) }()
	return func() {
		p.Close()
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second): // a send stuck on a dead pipe ends when the session closes
		}
	}
}

// onOffset routes one offset frame of the node.
func (s *sess) onOffset(ctx context.Context, m protocol.Message, o protocol.Offset) {
	if s.restore == nil {
		return
	}
	switch {
	case s.scoped(m):
		s.restore.Offer(ctx, o)
	case m.LeaseID == s.lease.LeaseID && m.Epoch != nil:
		s.restore.Refuse(ctx, o.SHA256, restore.ReasonStaleEpoch)
	}
}
