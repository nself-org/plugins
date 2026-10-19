package specs

// Purpose: write the small regular files of a restore on a few goroutines
// (D-1008 round 3). A Go build cache is a thousand files of about 50 KB: the
// restore is a page-cache copy, kernel bound, and it was on the critical path of
// the first job of every run, one file at a time.
// Inputs:  the pinned root, the unpack options and each file's body.
// Outputs: files on disk with the modes and times the archive names.
// Constraints: the tar stream, the sanitiser, the bomb guard and every
// directory and symlink stay on the one reading goroutine, in archive order;
// only the open, write, chmod and chtimes of a regular file move. A body is read
// into a bounded buffer (smallFile, writeBuffers) so memory stays flat, and the
// first write error stops the restore like a sequential one would. A larger file
// is written in place by the caller.

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"sync"
	"time"
)

const (
	smallFile     = 4 << 20 // bodies up to this size are buffered and handed to a writer
	writeBuffers  = 16      // buffers in flight: bounds memory to writeBuffers * smallFile
	maxFileWriter = 8
)

// afterFileStart is a test seam: called as a writer goroutine starts on a file.
var afterFileStart func()

type writeJob struct {
	name string
	buf  []byte
	mode os.FileMode
	at   time.Time
}

type fileWriter struct {
	root *os.Root
	opt  UnpackOptions
	jobs chan writeJob
	free chan []byte
	wg   sync.WaitGroup
	once sync.Once
	mu   sync.Mutex
	err  error
	made map[string]bool // directories the reading goroutine already created
}

// newFileWriter starts the writers; nil means write in place (a single CPU).
func newFileWriter(root *os.Root, opt UnpackOptions) *fileWriter {
	n := min(runtime.GOMAXPROCS(0), maxFileWriter)
	if n < 2 {
		return nil
	}
	w := &fileWriter{root: root, opt: opt, jobs: make(chan writeJob, writeBuffers), free: make(chan []byte, writeBuffers), made: map[string]bool{}}
	for range writeBuffers {
		w.free <- nil
	}
	for range n {
		w.wg.Add(1)
		go w.loop()
	}
	return w
}

func (w *fileWriter) loop() {
	defer w.wg.Done()
	for j := range w.jobs {
		if w.failed() == nil {
			if err := writeSmall(w.root, j.name, j.buf, j.mode, j.at, w.opt); err != nil {
				w.fail(err)
			}
		}
		w.free <- j.buf
	}
}

func (w *fileWriter) fail(err error) {
	w.mu.Lock()
	if w.err == nil {
		w.err = err
	}
	w.mu.Unlock()
}

func (w *fileWriter) failed() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

// submit reads the body of h from r (already bomb-guarded) and queues the write.
func (w *fileWriter) submit(name string, r io.Reader, h *tar.Header) error {
	if err := w.failed(); err != nil {
		return err
	}
	if dir := path.Dir(name); dir != "." && !w.made[dir] {
		if err := w.root.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		w.made[dir] = true
	}
	buf := <-w.free
	if int64(cap(buf)) < h.Size {
		buf = make([]byte, h.Size)
	}
	buf = buf[:h.Size]
	if _, err := io.ReadFull(r, buf); err != nil {
		w.free <- buf
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			err = io.ErrUnexpectedEOF // a short body; any other error (a bomb guard's, a flipped bit's) is kept as it is
		}
		return fmt.Errorf("specs: writing %q: %w", name, err)
	}
	w.jobs <- writeJob{name: name, buf: buf, mode: os.FileMode(h.Mode & 0o777), at: h.ModTime}
	return nil
}

// close waits for every queued file and reports the first write error. It may
// be called again, and on a nil writer (a restore that writes in place).
func (w *fileWriter) close() error {
	if w == nil {
		return nil
	}
	w.once.Do(func() {
		close(w.jobs)
		w.wg.Wait()
	})
	return w.failed()
}

// writeSmall creates name with data and the archive's mode and time.
func writeSmall(root *os.Root, name string, data []byte, mode os.FileMode, at time.Time, opt UnpackOptions) error {
	if afterFileStart != nil {
		afterFileStart()
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("specs: writing %q: %w", name, err)
	}
	return settleFile(root, name, mode, at, opt)
}
