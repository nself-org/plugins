package session

import (
	"bytes"
	"sort"
	"strings"
	"sync"
)

const (
	maskText   = "[redacted]"
	maxLogLine = 8 << 10
	// minLineMask is the shortest line of a multi-line secret that is masked on
	// its own (shorter ones, such as a closing brace, would mask ordinary text).
	minLineMask = 4
)

// lineWriter turns job output chunks into numbered log lines. The job's own
// redactor has already run; this is the second layer, masking the literal
// secret values the lease carried (runner credential, job token, secret frame
// values) before any line leaves the node. A line with no newline is cut only
// where no secret value straddles the cut, and the last (longest-1) bytes are
// held back so a value arriving in two chunks is still found whole.
type lineWriter struct {
	mu   sync.Mutex
	emit func(line string)
	// masks returns the literal values to hide; it is re-read per line so a
	// secret that arrives after the job started is covered.
	masks func() [][]byte
	buf   []byte
	// covered is how many leading bytes of buf already belong to a secret value
	// that was emitted as [redacted] when the line was cut inside a long run of
	// matches; they stay masked when the rest of the line is emitted.
	covered int
}

func newLineWriter(emit func(string), masks func() [][]byte) *lineWriter {
	return &lineWriter{emit: emit, masks: masks}
}

func (w *lineWriter) Write(p []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.take(i)
		w.buf = w.buf[1:] // the newline
		w.covered = max(w.covered-1, 0)
	}
	masks := w.maskList()
	longest := 0
	for _, m := range masks {
		longest = max(longest, len(m))
	}
	for len(w.buf) > maxLogLine+longest {
		// Emit all but the last (longest-1) bytes, which may begin a secret value
		// that finishes in a later chunk. Never cut inside a complete value. When
		// the whole buffer is one run of matches there is no boundary to cut at:
		// emit it masked and remember that the bytes kept back are already part
		// of a value, so a later completion of that value cannot show them.
		hold := max(longest-1, 0)
		cut, keep := len(w.buf)-hold, 0
		for _, r := range w.spans(masks) {
			if r[0] < cut && cut < r[1] {
				cut = r[0]
				if cut == 0 {
					cut = max(r[1]-hold, 1)
					keep = r[1] - cut
				}
				break
			}
		}
		w.take(cut)
		w.covered = max(w.covered, keep)
	}
}

// Flush emits the unterminated tail.
func (w *lineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.take(len(w.buf))
	}
}

// spans are the masked ranges of the buffer: value matches plus the covered prefix.
func (w *lineWriter) spans(masks [][]byte) [][2]int {
	sp := spans(w.buf, masks)
	if w.covered > 0 {
		sp = mergeSpans(append(sp, [2]int{0, min(w.covered, len(w.buf))}))
	}
	return sp
}

// take emits buf[:n] as one line, masked, and consumes it.
func (w *lineWriter) take(n int) {
	b := w.buf[:n]
	sp := spans(b, w.maskList())
	if w.covered > 0 {
		sp = mergeSpans(append(sp, [2]int{0, min(w.covered, n)}))
	}
	w.buf = w.buf[n:]
	w.covered = max(w.covered-n, 0)
	b = bytes.TrimSuffix(replaceSpans(b, sp), []byte("\r"))
	w.emit(strings.ToValidUTF8(string(b), "\uFFFD"))
}

func (w *lineWriter) maskList() [][]byte {
	if w.masks == nil {
		return nil
	}
	var out [][]byte
	for _, m := range w.masks() {
		if len(m) > 0 {
			out = append(out, m)
		}
	}
	return out
}

// spans returns the merged [start,end) byte ranges of b covered by any mask
// value, found in the original bytes so overlapping values (one a prefix of
// another, or sharing a suffix) are hidden as a whole.
func spans(b []byte, masks [][]byte) [][2]int {
	var found [][2]int
	for _, m := range masks {
		for from := 0; from < len(b); {
			at := bytes.Index(b[from:], m)
			if at < 0 {
				break
			}
			at += from
			found = append(found, [2]int{at, at + len(m)})
			from = at + 1
		}
	}
	return mergeSpans(found)
}

// mergeSpans sorts ranges and merges those that overlap or touch.
func mergeSpans(found [][2]int) [][2]int {
	sort.Slice(found, func(i, j int) bool { return found[i][0] < found[j][0] })
	var merged [][2]int
	for _, f := range found {
		if n := len(merged); n > 0 && f[0] <= merged[n-1][1] {
			merged[n-1][1] = max(merged[n-1][1], f[1])
			continue
		}
		merged = append(merged, f)
	}
	return merged
}

// maskAll replaces every byte range covered by a mask value with maskText.
func maskAll(b []byte, masks [][]byte) []byte { return replaceSpans(b, spans(b, masks)) }

func replaceSpans(b []byte, sp [][2]int) []byte {
	if len(sp) == 0 {
		return b
	}
	var out []byte
	last := 0
	for _, r := range sp {
		out = append(out, b[last:r[0]]...)
		out = append(out, maskText...)
		last = r[1]
	}
	return append(out, b[last:]...)
}

// secretMasks returns the strings to hide for one secret value: the value and,
// for a multi-line value, each of its lines of at least minLineMask bytes
// (the value may be printed one line at a time).
func secretMasks(v string) []string {
	if v == "" {
		return nil
	}
	out := []string{v}
	if strings.ContainsAny(v, "\r\n") {
		for _, ln := range strings.FieldsFunc(v, func(r rune) bool { return r == '\n' || r == '\r' }) {
			if ln = strings.TrimSpace(ln); len(ln) >= minLineMask {
				out = append(out, ln)
			}
		}
	}
	return out
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
