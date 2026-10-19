package redact

// Purpose: the value registry. Register adds a secret value (and, for a
// multi-line value, each line of at least MinValueLen bytes) in every encoding
// of encodings.go; the matchers are rebuilt lazily on the next use, once for a
// batch of Register calls (a stream that needs the new values waits for that one
// rebuild, which is what makes a value registered before a write apply to it).
// Inputs:  Register(value). Outputs: NewFilter, Wrap, Scan (stream.go, scan.go).
// Constraints: safe for concurrent Register and use; a value shorter than
// MinValueLen or longer than MaxValueLen is refused with E673 (never skipped); nothing here formats a
// value into an error or a log line.

import (
	"sync"
)

// Redactor holds registered values and masks them in streams. The zero value
// is not usable; use New.
type Redactor struct {
	mu    sync.RWMutex
	setA  map[string]struct{}
	setB  map[string]struct{}
	dirty bool
	snap  *snapshot
}

// snapshot is an immutable pair of automata (either may be nil).
type snapshot struct{ a, b *automaton }

// New returns an empty Redactor. Token shapes and the neutraliser work with no
// registered value, so it is useful before any secret exists.
func New() *Redactor {
	return &Redactor{setA: map[string]struct{}{}, setB: map[string]struct{}{}, snap: &snapshot{}}
}

// Register adds v. It returns an E673 *Error for a value under MinValueLen or
// over MaxValueLen bytes.
func (r *Redactor) Register(v []byte) error {
	if len(v) < MinValueLen {
		return errTooShort()
	}
	if len(v) > MaxValueLen {
		return errTooLong()
	}
	values := append([][]byte{v}, lineValues(v)...)
	values = append(values, pemParts(v)...)
	var as, bs []string
	for _, x := range values {
		a, b := valueForms(x)
		as, bs = append(as, toStrings(a)...), append(bs, toStrings(b)...)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range as {
		r.setA[s] = struct{}{}
	}
	for _, s := range bs {
		r.setB[s] = struct{}{}
	}
	r.dirty = true
	return nil
}

func toStrings(p [][]byte) []string {
	out := make([]string, 0, len(p))
	for _, b := range p {
		if len(b) > 0 {
			out = append(out, string(b))
		}
	}
	return out
}

func keys(m map[string]struct{}) [][]byte {
	out := make([][]byte, 0, len(m))
	for k := range m {
		out = append(out, []byte(k))
	}
	return out
}

func (r *Redactor) snapshot() *snapshot {
	r.mu.RLock()
	if !r.dirty {
		s := r.snap
		r.mu.RUnlock()
		return s
	}
	r.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dirty {
		r.snap = &snapshot{a: build(keys(r.setA)), b: build(keys(r.setB))}
		r.dirty = false
	}
	return r.snap
}

// values returns the spans of registered values in b and the number of
// trailing bytes that could still begin a match.
func (s *snapshot) values(b []byte) (spans []span, hold int) {
	if s.a != nil {
		spans, hold = s.a.find(b, nil)
	}
	if s.b != nil {
		var h int
		spans, h = s.b.findSkip(b, spans)
		if h > hold {
			hold = h
		}
	}
	return spans, hold
}
