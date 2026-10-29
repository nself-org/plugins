package specs

// Purpose: pass a byte stream through the registered-secret scan before any of
// it reaches the blob store (R-SEC-18: scan before commit, Epic CACHE D5).
// Inputs:  a downstream writer and an authz.ValueScanner.
// Outputs: scanWriter, ErrSecretMatch.
// Constraints: bytes are released downstream only after the window holding them
// was scanned; windows are 1 MiB with a 1 MiB overlap, so a value that spans
// two windows is found as long as its longest encoded form fits the overlap:
// redact.Register caps a value at redact.MaxValueLen so that always holds.
// A nil scanner refuses everything. The scanned stream is the uncompressed tar;
// gzip payloads among its files are scanned inflated (inflate.go).

import (
	"errors"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

const (
	scanWindow  = 1 << 20
	scanOverlap = 1 << 20
)

// The widest encoded form of a registered value is 6 times its length (every
// byte as a \uXXXX escape); the overlap must hold 8 times the cap.
const _ = uint(scanOverlap - 8*redact.MaxValueLen)

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
	if !authz.CleanCandidates(w.sc, window) {
		return ErrSecretMatch
	}
	w.tail = append(w.tail[:0], window[max(0, len(window)-scanOverlap):]...)
	_, err := w.dst.Write(chunk)
	return err
}
