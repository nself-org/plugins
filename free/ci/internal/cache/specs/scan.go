package specs

// Purpose: pass a byte stream through the registered-secret scan before any of
// it reaches the blob store (R-SEC-18: scan before commit, Epic CACHE D5).
// Inputs:  a downstream writer and an authz.ValueScanner.
// Outputs: scanWriter, ErrSecretMatch.
// Constraints: bytes are released downstream only after the window holding them
// was scanned, and in stream order; windows are 4 MiB with a 1 MiB overlap, so a
// value that spans two windows is found as long as its longest encoded form
// fits the overlap: redact.Register caps a value at redact.MaxValueLen so that
// always holds. Windows are scanned by a small shared worker pool while the
// stream keeps filling the next ones (D-1515); the scanner must be safe for
// concurrent use, as authz.ValueScanner says.
// A nil scanner refuses everything. The scanned stream is the uncompressed tar;
// gzip payloads among its files are scanned inflated (inflate.go).

import (
	"errors"
	"io"
	"runtime"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/redact"
)

const (
	scanWindow  = 4 << 20
	scanOverlap = 1 << 20
)

// The widest encoded form of a registered value is 6 times its length (every
// byte as a \uXXXX escape); the overlap must hold 8 times the cap.
const _ = uint(scanOverlap - 8*redact.MaxValueLen)

// ErrSecretMatch means the stream holds a registered secret value, or no
// scanner was available to say it does not. It carries no value or offset.
var ErrSecretMatch = errors.New("specs: the cache holds a registered secret value")

// scanSlots bounds the windows being scanned at once across every writer.
var scanSlots = make(chan struct{}, min(max(runtime.GOMAXPROCS(0), 2), 8))

// scanBufs recycles window buffers (overlap plus one window).
var scanBufs = sync.Pool{New: func() any { return make([]byte, 0, scanOverlap+scanWindow) }}

// scanJob is one window: win holds the overlap tail followed by the chunk to
// release; done carries the verdict.
type scanJob struct {
	win  []byte
	tail int
	done chan bool
}

type scanWriter struct {
	dst   io.Writer
	sc    authz.ValueScanner
	cur   []byte // overlap tail (copied) then the chunk being filled
	tail  int    // length of the overlap tail at the front of cur
	queue []*scanJob
	err   error
}

func newScanWriter(dst io.Writer, sc authz.ValueScanner) *scanWriter {
	return &scanWriter{dst: dst, sc: sc}
}

func (w *scanWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n := len(p)
	for len(p) > 0 {
		if w.cur == nil {
			w.cur = scanBufs.Get().([]byte)[:w.tail]
		}
		k := min(len(p), scanWindow-(len(w.cur)-w.tail))
		w.cur = append(w.cur, p[:k]...)
		p = p[k:]
		if len(w.cur)-w.tail == scanWindow {
			if w.err = w.submit(); w.err != nil {
				return 0, w.err
			}
		}
	}
	return n, nil
}

// Close scans and releases the last window and waits for the rest; it does not
// close dst.
func (w *scanWriter) Close() error {
	if w.err == nil && w.cur != nil && len(w.cur) > w.tail {
		w.err = w.submit()
	}
	for w.err == nil && len(w.queue) > 0 {
		w.err = w.releaseFront()
	}
	w.queue = nil
	return w.err
}

// submit queues w.cur for scanning and starts the next window with its tail.
// It releases finished windows from the front, and waits for the oldest while
// too many are in flight.
func (w *scanWriter) submit() error {
	if w.sc == nil {
		return ErrSecretMatch
	}
	job := &scanJob{win: w.cur, tail: w.tail, done: make(chan bool, 1)}
	w.cur = nil
	w.tail = min(scanOverlap, len(job.win))
	go func() {
		scanSlots <- struct{}{}
		job.done <- authz.CleanCandidates(w.sc, job.win)
		<-scanSlots
	}()
	w.queue = append(w.queue, job)
	// Copy the overlap now: the window buffer is recycled once it is released.
	w.cur = append(scanBufs.Get().([]byte)[:0], job.win[len(job.win)-w.tail:]...)
	for len(w.queue) > cap(scanSlots)+1 || (len(w.queue) > 0 && len(w.queue[0].done) > 0) {
		if err := w.releaseFront(); err != nil {
			return err
		}
	}
	return nil
}

// releaseFront waits for the oldest window and, if it is clean, writes its
// chunk downstream.
func (w *scanWriter) releaseFront() error {
	job := w.queue[0]
	w.queue = w.queue[1:]
	if !<-job.done {
		return ErrSecretMatch
	}
	_, err := w.dst.Write(job.win[job.tail:])
	scanBufs.Put(job.win[:0])
	return err
}
