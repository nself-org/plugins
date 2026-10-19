package agent

// Purpose: tests of the lease log writer: line_seq dedup, durability across a reopen, a torn tail from a kill -9,
//          permissions and the file name.
// Inputs:  files in t.TempDir().
// Constraints: no sleeps.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func logOf(seq uint64, text string) protocol.Log {
	return protocol.Log{LeaseID: "l1", LineSeq: seq, Stream: "stdout", Text: text}
}

func openTemp(t *testing.T) (*LogWriter, string) {
	t.Helper()
	path := LogFile(filepath.Join(t.TempDir(), "logs"), "lease/1")
	w, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w, path
}

func TestLogFileName(t *testing.T) {
	if got := LogFile("/d", "lease-a1/../x y"); got != "/d/lease-a1_.._x_y.jsonl" {
		t.Fatalf("file = %q", got)
	}
}

func TestLogWriterDropsSeqAtOrBelowTheTail(t *testing.T) {
	w, path := openTemp(t)
	for _, c := range []struct {
		seq  uint64
		want bool
	}{{1, true}, {2, true}, {2, false}, {1, false}, {5, true}, {4, false}, {6, true}} {
		got, err := w.Append(logOf(c.seq, "line"))
		if err != nil || got != c.want {
			t.Fatalf("seq %d: written=%v err=%v, want %v", c.seq, got, err, c.want)
		}
	}
	if w.Last() != 6 {
		t.Fatalf("last = %d", w.Last())
	}
	b, _ := os.ReadFile(path)
	if n := bytes.Count(b, []byte("\n")); n != 4 {
		t.Fatalf("%d lines on disk, want 4:\n%s", n, b)
	}
	if !strings.HasPrefix(string(b), `{"line_seq":1,"stream":"stdout","text":"line"}`+"\n") {
		t.Fatalf("first line = %q", b)
	}
}

func TestLogWriterReadsTheTailOnOpen(t *testing.T) {
	w, path := openTemp(t)
	for i := uint64(1); i <= 3; i++ {
		_, _ = w.Append(logOf(i, "a"))
	}
	_ = w.Close()
	w2, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	if w2.Last() != 3 {
		t.Fatalf("tail = %d, want 3", w2.Last())
	}
	if ok, _ := w2.Append(logOf(3, "again")); ok {
		t.Fatal("a resend of line 3 after a reopen was written")
	}
	if ok, _ := w2.Append(logOf(4, "new")); !ok {
		t.Fatal("line 4 was dropped")
	}
}

func TestLogWriterCutsATornTail(t *testing.T) {
	w, path := openTemp(t)
	_, _ = w.Append(logOf(1, "one"))
	_, _ = w.Append(logOf(2, "two"))
	_ = w.Close()
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	_, _ = f.WriteString(`{"line_seq":3,"stream":"stdout","te`) // kill -9 mid-write
	_ = f.Close()
	w2, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	if w2.Last() != 2 {
		t.Fatalf("tail = %d, want 2 (the torn line is not a line)", w2.Last())
	}
	if ok, _ := w2.Append(logOf(3, "three")); !ok {
		t.Fatal("line 3 not written after the torn tail")
	}
	b, _ := os.ReadFile(path)
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if !strings.HasPrefix(l, `{"line_seq":`) || !strings.HasSuffix(l, `}`) {
			t.Fatalf("damaged line %q in\n%s", l, b)
		}
	}
	if n := strings.Count(string(b), "\n"); n != 3 {
		t.Fatalf("%d lines, want 3", n)
	}
}

func TestLogWriterOnlyATornLineStartsOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"line_seq":1,"str`), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.Last() != 0 {
		t.Fatalf("tail = %d", w.Last())
	}
	if ok, _ := w.Append(logOf(1, "x")); !ok {
		t.Fatal("line 1 not written")
	}
}

func TestLogWriterRefusesAFileThatIsNotALog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLog(path); err == nil {
		t.Fatal("opened a file whose last line is not a log line")
	}
}

func TestLogWriterModesAndClose(t *testing.T) {
	w, path := openTemp(t)
	st, _ := os.Stat(path)
	dst, _ := os.Stat(filepath.Dir(path))
	if st.Mode().Perm() != 0o600 || dst.Mode().Perm() != 0o700 {
		t.Fatalf("modes file %v dir %v", st.Mode().Perm(), dst.Mode().Perm())
	}
	_ = w.Close()
	if _, err := w.Append(logOf(1, "x")); err == nil {
		t.Fatal("append after close succeeded")
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestLogWriterBigLineAtTheFrameLimit(t *testing.T) {
	w, path := openTemp(t)
	big := strings.Repeat("x", protocol.MaxFrame-200)
	_, _ = w.Append(logOf(1, big))
	_, _ = w.Append(logOf(2, big))
	_, _ = w.Append(logOf(3, big))
	_ = w.Close()
	w2, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	if w2.Last() != 3 {
		t.Fatalf("tail = %d", w2.Last())
	}
}
