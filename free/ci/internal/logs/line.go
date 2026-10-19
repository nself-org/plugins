package logs

// Purpose: the stored line, the writer options and the tail scan of an active
// file (the recovery half of the writer).
// Inputs:  the bytes of an active file. Outputs: its last good line, its first
// sequence number and the length of its good prefix.
// Constraints: a stored line is one JSON object ending in "\n"; a torn tail (bytes
// after the last "\n", the signature of a kill -9 mid-write) is not a line and
// is cut off; a bad line that IS newline-terminated means the file is corrupt
// (E632), never silently repaired.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"time"
)

// Line is one stored log line. Seq counts from 1 per attempt and is the truth
// of the log's order. Stream is "out" for job output and "sys" for notes the
// log itself adds (the exec funnel merges stderr into the one output pipe).
type Line struct {
	TS     string `json:"ts"`
	Stream string `json:"stream"`
	Seq    int64  `json:"seq"`
	Text   string `json:"text"`
}

// Defaults for Options.
const (
	DefaultSegmentBytes = 4 << 20
	DefaultMaxBytes     = 64 << 20
	DefaultMaxLine      = 16 << 10
	// CapNote is the last line of an attempt log that hit its size cap.
	CapNote = "[log truncated: per-attempt size cap reached]"
)

// Options tunes a Writer; zero values take the defaults.
type Options struct {
	SegmentBytes int64 // rotate and gzip the active file at this size
	MaxBytes     int64 // uncompressed bytes kept per attempt; a note ends the log
	MaxLine      int   // longest stored text; longer lines are split
	Now          func() time.Time
}

func (o Options) withDefaults() Options {
	if o.SegmentBytes <= 0 {
		o.SegmentBytes = DefaultSegmentBytes
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.MaxLine <= 0 {
		o.MaxLine = DefaultMaxLine
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// Stamp formats a time the way stored lines carry it.
func Stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func encodeLine(l Line) []byte {
	b, _ := json.Marshal(l) // strings and ints only: cannot fail
	return append(b, '\n')
}

// tailScan is what scanning an active file found.
type tailScan struct {
	First, Last int64 // 0 when the file holds no line
	Lines       int
	Good        int64 // bytes of complete, valid lines
	Torn        bool  // bytes follow the last newline
}

// scanActive validates the active file at path. A missing file is empty.
func scanActive(path string) (tailScan, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return tailScan{}, nil
	}
	if err != nil {
		return tailScan{}, fail(CodeStore, "cannot open the log file: %v", err)
	}
	defer func() { _ = f.Close() }()
	var t tailScan
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		raw, err := r.ReadBytes('\n')
		if err == io.EOF {
			t.Torn = len(raw) > 0
			return t, nil
		}
		if err != nil {
			return t, fail(CodeStore, "cannot read the log file: %v", err)
		}
		var l Line
		if json.Unmarshal(bytes.TrimSpace(raw), &l) != nil || l.Seq < 1 || (t.Lines > 0 && l.Seq != t.Last+1) {
			return t, fail(CodeStore, "the log file is corrupt at line %d (after sequence %d)", t.Lines+1, t.Last)
		}
		if t.Lines == 0 {
			t.First = l.Seq
		}
		t.Last, t.Lines, t.Good = l.Seq, t.Lines+1, t.Good+int64(len(raw))
	}
}
