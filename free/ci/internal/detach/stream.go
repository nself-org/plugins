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

// eventSchema is the schema every ci.events record carries (internal/events.Schema).
const eventSchema = "ci.events/v1"

var errorCode = regexp.MustCompile(`^E[0-9]{3}$`)

type envelope struct {
	SchemaVersion string `json:"schema_version"`
	Data          *struct {
		Type   string `json:"type"`
		Schema string `json:"schema"`
		Seq    int64  `json:"seq"`
		RunID  string `json:"run_id"`
	} `json:"data"`
	Error *struct {
		Code     string `json:"code"`
		ExitCode int    `json:"exit_code"`
	} `json:"error"`
}

// ParseLine parses one stream line; ok is false for anything that is not a v1
// envelope carrying a ci.events record (schema ci.events/v1, seq >= 1) or a
// coded error (stderr text, other JSON, blank lines).
func ParseLine(line []byte) (Record, bool) {
	line = bytes.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '{' {
		return Record{}, false
	}
	var e envelope
	if json.Unmarshal(line, &e) != nil || e.SchemaVersion != "1" {
		return Record{}, false
	}
	switch {
	case e.Error != nil && errorCode.MatchString(e.Error.Code):
		return Record{Raw: line, Err: true, Exit: e.Error.ExitCode}, true
	case e.Data != nil && e.Data.Schema == eventSchema && e.Data.Type != "" && e.Data.Seq >= 1:
		return Record{Raw: line, Type: e.Data.Type, Seq: e.Data.Seq, RunID: e.Data.RunID}, true
	}
	return Record{}, false
}

const (
	maxChunk = 4 << 20 // most bytes one Next call reads
	maxLine  = 8 << 20 // longest line kept; a longer one is dropped whole
)

// Reader returns the complete lines appended to a file since its last call. It
// keeps the file open, so a rename of the path (Start renames the token-named
// file to the run id) does not lose the lines written afterwards.
type Reader struct {
	Path string
	f    *os.File
	buf  []byte
	skip bool // inside a line longer than maxLine
}

// Close releases the file.
func (r *Reader) Close() error {
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Next reads what the file gained (at most maxChunk bytes). A missing file is
// not an error (nothing yet).
func (r *Reader) Next() ([][]byte, error) {
	if r.f == nil {
		f, err := os.Open(r.Path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		r.f = f
	}
	data, err := io.ReadAll(io.LimitReader(r.f, maxChunk))
	if err != nil {
		return nil, err
	}
	r.buf = append(r.buf, data...)
	var lines [][]byte
	for {
		i := bytes.IndexByte(r.buf, '\n')
		if i < 0 {
			break
		}
		if !r.skip {
			lines = append(lines, append([]byte(nil), r.buf[:i]...))
		}
		r.skip = false
		r.buf = r.buf[i+1:]
	}
	if len(r.buf) > maxLine {
		r.buf, r.skip = nil, true
	}
	r.buf = append([]byte(nil), r.buf...) // do not pin the read buffer
	return lines, nil
}

// runIDScan is how much of a stream file RunIDOf reads: the first record is a
// short line, and a sweep must not load whole retained streams.
const runIDScan = 64 << 10

// RunIDOf reads the start of a stream file and returns the run id of its first
// record ("" when it holds none yet).
func RunIDOf(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	head, _ := io.ReadAll(io.LimitReader(f, runIDScan))
	for _, l := range bytes.Split(head, []byte{'\n'}) {
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
