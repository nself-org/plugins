package logs

// Purpose: turn the redacted output chunks of one attempt into stored lines.
// Inputs:  Write(chunk) with bytes that already passed the redactor (the exec
// funnel redacts before the run's output callback, which feeds this sink).
// Outputs: lines on a Writer, stored by one flusher goroutine every FlushEvery
// or when FlushBytes are waiting.
// Constraints: Write only queues, so a slow or stalled disk can never block the
// job's output pipe; the queue is bounded (MaxPending): past it new lines are
// dropped, counted, and a "sys" line says how many; after the first storage
// error the sink drops further output and reports the error from Err and
// Close; sequence numbers are assigned when the flusher stores a batch, so
// dropped lines never leave a gap; a text longer than MaxLine is split on a
// rune boundary; an unfinished line is stored once it has waited PartialAfter;
// the arrival time of a line is its stamp; Close waits at most CloseTimeout
// for the flusher.

import (
	"fmt"
	"sync"
	"time"
	"unicode/utf8"
)

// Flush tuning; tests may lower them.
var (
	FlushEvery   = 100 * time.Millisecond
	PartialAfter = 400 * time.Millisecond
	FlushBytes   = 64 << 10
	MaxPending   = 4 << 20
	CloseTimeout = 10 * time.Second
)

// Sink splits chunks into lines and stores them through a Writer.
type Sink struct {
	mu           sync.Mutex
	w            *Writer
	stream       string
	pending      []Line
	pendingBytes int
	dropped      int
	partial      []byte
	partialSince time.Time
	err          error
	closed       bool
	kick         chan struct{}
	stop         chan struct{}
	finished     chan struct{}
	abandoned    bool // Close gave up; the flusher closes the writer when it ends
}

// NewSink writes to w; stream labels the lines ("out" for job output).
func NewSink(w *Writer, stream string) *Sink {
	s := &Sink{w: w, stream: stream, kick: make(chan struct{}, 1), stop: make(chan struct{}), finished: make(chan struct{})}
	go s.run()
	return s
}

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
		if n == 0 {
			break // wait for the rest of an incomplete rune
		}
		s.addText(s.partial[:n], now)
		s.partial = append(s.partial[:0], s.partial[n:]...)
	}
	if s.pendingBytes >= FlushBytes {
		select {
		case s.kick <- struct{}{}:
		default:
		}
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
	// At the end of a chunk, the last rune may still be incomplete.
	if n == 0 {
		return 0
	}
	if n == len(b) {
		start := n - 1
		for start > 0 && !utf8.RuneStart(b[start]) {
			start--
		}
		if !utf8.FullRune(b[start:]) {
			return start
		}
	}
	for n > 0 && n < len(b) && !utf8.RuneStart(b[n]) {
		n--
	}
	if n == 0 {
		return min(len(b), utf8.UTFMax)
	}
	return n
}

// addText queues one line (splitting it when longer than MaxLine); past
// MaxPending it counts the line as dropped instead. Callers hold s.mu.
func (s *Sink) addText(b []byte, at time.Time) {
	for first := true; first || len(b) > 0; first = false {
		n := len(b)
		if n > s.w.opt.MaxLine {
			n = cutRune(b, s.w.opt.MaxLine)
		}
		if s.pendingBytes >= MaxPending {
			s.dropped++
		} else {
			s.pending = append(s.pending, Line{TS: Stamp(at), Stream: s.stream, Text: string(b[:n])})
			s.pendingBytes += n + 64
		}
		b = b[n:]
	}
}

// run is the flusher: it stores batches until stopped, then once more.
func (s *Sink) run() {
	defer func() {
		s.mu.Lock()
		abandoned := s.abandoned
		s.mu.Unlock()
		if abandoned {
			_ = s.w.Close() // Close timed out and left the writer to this goroutine
		}
		close(s.finished)
	}()
	tick := time.NewTicker(FlushEvery)
	defer tick.Stop()
	for final := false; !final; {
		select {
		case <-tick.C:
		case <-s.kick:
		case <-s.stop:
			final = true
		}
		if batch := s.take(final); len(batch) > 0 {
			s.store(batch)
		}
	}
}

// take removes the lines ready to store; with final, or once an unfinished
// line has waited PartialAfter, the unfinished line goes too.
func (s *Sink) take(final bool) []Line {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.partial) > 0 && (final || s.w.opt.Now().Sub(s.partialSince) >= PartialAfter) {
		s.addText(s.partial, s.partialSince)
		s.partial = s.partial[:0]
	}
	batch := s.pending
	if s.dropped > 0 && len(batch) > 0 { // lines drop only once the queue is full, so the gap follows the queued ones
		batch = append(batch, Line{TS: Stamp(s.w.opt.Now()), Stream: "sys", Text: fmt.Sprintf("[%d output lines dropped: log storage was too slow]", s.dropped)})
		s.dropped = 0
	}
	s.pending, s.pendingBytes = nil, 0
	if s.err != nil {
		return nil
	}
	return batch
}

// store numbers a batch from the writer's tail and writes it. It runs
// without s.mu, so a stalled disk blocks only this goroutine.
func (s *Sink) store(batch []Line) {
	base := s.w.LastSeq() + 1
	for i := range batch {
		batch[i].Seq = base + int64(i)
	}
	if _, err := s.w.AppendBatch(batch); err != nil {
		s.mu.Lock()
		if s.err == nil {
			s.err = err
		}
		s.mu.Unlock()
	}
}

// Err is the first storage error, or nil.
func (s *Sink) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Close stores everything still held, then closes the writer. Safe to call
// twice. CloseTimeout bounds the whole call, the final flush and the writer's
// closing fsync together: when the disk is so slow that either does not finish
// in time, Close gives up with an error and leaves the rest to its goroutine.
func (s *Sink) Close() error {
	s.mu.Lock()
	if s.closed {
		defer s.mu.Unlock()
		return s.err
	}
	s.closed = true
	s.mu.Unlock()
	close(s.stop)
	deadline := time.NewTimer(CloseTimeout)
	defer deadline.Stop()
	select {
	case <-s.finished:
	case <-deadline.C:
		s.mu.Lock()
		s.abandoned = true
		s.mu.Unlock()
		return s.giveUp()
	}
	err := s.Err()
	closed := make(chan error, 1)
	go func() { closed <- s.w.Close() }() // the final fsync can stall as well; Writer.Close is idempotent
	select {
	case cerr := <-closed:
		if err == nil {
			err = cerr
		}
		return err
	case <-deadline.C:
		if err != nil {
			return err // a storage error that came first outranks the timeout
		}
		return s.giveUp()
	}
}

// giveUp records and returns the timeout error unless a storage error came first.
func (s *Sink) giveUp() error {
	err := fail(CodeStore, "storing the log did not finish within %v; later lines may be missing", CloseTimeout)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
	return err
}
