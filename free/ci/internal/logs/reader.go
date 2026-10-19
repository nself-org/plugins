package logs

// Purpose: read stored lines back, finished or still being written.
// Inputs:  an attempt directory, a sequence to start after, and for Follow a
// done callback that says whether the attempt has ended.
// Outputs: Line values in sequence order through a callback.
// Constraints: Read sees rotated segments (gzip) then the active file and
// skips lines at or below `after`; Follow polls every PollEvery, reads the
// active file from a remembered offset, notices a rotation (the active file
// is replaced by a new one) and re-reads the new segment, and after done
// reports true it drains once more before returning; a half-written last line
// is not delivered until its newline arrives; nothing is read or written
// outside dir.

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// PollEvery is how often Follow looks for new lines.
var PollEvery = 100 * time.Millisecond

// Exists reports whether dir holds any stored log file.
func Exists(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, activeName)); err == nil {
		return true
	}
	segs, err := segments(dir)
	return err == nil && len(segs) > 0
}

// Read delivers the stored lines with Seq > after to fn, oldest first. fn
// returning an error stops the read with that error.
func Read(dir string, after int64, fn func(Line) error) error {
	// the active file is opened before the segments are listed: a rotation in between
	// moves its lines into a segment the listing then shows, and the open handle
	// still reads the old file; lines seen twice are dropped by their sequence
	f, ferr := os.Open(filepath.Join(dir, activeName))
	if f != nil {
		defer func() { _ = f.Close() }()
	}
	segs, err := segments(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fail(CodeNone, "no stored logs here")
		}
		return fail(CodeStore, "cannot list the log directory: %v", err)
	}
	cur := after
	for _, s := range segs {
		if s.Last <= cur {
			continue
		}
		if cur, err = readSegment(s.Path, cur, fn); err != nil {
			return err
		}
	}
	if os.IsNotExist(ferr) {
		return nil
	}
	if ferr != nil {
		return fail(CodeStore, "cannot open the log file: %v", ferr)
	}
	_, _, err = readLines(f, cur, fn)
	return err
}

func readSegment(path string, after int64, fn func(Line) error) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return after, fail(CodeStore, "cannot open a log segment: %v", err)
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return after, fail(CodeStore, "a log segment is not valid gzip: %v", err)
	}
	cur, _, err := readLines(zr, after, fn)
	return cur, err
}

// readLines delivers complete lines above after. It returns the highest
// sequence seen, and the bytes consumed (complete lines only).
func readLines(r io.Reader, after int64, fn func(Line) error) (cur int64, used int64, err error) {
	cur = after
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		raw, rerr := br.ReadBytes('\n')
		if rerr != nil { // EOF: a partial last line stays unread
			if rerr == io.EOF {
				return cur, used, nil
			}
			return cur, used, fail(CodeStore, "cannot read the log: %v", rerr)
		}
		used += int64(len(raw))
		var l Line
		if json.Unmarshal(bytes.TrimSpace(raw), &l) != nil || l.Seq < 1 {
			return cur, used, fail(CodeStore, "a stored log line is corrupt")
		}
		if l.Seq <= cur {
			continue
		}
		cur = l.Seq
		if err = fn(l); err != nil {
			return cur, used, err
		}
	}
}

// Follow delivers lines as they are stored until done() reports true (it then
// drains once more) or ctx ends. A directory that does not exist yet is
// waited for while done() is false.
func Follow(ctx context.Context, dir string, after int64, done func() bool, fn func(Line) error) error {
	fl := follower{dir: dir, cur: after, fn: fn}
	t := time.NewTicker(PollEvery)
	defer t.Stop()
	for {
		finished := done()
		if err := fl.poll(); err != nil {
			return err
		}
		if finished {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

type follower struct {
	dir    string
	cur    int64
	fn     func(Line) error
	active os.FileInfo // identity of the active file read so far
	offset int64
}

// poll reads whatever was stored since the last call.
func (fl *follower) poll() error {
	// open first, list second: see Read
	f, ferr := os.Open(filepath.Join(fl.dir, activeName))
	if f != nil {
		defer func() { _ = f.Close() }()
	}
	segs, err := segments(fl.dir)
	if err != nil && !os.IsNotExist(err) {
		return fail(CodeStore, "cannot list the log directory: %v", err)
	}
	for _, s := range segs {
		if s.Last <= fl.cur {
			continue
		}
		if fl.cur, err = readSegment(s.Path, fl.cur, fl.fn); err != nil {
			return err
		}
		fl.active, fl.offset = nil, 0 // a rotation moved the active file's lines into this segment
	}
	if os.IsNotExist(ferr) {
		return nil
	}
	if ferr != nil {
		return fail(CodeStore, "cannot open the log file: %v", ferr)
	}
	st, err := f.Stat()
	if err != nil {
		return fail(CodeStore, "cannot stat the log file: %v", err)
	}
	if fl.active == nil || !os.SameFile(fl.active, st) || st.Size() < fl.offset {
		fl.active, fl.offset = st, 0
	}
	if _, err = f.Seek(fl.offset, io.SeekStart); err != nil {
		return fail(CodeStore, "cannot seek the log file: %v", err)
	}
	cur, used, err := readLines(f, fl.cur, fl.fn)
	fl.cur, fl.offset = cur, fl.offset+used
	return err
}
