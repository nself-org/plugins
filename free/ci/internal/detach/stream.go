package detach

// Purpose: read the event file of a detached run (P7-CI-63). The child writes
// `--json --stream` NDJSON there, and its stderr lands in the same file, so a
// reader keeps the lines that parse as v1 envelopes and ignores the rest.
// Inputs:  the stream file path. Outputs: Records and complete lines.
// Constraints: only complete lines (ending in \n) are handed out, so a record
// still being written is never cut; reading never blocks and never writes.

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Record is one parsed line of a stream.
type Record struct {
	Raw   []byte // the line without its newline
	Type  string // data.type: queued, planned, ..., result
	Seq   int64
	RunID string
	Err   bool // an error envelope (the stream failed)
	Exit  int  // error.exit_code when Err
}

// Final reports whether no line follows this one: the result or an error.
func (r Record) Final() bool { return r.Err || r.Type == "result" }

type envelope struct {
	Data *struct {
		Type  string `json:"type"`
		Seq   int64  `json:"seq"`
		RunID string `json:"run_id"`
	} `json:"data"`
	Error *struct {
		ExitCode int `json:"exit_code"`
	} `json:"error"`
}

// ParseLine parses one stream line; ok is false for anything that is not a v1
// envelope carrying a record or an error (stderr text, blank lines).
func ParseLine(line []byte) (Record, bool) {
	line = bytes.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '{' {
		return Record{}, false
	}
	var e envelope
	if json.Unmarshal(line, &e) != nil {
		return Record{}, false
	}
	switch {
	case e.Error != nil:
		return Record{Raw: line, Err: true, Exit: e.Error.ExitCode}, true
	case e.Data != nil && e.Data.Type != "":
		return Record{Raw: line, Type: e.Data.Type, Seq: e.Data.Seq, RunID: e.Data.RunID}, true
	}
	return Record{}, false
}

// Reader returns the complete lines appended to a file since its last call.
type Reader struct {
	Path string
	off  int64
	buf  []byte
}

// Next reads what the file gained. A missing file is not an error (nothing yet).
func (r *Reader) Next() ([][]byte, error) {
	f, err := os.Open(r.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if _, err = f.Seek(r.off, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	r.off += int64(len(data))
	r.buf = append(r.buf, data...)
	var lines [][]byte
	for {
		i := bytes.IndexByte(r.buf, '\n')
		if i < 0 {
			break
		}
		lines = append(lines, append([]byte(nil), r.buf[:i]...))
		r.buf = r.buf[i+1:]
	}
	r.buf = append([]byte(nil), r.buf...) // do not pin the read buffer
	return lines, nil
}

// RunIDOf reads a stream file from the start and returns the run id of its
// first record ("" when it holds none yet).
func RunIDOf(path string) string {
	r := &Reader{Path: path}
	lines, err := r.Next()
	if err != nil {
		return ""
	}
	for _, l := range lines {
		if rec, ok := ParseLine(l); ok && rec.RunID != "" {
			return rec.RunID
		}
	}
	return ""
}

var safeRunID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidRunID reports whether id is safe to use as a file name.
func ValidRunID(id string) bool { return safeRunID.MatchString(id) }

// Home resolves the state directory the way the coordinator does: home, else
// NSELF_CI_HOME, else ~/.nself/ci ("" when no home directory is known).
func Home(home string) string {
	if home == "" {
		home = os.Getenv("NSELF_CI_HOME")
	}
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return home
}

// Dir is the directory of detached streams under the state directory.
func Dir(home string) string { return filepath.Join(home, "detached") }

// StreamPath finds the event file of a run: <run>.ndjson, else a token-named
// file whose first record names the run ("" when there is none).
func StreamPath(home, runID string) string {
	if !ValidRunID(runID) {
		return ""
	}
	p := filepath.Join(Dir(home), runID+".ndjson")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	entries, err := os.ReadDir(Dir(home))
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ndjson") {
			continue
		}
		if q := filepath.Join(Dir(home), e.Name()); RunIDOf(q) == runID {
			return q
		}
	}
	return ""
}
