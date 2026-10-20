package session

// Purpose: the coordinator-to-node half of a blob transfer (Epic CACHE D7, P7-CACHE-21): the
// `digest` and `chunk{stream: cache}` frames of the current lease reach the job hook that asked
// for the blob, which answers with `offset` frames through the Uploader.
// Inputs:  digest and chunk frames from the carrier.
// Outputs: Transfer.In, a bounded channel of the frames for one lease.
// Constraints: a frame is routed only when its envelope names a running lease and the lease's
// epoch, and only for the cache stream; a full channel drops the frame (the hook's receiver
// asks the sender for the position it expects, so a drop is a resume, never a hole); the
// session loop never blocks on a hook.

import "github.com/nself-org/plugins/free/ci/internal/protocol"

// inboundBuffer is how many transfer frames one lease holds before it drops.
const inboundBuffer = 32

// StreamCache is the transfer stream of cache blobs.
const StreamCache = "cache"

// Transfer is what a BeforeTransfer hook receives for the lease it runs for.
type Transfer struct {
	// Up sends the hook's offset answers (and any other transfer frame).
	Up Uploader
	// In carries the coordinator's digest and chunk frames of this lease, in arrival order; the
	// Body of each message is a *protocol.Digest or *protocol.Chunk.
	In <-chan protocol.Message
}

// onInbound routes one digest or chunk frame to its lease.
func (s *Session) onInbound(m protocol.Message) {
	l, ok := s.active[m.LeaseID]
	if !ok || m.Epoch == nil || *m.Epoch != l.lease.Epoch || l.cancelled.Load() {
		return
	}
	switch b := m.Body.(type) {
	case *protocol.Digest:
		if b.Stream != StreamCache {
			return
		}
	case *protocol.Chunk:
		if b.Stream != StreamCache {
			return
		}
	default:
		return
	}
	select {
	case l.inbound <- m:
	default: // full: the receiver resyncs through its offset answer
	}
}
