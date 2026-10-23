package agent

// Purpose: the coordinator's durable log of one lease (R-DS-19, R-DS-24): a JSONL file whose every line embeds
//          the agent's line_seq, so a resend after a reconnect or a coordinator crash never duplicates or loses
//          a line.
// Inputs:  protocol.Log frames of one lease.
// Outputs: <dir>/<lease>.jsonl; Append reports whether the line was new.
// Constraints: the tail line_seq is read when the file is opened and a line with line_seq <= tail is dropped;
//          a line is acknowledged only after Sync (the driver syncs before every heartbeat ack and result); a torn last line left by a kill -9 is cut
//          off on open, so the writer never appends after half a line; the file is 0600 in a 0700 directory;
//          text is stored as the agent sent it (already redacted at source).

import (
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
//
// A line is at most about six times its frame: the JSON encoder writes U+2028 and U+2029 as six bytes (three in
// the frame) and, up to Go 1.26, an invalid byte as the six-byte escape \ufffd (Go 1.27 writes the three-byte rune).
// Eight frames leave room for the envelope around the worst case.
const tailWindow = 8 * protocol.MaxFrame

var unsafeFile = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// MaxLogBytes caps the durable log of one lease.
const MaxLogBytes = 256 << 20

// logCap is MaxLogBytes; a test lowers it.
var logCap int64 = MaxLogBytes

// truncNote is the text of the line that ends a log which reached the cap.
const truncNote = "[log truncated: the coordinator keeps at most 256 MiB of one lease's output]"

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
	mu     sync.Mutex
	f      *os.File
	last   uint64
	dirty  bool // lines were appended since the last Sync
	capped bool // the size cap was hit and the truncation line written
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
	buf := make([]byte, min(size, tailWindow))
	start := size - int64(len(buf))
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
	w.capped = ll.Text == truncNote // a restart does not write a second note
	return nil
}

// Last is the highest line_seq on disk.
func (w *LogWriter) Last() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

// Append writes l unless its line_seq is not above the tail. written is false for a duplicate. The line is in
// the file when Append returns and on stable storage after the next Sync.
func (w *LogWriter) Append(l protocol.Log) (written bool, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return false, os.ErrClosed
	}
	if l.LineSeq <= w.last {
		return false, nil
	}
	var enc bytes.Buffer
	je := json.NewEncoder(&enc)
	je.SetEscapeHTML(false) // '<' stays one byte; only U+2028/2029 and invalid bytes grow (see tailWindow)
	if err := je.Encode(logLine{LineSeq: l.LineSeq, Stream: l.Stream, Text: l.Text}); err != nil {
		return false, err
	}
	b := enc.Bytes() // the encoder ends the line with '\n'
	before, err := w.f.Seek(0, io.SeekEnd)
	if err != nil {
		return false, err
	}
	if w.capped || before+int64(len(b)) > logCap { // a runaway job cannot fill the coordinator's disk; after the note nothing more is kept
		if !w.capped {
			w.capped = true
			note, _ := json.Marshal(logLine{LineSeq: l.LineSeq, Stream: "stderr", Text: truncNote})
			_, _ = w.f.Write(append(note, '\n'))
			w.last = l.LineSeq
			w.dirty = true
		}
		return false, nil
	}
	if _, err = w.f.Write(b); err != nil {
		_ = w.f.Truncate(before) // never leave half a line for the next append to follow
		return false, err
	}
	w.last = l.LineSeq
	w.dirty = true
	return true, nil
}

// Sync flushes the lines appended since the last Sync to stable storage. The pull driver calls it before it
// acknowledges anything (a heartbeat or a result), so an acknowledged line is on disk while a stream of
// thousands of lines per second does not pay one fsync each.
func (w *LogWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil || !w.dirty {
		return nil
	}
	if err := w.f.Sync(); err != nil {
		return err
	}
	w.dirty = false
	return nil
}

// Close closes the file.
func (w *LogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	var err error
	if w.dirty {
		err = w.f.Sync()
	}
	err = errors.Join(err, w.f.Close())
	w.f = nil
	return err
}
