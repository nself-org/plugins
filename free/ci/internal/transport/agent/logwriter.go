package agent

// Purpose: the coordinator's durable log of one lease (R-DS-19, R-DS-24): a JSONL file whose every line embeds
//          the agent's line_seq, so a resend after a reconnect or a coordinator crash never duplicates or loses
//          a line.
// Inputs:  protocol.Log frames of one lease.
// Outputs: <dir>/<lease>.jsonl; Append reports whether the line was new.
// Constraints: the tail line_seq is read when the file is opened and a line with line_seq <= tail is dropped;
//          a line is acknowledged (Append returns) only after fsync; a torn last line left by a kill -9 is cut
//          off on open, so the writer never appends after half a line; the file is 0600 in a 0700 directory;
//          text is stored as the agent sent it (already redacted at source).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// tailWindow is how much of the file end OpenLog reads to find the last complete line.
const tailWindow = 2 * protocol.MaxFrame

var unsafeFile = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// LogFile is the file name of a lease's log.
func LogFile(dir, leaseID string) string {
	return filepath.Join(dir, unsafeFile.ReplaceAllString(leaseID, "_")+".jsonl")
}

type logLine struct {
	LineSeq uint64 `json:"line_seq"`
	Stream  string `json:"stream"`
	Text    string `json:"text"`
}

// LogWriter appends the lines of one lease.
type LogWriter struct {
	mu   sync.Mutex
	f    *os.File
	last uint64
}

// OpenLog opens (creating) the log file, cuts off a torn last line and reads the tail line_seq.
func OpenLog(path string) (*LogWriter, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	w := &LogWriter{f: f}
	if err = w.recover(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return w, nil
}

// recover finds the last complete line, truncates anything after it and records its line_seq.
func (w *LogWriter) recover() error {
	st, err := w.f.Stat()
	if err != nil {
		return err
	}
	size := st.Size()
	if size == 0 {
		return nil
	}
	start := max(0, size-tailWindow)
	buf := make([]byte, size-start)
	if _, err = w.f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	end := bytes.LastIndexByte(buf, '\n') + 1 // length of the complete part
	if end == 0 {
		if start > 0 {
			return fmt.Errorf("log file has no complete line in its last %d bytes", tailWindow)
		}
		return w.f.Truncate(0)
	}
	if int64(end) != int64(len(buf)) {
		if err = w.f.Truncate(start + int64(end)); err != nil {
			return err
		}
	}
	complete := buf[:end]
	line := complete
	if i := bytes.LastIndexByte(complete[:end-1], '\n'); i >= 0 {
		line = complete[i+1:]
	} else if start > 0 {
		return fmt.Errorf("log file has no complete line in its last %d bytes", tailWindow)
	}
	var ll logLine
	if err = json.Unmarshal(bytes.TrimRight(line, "\n"), &ll); err != nil {
		return fmt.Errorf("log file: the last line is not a log line: %w", err)
	}
	w.last = ll.LineSeq
	return nil
}

// Last is the highest line_seq on disk.
func (w *LogWriter) Last() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

// Append writes l unless its line_seq is not above the tail; it returns after fsync. written is false for a
// duplicate.
func (w *LogWriter) Append(l protocol.Log) (written bool, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return false, os.ErrClosed
	}
	if l.LineSeq <= w.last {
		return false, nil
	}
	b, err := json.Marshal(logLine{LineSeq: l.LineSeq, Stream: l.Stream, Text: l.Text})
	if err != nil {
		return false, err
	}
	if _, err = w.f.Seek(0, io.SeekEnd); err != nil {
		return false, err
	}
	bw := bufio.NewWriterSize(w.f, len(b)+1)
	_, _ = bw.Write(b)
	_ = bw.WriteByte('\n')
	if err = bw.Flush(); err != nil {
		return false, err
	}
	if err = w.f.Sync(); err != nil {
		return false, err
	}
	w.last = l.LineSeq
	return true, nil
}

// Close closes the file.
func (w *LogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}
