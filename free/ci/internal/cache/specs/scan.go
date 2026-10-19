package specs

// Purpose: pass a byte stream through the registered-secret scan before any of
// it reaches the blob store (R-SEC-18: scan before commit, Epic CACHE D5).
// Inputs:  a downstream writer and an authz.ValueScanner.
// Outputs: scanWriter, ErrSecretMatch.
// Constraints: bytes are released downstream only after the window holding them
// was scanned; windows are 1 MiB with a 1 MiB overlap, so a value that spans
// two windows is found. A value (in any encoding) longer than the overlap that
// straddles a window edge is not (the same documented bound as artifact scans).
// A nil scanner refuses everything. The scanned stream is the uncompressed tar,
// so compression cannot hide a value.

import (
	"errors"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

const (
	scanWindow  = 1 << 20
	scanOverlap = 1 << 20
)

// ErrSecretMatch means the stream holds a registered secret value, or no
// scanner was available to say it does not. It carries no value or offset.
var ErrSecretMatch = errors.New("specs: the cache holds a registered secret value")

type scanWriter struct {
	dst     io.Writer
	sc      authz.ValueScanner
	pending []byte
	tail    []byte
	err     error
}

func newScanWriter(dst io.Writer, sc authz.ValueScanner) *scanWriter {
	return &scanWriter{dst: dst, sc: sc}
}

func (w *scanWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.pending = append(w.pending, p...)
	for len(w.pending) >= scanWindow {
		if w.err = w.release(w.pending[:scanWindow]); w.err != nil {
			return 0, w.err
		}
		w.pending = w.pending[scanWindow:]
	}
	return len(p), nil
}

// Close scans and releases the last window; it does not close dst.
func (w *scanWriter) Close() error {
	if w.err == nil && len(w.pending) > 0 {
		w.err = w.release(w.pending)
		w.pending = nil
	}
	return w.err
}

func (w *scanWriter) release(chunk []byte) error {
	if w.sc == nil {
		return ErrSecretMatch
	}
	window := append(append(make([]byte, 0, len(w.tail)+len(chunk)), w.tail...), chunk...)
	if len(w.sc.Scan(window)) != 0 {
		return ErrSecretMatch
	}
	w.tail = append(w.tail[:0], window[max(0, len(window)-scanOverlap):]...)
	_, err := w.dst.Write(chunk)
	return err
}
