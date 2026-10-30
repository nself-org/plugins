package ssh

// Purpose: the coordinator end of the reverse blob path on carrier A (P7-CACHE-21,
// Epic CACHE D7): after the lease is sent, a Sink that owns the lease's cache plan
// (transport.Restorer) pushes the restore blobs as digest and chunk frames, and the
// node's offset answers are routed back to it.
// Inputs:  the node's `offset` frames for this lease and epoch.
// Outputs: `digest` and `chunk` frames fenced to the lease and epoch.
// Constraints: the push runs beside the serve loop and ends with it (the context is
// cancelled when the lease ends or is cancelled); an offset for another lease or
// epoch is dropped; the answer channel never blocks the read loop (a full channel
// drops the oldest answer, the newest one is the node's current position).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// offsetBuffer is how many unread node answers a session keeps.
const offsetBuffer = 64

type restoreLink struct{ s *sess }

func (l restoreLink) Digest(d protocol.Digest) error  { return l.s.send("digest", d, true) }
func (l restoreLink) Chunk(c protocol.Chunk) error    { return l.s.send("chunk", c, true) }
func (l restoreLink) Offsets() <-chan protocol.Offset { return l.s.offsets }

var _ transport.RestoreLink = restoreLink{}

// startRestore begins the push when the Sink has restore blobs; the returned func stops it.
func (s *sess) startRestore(ctx context.Context) func() {
	r, ok := s.sink.(transport.Restorer)
	if !ok {
		return func() {}
	}
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = r.Restore(rctx, restoreLink{s}) // the Sink audits a refusal; the node runs cold
	}()
	return func() { cancel(); <-done }
}

// onOffset hands a node answer to the push.
func (s *sess) onOffset(o protocol.Offset) {
	for {
		select {
		case s.offsets <- o:
			return
		default:
		}
		select {
		case <-s.offsets:
		default:
		}
	}
}
