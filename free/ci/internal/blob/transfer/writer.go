package transfer

// Purpose: the writer a Receiver hands to a carrier driver: it implements
// transport.BlobWriter plus the Abort and Verify extensions the drivers use
// (internal/transport/agent/blobs.go, internal/transport/ssh/blob.go).
// Inputs:  chunk bytes in order from the committed offset.
// Outputs: a blob published in the realm after the whole-blob digest matches.
// Constraints: Write refuses a stale epoch and a byte past the announced size;
// Verify and Close publish through cas.Partial.Finalize, which hashes every byte
// and removes the partial on a mismatch (E714 digest_mismatch), so nothing
// unverified is ever visible; Abort keeps the partial for a resume (the sweep
// removes it after 24 h).

import (
	"context"
	"errors"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// Writer is one open blob.
type Writer struct {
	r *Receiver
	h transport.BlobHeader
	p *cas.Partial

	mu        sync.Mutex
	offset    int64
	done      bool // published (or already held)
	published bool
}

// Offset is the number of bytes the receiver holds.
func (w *Writer) Offset() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.offset
}

// Write appends the next bytes of the blob.
func (w *Writer) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.done || w.p == nil {
		return 0, Failf(ReasonProtocol, "blob %s is already complete", short(w.h.SHA256))
	}
	if err := w.r.epoch(context.Background(), w.h.SHA256); err != nil {
		return 0, err
	}
	if w.offset+int64(len(b)) > w.h.Size {
		return 0, Failf(ReasonProtocol, "blob %s would exceed its announced %d B", short(w.h.SHA256), w.h.Size)
	}
	n, err := w.p.Write(b)
	w.offset += int64(n)
	if err != nil {
		return n, Failf(ReasonInterrupted, "blob %s could not be stored: %v", short(w.h.SHA256), err)
	}
	return n, nil
}

// publish verifies every byte and makes the blob visible. It is idempotent.
func (w *Writer) publish() error {
	if w.done {
		return nil
	}
	if w.p == nil {
		return Failf(ReasonProtocol, "blob %s was aborted", short(w.h.SHA256))
	}
	if w.offset != w.h.Size {
		return Failf(ReasonInterrupted, "blob %s holds %d of %d B", short(w.h.SHA256), w.offset, w.h.Size)
	}
	if err := w.r.epoch(context.Background(), w.h.SHA256); err != nil {
		return err
	}
	if err := w.p.Finalize(); err != nil {
		if errors.Is(err, cas.ErrPartialConflict) {
			w.offset = 0 // the partial is gone: a retry starts clean
			return Failf(ReasonDigest, "blob %s does not match its digest; nothing was published", short(w.h.SHA256))
		}
		return Failf(ReasonInterrupted, "blob %s could not be published: %v", short(w.h.SHA256), err)
	}
	w.done, w.published = true, true
	return nil
}

// Verify checks the whole blob against want (the announced digest) and publishes it when it matches.
func (w *Writer) Verify(want string) (resultErr error) {
	defer func() { w.r.notify(context.Background(), w.h.SHA256, resultErr) }()
	w.mu.Lock()
	defer w.mu.Unlock()
	if want != w.h.SHA256 {
		return Failf(ReasonDigest, "blob %s was asked to verify as %s", short(w.h.SHA256), short(want))
	}
	return w.publish()
}

// Close publishes the blob when it is whole and its digest matches.
func (w *Writer) Close() (resultErr error) {
	defer func() { w.r.notify(context.Background(), w.h.SHA256, resultErr) }()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.publish()
}

// Abort discards nothing on disk: the partial stays resumable. It marks the writer unusable.
func (w *Writer) Abort() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.done {
		w.p = nil
	}
	return nil
}

// Held reports whether the realm already held the blob when it was announced (no byte will follow).
func (w *Writer) Held() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.done && !w.published
}

// Published reports whether this writer made the blob visible (false for a duplicate).
func (w *Writer) Published() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.published
}

var _ transport.BlobWriter = (*Writer)(nil)
