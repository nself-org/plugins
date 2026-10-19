package session

import (
	"bytes"
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
		w.line(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	masks := w.maskList()
	longest := 0
	for _, m := range masks {
		longest = max(longest, len(m))
	}
	for len(w.buf) > maxLogLine+longest {
		cut := len(w.buf) - max(longest-1, 0)
		for moved := true; moved; { // never cut through a complete secret value
			moved = false
			for _, m := range masks {
				for from := 0; ; {
					at := bytes.Index(w.buf[from:], m)
					if at < 0 {
						break
					}
					at += from
					if at < cut && cut < at+len(m) {
						cut, moved = at, true
					}
					from = at + 1
				}
			}
		}
		if cut <= 0 {
			break
		}
		w.line(w.buf[:cut])
		w.buf = w.buf[cut:]
	}
}

// Flush emits the unterminated tail.
func (w *lineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.line(w.buf)
		w.buf = nil
	}
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

func (w *lineWriter) line(b []byte) {
	b = bytes.TrimSuffix(b, []byte("\r"))
	w.emit(strings.ToValidUTF8(string(maskAll(b, w.maskList())), "�"))
}

// maskAll replaces every occurrence of every mask value, longest first.
func maskAll(b []byte, masks [][]byte) []byte {
	for _, m := range masks {
		b = bytes.ReplaceAll(b, m, []byte(maskText))
	}
	return b
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
