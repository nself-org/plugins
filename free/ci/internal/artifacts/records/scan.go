package records

// Purpose: stream a file to the blob store only after the registered-secret
// scan found nothing in it (R-SEC-18: scan before commit).
// Inputs:  a reader, an authz.ValueScanner, the per-file cap.
// Outputs: a reader that hands out bytes only once they were scanned.
// Constraints: windows of 16 MiB with a 4 MiB overlap, so a value that spans
// two windows is found; a value (in any encoding) longer than the overlap that
// straddles a window edge is not. internal/redact has no bound on a registered
// value's length, so 4 MiB is the supported bound (debt row for a Register cap). The blob store
// publishes at EOF, so a match or an over-cap file aborts before any blob is
// visible. A nil scanner refuses everything.

import (
	"errors"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

const (
	scanWindow  = 16 << 20
	scanOverlap = 4 << 20
)

type scanReader struct {
	src     io.Reader
	sc      authz.ValueScanner
	name    string
	max     int64
	reason  string // the refusal when the stream outgrows max
	read    int64
	tail    []byte
	out     []byte
	err     error
	matched bool
}

func newScanReader(src io.Reader, sc authz.ValueScanner, name string, max int64) *scanReader {
	return &scanReader{src: src, sc: sc, name: name, max: max, reason: ReasonFileCap}
}

func (r *scanReader) Read(p []byte) (int, error) {
	for len(r.out) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		r.fill()
	}
	n := copy(p, r.out)
	r.out = r.out[n:]
	return n, nil
}

// fill scans the next window and releases its new bytes, or sets r.err.
func (r *scanReader) fill() {
	chunk := make([]byte, scanWindow)
	n, err := io.ReadFull(r.src, chunk)
	chunk = chunk[:n]
	switch {
	case err == nil:
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		r.err = io.EOF
	default:
		r.err = failf(ReasonUnreadable, "%s: %v", r.name, err)
		return
	}
	if r.read += int64(n); r.read > r.max {
		r.err = failf(r.reason, "%s grew past its byte budget %d B while it was read", r.name, r.max)
		return
	}
	if len(chunk) == 0 {
		return
	}
	if r.sc == nil {
		r.err, r.matched = failf(ReasonSecretMatch, "no secret scanner is registered; %s is not stored", r.name), true
		return
	}
	window := append(append(make([]byte, 0, len(r.tail)+len(chunk)), r.tail...), chunk...)
	if len(r.sc.Scan(window)) != 0 {
		r.err, r.matched = failf(ReasonSecretMatch, "%s holds a registered secret value; it is not stored", r.name), true
		return
	}
	keep := min(scanOverlap, len(window))
	r.tail = append(r.tail[:0], window[len(window)-keep:]...)
	r.out = chunk
}
