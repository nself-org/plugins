package transport

// Purpose: the seam for the reverse blob path (Epic CACHE D7, P7-CACHE-21): a Sink
// that also implements Restorer names the cache blobs the coordinator may send to
// the node of the lease, and learns what happened to each.
// Inputs:  the lease's attested cache plan, read through the Sink.
// Outputs: RestoreBlobs (the planned blobs, each opened verified on demand) and
// RestoreEvent calls (a verified node copy, a refusal, a give-up).
// Constraints: the blob list is the lease plan and nothing else; a driver never
// sends a blob the Restorer did not list; events are advisory and never fail the lease.

import (
	"context"
	"io"
)

// RestoreBlob is one blob of the lease plan.
type RestoreBlob struct {
	SHA256 string // lower-case hex
	Size   int64
	// Open returns the verified bytes from the start, or an error when the lease is no longer current.
	Open func() (io.ReadCloser, error)
}

// Event kinds of a restore.
const (
	// RestoreCopied: the node confirmed a verified copy of the blob.
	RestoreCopied = "copied"
	// RestoreRefused: a request outside the plan, from a stale epoch or after a cancel.
	RestoreRefused = "refused"
	// RestoreFailed: the transfer gave up (the node falls back to a cold miss).
	RestoreFailed = "failed"
)

// RestoreEvent reports one step of a restore to the Restorer.
type RestoreEvent struct {
	Kind   string
	SHA256 string
	Reason string
	Bytes  int64
}

// Restorer is implemented by a Sink that serves cache blobs to its node.
type Restorer interface {
	RestoreBlobs(ctx context.Context) ([]RestoreBlob, error)
	RestoreEvent(ctx context.Context, e RestoreEvent)
}
