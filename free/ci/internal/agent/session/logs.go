package session

import (
	"bytes"
	"strings"
	"sync"
)

const (
	maskText   = "[redacted]"
	maxLogLine = 8 << 10
)

// lineWriter turns job output chunks into numbered log lines. The job's own
// redactor has already run; this is the second layer, masking the literal
// secret values the lease carried (runner credential, secret frame values)
// before any line leaves the node.
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
	for len(w.buf) > maxLogLine { // a line with no newline is cut, not held without bound
		w.line(w.buf[:maxLogLine])
		w.buf = w.buf[maxLogLine:]
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

func (w *lineWriter) line(b []byte) {
	b = bytes.TrimSuffix(b, []byte("\r"))
	if w.masks != nil {
		for _, m := range w.masks() {
			if len(m) > 0 {
				b = bytes.ReplaceAll(b, m, []byte(maskText))
			}
		}
	}
	w.emit(strings.ToValidUTF8(string(b), "�"))
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
