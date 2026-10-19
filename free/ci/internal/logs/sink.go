package logs

// Purpose: turn the redacted output chunks of one attempt into stored lines.
// Inputs:  Write(chunk) with bytes that already passed the redactor (the exec
// funnel redacts before the run's output callback, which feeds this sink).
// Outputs: lines on a Writer, flushed every FlushEvery or at FlushBytes.
// Constraints: Write never fails the job: after the first storage error the sink
// drops further output and reports the error from Err and Close; sequence
// numbers are assigned at flush time from the writer's tail, so dropped lines
// never leave a gap; a text longer than MaxLine is split on a rune boundary;
// an unfinished line is stored once it has waited PartialAfter; the arrival
// time of a line is its stamp.

import (
	"sync"
	"time"
	"unicode/utf8"
)

// Flush tuning; tests may lower them.
var (
	FlushEvery   = 100 * time.Millisecond
	PartialAfter = 400 * time.Millisecond
	FlushBytes   = 64 << 10
)

// Sink splits chunks into lines and stores them through a Writer.
type Sink struct {
	mu           sync.Mutex
	w            *Writer
	stream       string
	pending      []Line
	pendingBytes int
	partial      []byte
	partialSince time.Time
	timer        *time.Timer
	err          error
	closed       bool
}

// NewSink writes to w; stream labels the lines ("out" for job output).
func NewSink(w *Writer, stream string) *Sink { return &Sink{w: w, stream: stream} }

// Write accepts a chunk. It always reports the chunk as consumed.
func (s *Sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.err != nil {
		return len(p), nil
	}
	now := s.w.opt.Now()
	if len(s.partial) == 0 && len(p) > 0 {
		s.partialSince = now
	}
	s.partial = append(s.partial, p...)
	for {
		i := indexNewline(s.partial)
		if i < 0 {
			break
		}
		s.addText(trimCR(s.partial[:i]), now)
		s.partial = append(s.partial[:0], s.partial[i+1:]...)
		s.partialSince = now
	}
	for len(s.partial) >= s.w.opt.MaxLine {
		n := cutRune(s.partial, s.w.opt.MaxLine)
		s.addText(s.partial[:n], now)
		s.partial = append(s.partial[:0], s.partial[n:]...)
	}
	if s.pendingBytes >= FlushBytes {
		s.flush(false)
	} else if len(s.pending) > 0 || len(s.partial) > 0 {
		s.arm()
	}
	return len(p), nil
}

func indexNewline(b []byte) int {
	for i, c := range b {
		if c == '\n' {
			return i
		}
	}
	return -1
}

func trimCR(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\r' {
		return b[:n-1]
	}
	return b
}

// cutRune returns n, moved back so the cut does not split a rune.
func cutRune(b []byte, n int) int {
	for n > 0 && n < len(b) && !utf8.RuneStart(b[n]) {
		n--
	}
	if n == 0 {
		return min(len(b), utf8.UTFMax)
	}
	return n
}

// addText queues one line (splitting it when longer than MaxLine).
func (s *Sink) addText(b []byte, at time.Time) {
	for first := true; first || len(b) > 0; first = false {
		n := len(b)
		if n > s.w.opt.MaxLine {
			n = cutRune(b, s.w.opt.MaxLine)
		}
		s.pending = append(s.pending, Line{TS: Stamp(at), Stream: s.stream, Text: string(b[:n])})
		s.pendingBytes += n + 64
		b = b[n:]
	}
}

// arm schedules the timed flush if none is waiting.
func (s *Sink) arm() {
	if s.timer == nil {
		s.timer = time.AfterFunc(FlushEvery, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.timer = nil
			if !s.closed {
				s.flush(false)
				if len(s.pending) > 0 || len(s.partial) > 0 {
					s.arm()
				}
			}
		})
	}
}

// flush stores the pending lines; with final, or once an unfinished line has
// waited PartialAfter, the unfinished line goes too.
func (s *Sink) flush(final bool) {
	if len(s.partial) > 0 && (final || s.w.opt.Now().Sub(s.partialSince) >= PartialAfter) {
		s.addText(s.partial, s.partialSince)
		s.partial = s.partial[:0]
	}
	if len(s.pending) == 0 || s.err != nil {
		s.pending, s.pendingBytes = nil, 0
		return
	}
	base := s.w.LastSeq() + 1
	for i := range s.pending {
		s.pending[i].Seq = base + int64(i)
	}
	if _, err := s.w.AppendBatch(s.pending); err != nil {
		s.err = err
	}
	s.pending, s.pendingBytes = nil, 0
}

// Err is the first storage error, or nil.
func (s *Sink) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close stores everything still held, then closes the writer. Safe to call twice.
func (s *Sink) Close() error {
	s.mu.Lock()
	if s.closed {
		defer s.mu.Unlock()
		return s.err
	}
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.flush(true)
	err := s.err
	s.mu.Unlock()
	if cerr := s.w.Close(); err == nil {
		err = cerr
	}
	return err
}
