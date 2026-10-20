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

func writeLines(t *testing.T, path string, n int, textLen int) {
	t.Helper()
	w, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("y", textLen)
	for i := 1; i <= n; i++ {
		if _, err = w.Append(logOf(uint64(i), text)); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
}

func TestLogWriterReadsTheTailOfAFileLargerThanTheWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.jsonl")
	writeLines(t, path, 5000, 1000) // about 5 MiB, more than the 4 MiB window
	if st, _ := os.Stat(path); st.Size() <= tailWindow {
		t.Fatalf("file is %d bytes, want more than the window", st.Size())
	}
	w, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if w.Last() != 5000 {
		t.Fatalf("tail = %d, want 5000", w.Last())
	}
	_ = w.Close()
	// and with a torn tail beyond the window
	f, _ := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	_, _ = f.WriteString(`{"line_seq":5001,"stre`)
	_ = f.Close()
	w, err = OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.Last() != 5000 {
		t.Fatalf("tail after a torn line = %d", w.Last())
	}
	if ok, _ := w.Append(logOf(5001, "z")); !ok {
		t.Fatal("line 5001 not written")
	}
	b, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(b), `"text":"z"}`+"\n") || strings.Contains(string(b), `"stre{`) {
		t.Fatal("the torn fragment was not cut off")
	}
}

func TestLogWriterRefusesAFileWithoutACompleteLineInItsWindow(t *testing.T) {
	dir := t.TempDir()
	noNewline := filepath.Join(dir, "a.jsonl")
	if err := os.WriteFile(noNewline, []byte(strings.Repeat("x", tailWindow+10)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLog(noNewline); err == nil || !strings.Contains(err.Error(), "no complete line") {
		t.Fatalf("err = %v", err)
	}
	oneHugeLine := filepath.Join(dir, "b.jsonl")
	if err := os.WriteFile(oneHugeLine, []byte(strings.Repeat("x", tailWindow+10)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLog(oneHugeLine); err == nil || !strings.Contains(err.Error(), "no complete line") {
		t.Fatalf("err = %v", err)
	}
}

func TestLogWriterAWindowSizedFileIsReadWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edge.jsonl")
	line := `{"line_seq":7,"stream":"stdout","text":"` + strings.Repeat("e", tailWindow-len(`{"line_seq":7,"stream":"stdout","text":""}`)-1) + `"}` + "\n"
	if len(line) != tailWindow {
		t.Fatalf("fixture is %d bytes, want %d", len(line), tailWindow)
	}
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenLog(path)
	if err != nil {
		t.Fatalf("a file exactly as large as the window: %v", err)
	}
	defer w.Close()
	if w.Last() != 7 {
		t.Fatalf("tail = %d", w.Last())
	}
}

func TestLogWriterEmptyFileHasNoTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	w, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if w.Last() != 0 {
		t.Fatal("tail of an empty file")
	}
	if ok, _ := w.Append(logOf(1, "a")); !ok {
		t.Fatal("first line not written")
	}
	if ok, _ := w.Append(logOf(0, "a")); ok {
		t.Fatal("line_seq 0 is not above the tail")
	}
}

func TestLogWriterOpenFailures(t *testing.T) {
	if _, err := OpenLog("/dev/null/x/y.jsonl"); err == nil {
		t.Fatal("opened a log under a file")
	}
	dir := t.TempDir()
	if _, err := OpenLog(dir); err == nil { // a directory is not a log file
		t.Fatal("opened a directory")
	}
}

func TestLogWriterSyncOnlyWhenDirtyAndCloseSyncs(t *testing.T) {
	w, path := openTemp(t)
	if err := w.Sync(); err != nil || w.dirty {
		t.Fatal("sync of a clean writer")
	}
	_, _ = w.Append(logOf(1, "a"))
	if !w.dirty {
		t.Fatal("append did not mark the writer dirty")
	}
	if err := w.Sync(); err != nil || w.dirty {
		t.Fatalf("sync: %v dirty=%v", err, w.dirty)
	}
	_, _ = w.Append(logOf(2, "b"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatalf("sync after close = %v", err)
	}
	if b, _ := os.ReadFile(path); strings.Count(string(b), "\n") != 2 {
		t.Fatal("lines lost")
	}
}

func TestLogWriterFailedWriteLeavesNoHalfLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ro.jsonl")
	if err := os.WriteFile(path, []byte(`{"line_seq":1,"stream":"stdout","text":"a"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ro, err := os.Open(path) // writes to a read-only descriptor fail
	if err != nil {
		t.Fatal(err)
	}
	w := &LogWriter{f: ro, last: 1}
	if ok, err := w.Append(logOf(2, "b")); ok || err == nil {
		t.Fatalf("append on a read-only file: ok=%v err=%v", ok, err)
	}
	if w.Last() != 1 || w.dirty {
		t.Fatalf("a failed append changed the writer: last %d dirty %v", w.Last(), w.dirty)
	}
	_ = ro.Close()
}

func TestLogWriterStopsAtTheCapWithOneTruncationNote(t *testing.T) {
	old := logCap
	logCap = 400
	t.Cleanup(func() { logCap = old })
	w, path := openTemp(t)
	var wrote int
	for i := uint64(1); i <= 40; i++ {
		ok, err := w.Append(logOf(i, strings.Repeat("x", 30)))
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			wrote++
		}
	}
	if wrote == 0 || wrote >= 40 {
		t.Fatalf("%d of 40 lines were written under a cap of 400 bytes", wrote)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "[log truncated"); n != 1 {
		t.Fatalf("%d truncation notes:\n%s", n, b)
	}
	if lines := strings.Count(string(b), "\n"); lines != wrote+1 {
		t.Fatalf("%d lines in the file, want %d lines and one note", lines, wrote)
	}
	if int64(len(b)) > logCap+200 {
		t.Fatalf("the file is %d bytes, cap %d", len(b), logCap)
	}
	// a later line with a higher seq is still dropped, and the note is not repeated
	if ok, _ := w.Append(logOf(99, "late")); ok {
		t.Fatal("a line was written after the cap")
	}
}

func TestLogWriterDoesNotEscapeHTMLSoALineStaysAsShortAsItsFrame(t *testing.T) {
	w, path := openTemp(t)
	if _, err := w.Append(logOf(1, "<a>&</a>")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	escaped := "\\" + "u003c" // built in code: the escape must not be a literal in the source
	if !strings.Contains(string(b), `"<a>&</a>"`) || strings.Contains(string(b), escaped) {
		t.Fatalf("log line %q", b)
	}
}

func TestLogWriterFindsTheLastLineWhenTheWindowStartsOnTheNewlineBeforeIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "edge.jsonl")
	first := `{"line_seq":1,"stream":"stdout","text":"a"}` + "\n"
	head, tail := `{"line_seq":2,"stream":"stdout","text":"`, `"}`+"\n"
	pad := strings.Repeat("p", tailWindow-1-len(head)-len(tail)) // the last line is one byte short of the window
	if err := os.WriteFile(path, []byte(first+head+pad+tail), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenLog(path)
	if err != nil {
		t.Fatalf("the window starts on the newline that ends line 1: %v", err)
	}
	defer w.Close()
	if w.Last() != 2 {
		t.Fatalf("tail = %d, want 2", w.Last())
	}
}

func TestLogWriterAcceptsALineThatFillsTheCapExactly(t *testing.T) {
	old := logCap
	t.Cleanup(func() { logCap = old })
	probe, probePath := openTemp(t)
	if _, err := probe.Append(logOf(1, "xxxx")); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(probePath)
	logCap = 2 * st.Size()
	w, path := openTemp(t)
	for i := uint64(1); i <= 2; i++ {
		if ok, err := w.Append(logOf(i, "xxxx")); err != nil || !ok {
			t.Fatalf("line %d: written=%v err=%v (two lines fill the cap exactly)", i, ok, err)
		}
	}
	if ok, _ := w.Append(logOf(3, "xxxx")); ok {
		t.Fatal("a third line went past the cap")
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "[log truncated") {
		t.Fatal("no truncation note")
	}
}

func TestLogWriterKeepsNothingAfterTheCapNoteAcrossARestart(t *testing.T) {
	old := logCap
	logCap = 300
	t.Cleanup(func() { logCap = old })
	w, path := openTemp(t)
	for i := uint64(1); i <= 12; i++ {
		_, _ = w.Append(logOf(i, strings.Repeat("x", 20)))
	}
	if ok, _ := w.Append(logOf(13, "s")); ok { // a short line that would fit under the cap is still dropped
		t.Fatal("a line was written after the note")
	}
	_ = w.Close()
	w2, err := OpenLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	for i := uint64(14); i <= 30; i++ {
		_, _ = w2.Append(logOf(i, strings.Repeat("x", 20)))
	}
	b, _ := os.ReadFile(path)
	if n := strings.Count(string(b), "[log truncated"); n != 1 {
		t.Fatalf("%d truncation notes after a restart:\n%s", n, b)
	}
}

func TestLogWriterExpandedLineFitsTheTailWindow(t *testing.T) {
	for _, tc := range []struct {
		name string
		unit string
	}{
		{"invalid UTF-8", "\xff"},
		{"Unicode separator", "\u2028"},
		{"escaped control", "\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "wide.jsonl")
			w, err := OpenLog(path)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.Repeat(tc.unit, (protocol.MaxFrame-200)/len(tc.unit))
			if ok, err := w.Append(logOf(1, text)); err != nil || !ok {
				t.Fatalf("append: %v %v", ok, err)
			}
			_ = w.Close()
			w, err = OpenLog(path)
			if err != nil {
				t.Fatalf("a widened line must still be found after a restart: %v", err)
			}
			defer w.Close()
			if w.Last() != 1 {
				t.Fatalf("tail %d", w.Last())
			}
		})
	}
}
