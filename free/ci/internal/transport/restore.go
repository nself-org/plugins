package transport

// Purpose: the seams of the reverse blob path (Epic CACHE D7, P7-CACHE-21): the
// coordinator pushes the cache blobs a lease restores to the node, and the node
// session hands those frames to the job hook that is receiving them.
// Inputs:  a Sink that can open a lease's planned restore blobs (Restorer), and
// the digest and chunk frames a node session receives (BlobFeed).
// Outputs: RestoreLink for a driver to give the Sink, BlobFrame for the node.
// Constraints: a carrier that cannot send coordinator frames to a node never
// calls Restore, and its node session never offers a feed, so a cold restore
// degrades at once instead of waiting for frames that cannot arrive.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// RestoreLink is the coordinator's end of one lease's reverse blob path, bound
// to the lease and epoch of the session (a frame it sends carries both).
type RestoreLink interface {
	Digest(protocol.Digest) error
	Chunk(protocol.Chunk) error
	// Offsets carries the node's offset answers for this lease.
	Offsets() <-chan protocol.Offset
}

// Restorer is implemented by a Sink that owns the cache plan of its lease. A
// driver calls Restore once, after the lease frame is sent, on its own goroutine;
// the error is the Sink's to audit, the job never waits for it.
type Restorer interface {
	Restore(ctx context.Context, link RestoreLink) error
}

// BlobFrame is one coordinator-to-node blob frame: exactly one field is set.
type BlobFrame struct {
	Digest *protocol.Digest
	Chunk  *protocol.Chunk
}

// BlobFeed is the node session's end of a lease's reverse blob path.
type BlobFeed interface {
	// Frames carries the lease's digest and chunk frames in arrival order.
	Frames() <-chan BlobFrame
	// Offset answers the coordinator.
	Offset(protocol.Offset) error
	// Close tells the session nobody reads Frames any more; later frames are dropped.
	Close()
}
