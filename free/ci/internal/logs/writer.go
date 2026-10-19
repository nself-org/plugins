package logs

// Purpose: the per-attempt log writer. It stores lines as JSON, one per line,
// with a sequence number per line, and acknowledges a line only after fsync.
// Inputs:  an attempt directory and batches of redacted Lines (the caller owns
// redaction; nothing unredacted may reach this package, see Sink).
// Outputs: <dir>/active.jsonl plus gzipped segments once the active file
// passes SegmentBytes.
// Constraints (R-DS-19): the file's last sequence is the truth, read when the
// writer opens (a torn tail from a kill -9 is cut off); a line with seq <= last
// is dropped as a replay; a line past last+1 is a gap and is refused; the
// returned ack covers only lines that were written and fsynced; a failed write
// is rolled back so no half batch stays; files 0600, directories 0700; one
// writer per attempt directory (the coordinator owns an attempt).

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// faultPoint is the crash-injection seam of the fault tests (build tag fault):
// it is called at each step whose interruption must leave a recoverable log.
// It is nil in production.
var faultPoint func(step string)

func fault(step string) {
	if faultPoint != nil {
		faultPoint(step)
	}
}

// Writer appends to one attempt's log.
type Writer struct {
	mu          sync.Mutex
	dir         string
	opt         Options
	f           *os.File
	last        int64
	activeFirst int64
	activeSize  int64
	rawTotal    int64
	capped      bool
	broken      error
	closed      bool
}

// Open prepares dir (creating it 0700), recovers the tail sequence from the
// files and returns a writer positioned after it.
func Open(dir string, opt Options) (*Writer, error) {
	opt = opt.withDefaults()
	if err := preparePrivate(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, activeName)
	if err := requireRegular(path); err != nil { // before recovery, which may truncate it
		return nil, err
	}
	w := &Writer{dir: dir, opt: opt}
	if err := w.recover(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		return nil, fail(CodeStore, "cannot open the log file: %v", err)
	}
	if err = f.Chmod(fileMode); err != nil && runtime.GOOS != "windows" {
		_ = f.Close()
		return nil, fail(CodeStore, "cannot make the log file private: %v", err)
	}
	// the new file and directory entries must be durable before any line is acknowledged
	if err = syncDir(dir); err == nil {
		err = syncDir(filepath.Dir(dir))
	}
	if err != nil {
		_ = f.Close()
		return nil, fail(CodeStore, "cannot sync the log directory: %v", err)
	}
	w.f = f
	return w, nil
}

// recover reads the tail state from the segments and the active file.
func (w *Writer) recover() error {
	_ = os.Remove(filepath.Join(w.dir, rotateTmp))
	segs, err := segments(w.dir)
	if err != nil {
		return fail(CodeStore, "cannot list the log directory: %v", err)
	}
	var segLast int64
	for _, s := range segs {
		w.rawTotal += s.Bytes
		segLast = max(segLast, s.Last)
	}
	path := filepath.Join(w.dir, activeName)
	t, err := scanActive(path)
	if err != nil {
		return err
	}
	if t.Torn {
		if err = os.Truncate(path, t.Good); err != nil {
			return fail(CodeStore, "cannot cut the torn tail of the log file: %v", err)
		}
	}
	if t.Lines > 0 && t.First <= segLast { // rotated, but the old active file was not removed yet
		if t.Last > segLast {
			return fail(CodeStore, "the log file overlaps a rotated segment")
		}
		if err = os.Truncate(path, 0); err != nil {
			return fail(CodeStore, "cannot clear an archived log file: %v", err)
		}
		t = tailScan{}
	}
	w.last, w.activeFirst, w.activeSize = max(segLast, t.Last), t.First, t.Good
	w.rawTotal += t.Good
	w.capped = w.rawTotal >= w.opt.MaxBytes
	return nil
}

// LastSeq is the highest sequence stored (and fsynced) so far; 0 for an empty log.
func (w *Writer) LastSeq() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

// Capped reports whether the size cap ended the log.
func (w *Writer) Capped() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.capped
}

// Append stores one line; see AppendBatch.
func (w *Writer) Append(l Line) (int64, error) { return w.AppendBatch([]Line{l}) }

// AppendBatch stores lines whose Seq values run last+1, last+2, ... Lines at or
// below the stored sequence are dropped (a replay after resume). It returns
// the highest sequence now durable: every line up to it was written and
// fsynced. Once the size cap is reached further lines are dropped.
func (w *Writer) AppendBatch(lines []Line) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.last, fail(CodeStore, "the log writer is closed")
	}
	if w.broken != nil {
		return w.last, w.broken
	}
	if w.capped {
		return w.last, nil
	}
	var buf bytes.Buffer
	next, raw, capNow := w.last+1, w.rawTotal, false
	for _, l := range lines {
		if l.Seq < next {
			continue
		}
		if l.Seq != next {
			return w.last, fail(CodeStore, "sequence gap: got %d, want %d", l.Seq, next)
		}
		b := encodeLine(l)
		buf.Write(b)
		next, raw = next+1, raw+int64(len(b))
		if raw >= w.opt.MaxBytes {
			note := encodeLine(Line{TS: Stamp(w.opt.Now()), Stream: "sys", Seq: next, Text: CapNote})
			buf.Write(note)
			next, raw, capNow = next+1, raw+int64(len(note)), true
			break
		}
	}
	if buf.Len() == 0 {
		return w.last, nil
	}
	if err := w.commit(buf.Bytes()); err != nil {
		return w.last, err
	}
	if w.activeFirst == 0 {
		w.activeFirst = w.last + 1
	}
	w.last, w.rawTotal, w.capped = next-1, raw, capNow
	w.activeSize += int64(buf.Len())
	if w.activeSize >= w.opt.SegmentBytes {
		_ = w.rotate() // a failed rotation only leaves a bigger active file; the next batch retries
	}
	return w.last, nil
}

// commit writes and fsyncs b; on failure it cuts the file back to its last
// good length so no half batch stays (when that fails too the writer stops).
func (w *Writer) commit(b []byte) error {
	_, err := w.f.Write(b)
	fault("written")
	if err == nil {
		err = w.f.Sync()
	}
	fault("synced")
	if err == nil {
		return nil
	}
	if terr := w.f.Truncate(w.activeSize); terr != nil {
		w.broken = fail(CodeStore, "the log file is in an unknown state after a failed write: %v", err)
		return w.broken
	}
	return fail(CodeStore, "cannot write the log: %v", err)
}

// Close syncs and closes the active file. It is safe to call twice.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.f == nil {
		return nil
	}
	err := w.f.Sync()
	if cerr := w.f.Close(); err == nil {
		err = cerr
	}
	return err
}
