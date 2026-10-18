package redact

// Purpose: the streaming redactor. A Filter holds a small carry-over window so
// a value, token shape, workflow command or escape sequence split across
// writes is still caught; only bytes that cannot start a match are released.
// Inputs:  Redact(chunk), Flush; or Wrap(w) for an io.WriteCloser.
// Outputs: the masked chunk. A Filter satisfies exec.Redactor structurally.
// Constraints: memory per stream is O(longest encoded value + 8 KiB token cap);
// a PEM body is dropped as it streams, never buffered; one Filter belongs to one
// stream and is not safe for concurrent use (Wrap adds the lock). Chunking never
// changes the output, except that two adjacent masks may print as one or two.

import (
	"errors"
	"io"
	"sort"
	"sync"
)

// Placeholder replaces every masked span.
const Placeholder = "[REDACTED]"

// pemTail is how many bytes are kept while looking for the end of a PEM block.
const pemTail = 64

// Filter is the per-stream state of a Redactor.
type Filter struct {
	r     *Redactor
	buf   []byte
	prev  byte
	inPEM bool
	mask  int // end of a printed mask inside buf, or -1 (see release)
}

// NewFilter starts a stream. Values registered later apply to bytes not yet released.
func (r *Redactor) NewFilter() *Filter { return &Filter{r: r, mask: -1} }

// Redact masks one chunk and returns the bytes that are safe to release now.
func (f *Filter) Redact(p []byte) []byte { return f.process(p, false) }

// Flush releases everything still held; call it once at the end of the stream.
func (f *Filter) Flush() []byte { return f.process(nil, true) }

func (f *Filter) process(p []byte, final bool) []byte {
	f.buf = append(f.buf, p...)
	var out []byte
	defer f.shrink()
	for {
		if f.inPEM && !f.dropPEM(final) {
			return out
		}
		var again bool
		out, again = f.release(out, final)
		if !again {
			return out
		}
	}
}

// shrink returns buffer capacity left over from one large write: the carry-over
// is small, the backing array must not stay as big as the largest chunk.
func (f *Filter) shrink() {
	if cap(f.buf) > 64<<10 && cap(f.buf) > 8*len(f.buf)+(16<<10) {
		f.buf = append(make([]byte, 0, len(f.buf)+4096), f.buf...)
	}
}

// dropPEM discards the body of an open PEM block; it reports true once the
// END line has been seen and ordinary processing resumes.
func (f *Filter) dropPEM(final bool) bool {
	if end, ok := pemClose(f.buf, 0); ok {
		f.prev = f.buf[end-1]
		n := copy(f.buf, f.buf[end:])
		f.buf, f.inPEM = f.buf[:n], false
		return true
	}
	if final {
		f.buf, f.inPEM, f.mask, f.prev = f.buf[:0], false, -1, 0
	} else if len(f.buf) > pemTail {
		n := copy(f.buf, f.buf[len(f.buf)-pemTail:])
		f.buf = f.buf[:n]
	}
	return false
}

// piece is one replacement in the released text.
type piece struct {
	s, e int
	with string
}

// release scans the buffer, appends the releasable prefix to out and keeps the
// rest. again is true when a PEM block just opened and the loop must continue.
//
// Bytes within `hold` of the end may begin a match and stay in the buffer. A
// mask that reaches past the release point also stays, flagged by f.mask (the
// buffer offset where the printed mask ends), so a later match that overlaps it
// extends the same mask instead of printing a second one.
func (f *Filter) release(out []byte, final bool) (res []byte, again bool) {
	b := f.buf
	if len(b) == 0 {
		return out, false
	}
	raw, vhold := f.r.snapshot().values(b)
	shp, shold, pemAt := findShapes(b, f.prev, final, nil)
	neu, nhold := neutralise(b, f.prev, final, nil)
	raw = append(raw, shp...)
	if pemAt >= 0 {
		// A registered value that contains the PEM block (a one-line key file) may
		// begin before the header: its prefix is part of the mask.
		raw = append(raw, span{min(pemAt, len(b)-vhold), len(b)})
	}
	r := len(b)
	if pemAt < 0 && !final {
		r -= max(vhold, shold, nhold)
		for _, e := range neu {
			if e.s < r && r < e.e {
				r = e.s
			}
		}
	}
	if f.mask >= 0 {
		raw = append(raw, span{0, f.mask})
	}
	merged := mergeSpans(raw)
	if r > 0 {
		out, f.mask = assemble(out, b[:r], merged, neu, f.mask >= 0, r)
	}
	if r > 0 {
		f.prev = b[r-1]
	}
	if pemAt >= 0 {
		he, _, _ := pemHeader(b, pemAt, pemBeg) // the header's own dashes are never an END marker
		keep := b[max(he, len(b)-pemTail):]
		n := copy(f.buf, keep)
		f.buf, f.inPEM, f.mask = f.buf[:n], true, -1
		return out, true
	}
	n := copy(f.buf, b[r:])
	f.buf = f.buf[:n]
	return out, false
}

// assemble writes b (the releasable prefix) with masks and neutralised text.
// cont says the first merged span continues a mask that is already printed. It
// returns the offset, relative to the retained buffer, where the last mask ends
// (-1 when it ended before r).
func assemble(out, b []byte, merged []span, neu []edit, cont bool, r int) ([]byte, int) {
	var pieces []piece
	end := -1
	for i, m := range merged {
		if m.s >= len(b) {
			break
		}
		with := Placeholder
		if i == 0 && cont && m.s == 0 {
			with = ""
		}
		pieces = append(pieces, piece{m.s, min(m.e, len(b)), with})
		end = m.e
	}
	for _, e := range neu {
		if e.e > len(b) {
			break
		}
		i := sort.Search(len(merged), func(k int) bool { return merged[k].e > e.s })
		if i < len(merged) && merged[i].s < e.e {
			continue
		}
		pieces = append(pieces, piece{e.s, e.e, e.with})
	}
	sort.Slice(pieces, func(i, j int) bool { return pieces[i].s < pieces[j].s })
	pos := 0
	for _, p := range pieces {
		out = append(out, b[pos:p.s]...)
		out = append(out, p.with...)
		pos = p.e
	}
	out = append(out, b[pos:]...)
	if end >= r {
		return out, end - r
	}
	return out, -1
}

// Wrap returns a writer that masks everything written through it before it
// reaches w. Close flushes the held bytes; it does not close w.
func (r *Redactor) Wrap(w io.Writer) io.WriteCloser {
	return &writer{w: w, f: r.NewFilter()}
}

type writer struct {
	mu sync.Mutex
	w  io.Writer
	f  *Filter
}

func (x *writer) Write(p []byte) (int, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if out := x.f.Redact(p); len(out) > 0 {
		if _, err := x.w.Write(out); err != nil {
			return 0, x.mask(err)
		}
	}
	return len(p), nil
}

func (x *writer) Close() error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if out := x.f.Flush(); len(out) > 0 {
		if _, err := x.w.Write(out); err != nil {
			return x.mask(err)
		}
	}
	return nil
}

// maskedError is a sink error whose text has been through the redactor, so a
// value the sink echoes (a path, a URL) never leaves this package in an error.
type maskedError struct {
	msg string
	err error
}

func (e *maskedError) Error() string { return e.msg }

// Is keeps errors.Is working for sentinel sink errors without exposing the raw
// error (and any value inside its text) through errors.Unwrap.
func (e *maskedError) Is(target error) bool { return errors.Is(e.err, target) }

func (x *writer) mask(err error) error {
	f := x.f.r.NewFilter()
	msg := append(f.Redact([]byte(err.Error())), f.Flush()...)
	return &maskedError{msg: string(msg), err: err}
}
