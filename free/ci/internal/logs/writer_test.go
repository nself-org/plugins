package logs

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mk(seq int64, text string) Line { return Line{TS: "t", Stream: "out", Seq: seq, Text: text} }

func readAll(t *testing.T, dir string) []Line {
	t.Helper()
	var out []Line
	if err := Read(dir, 0, func(l Line) error { out = append(out, l); return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertSeq(t *testing.T, got []Line, n int) {
	t.Helper()
	if len(got) != n {
		t.Fatalf("got %d lines, want %d", len(got), n)
	}
	for i, l := range got {
		if l.Seq != int64(i+1) {
			t.Fatalf("line %d has seq %d", i, l.Seq)
		}
	}
}

func TestAppendAcksAfterFsyncAndDropsReplays(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run", "att")
	w, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := w.AppendBatch([]Line{mk(1, "a"), mk(2, "b")}); err != nil || got != 2 {
		t.Fatalf("ack %d, %v", got, err)
	}
	if got, err := w.AppendBatch([]Line{mk(1, "a"), mk(2, "b"), mk(3, "c")}); err != nil || got != 3 {
		t.Fatalf("a replay with one new line: ack %d, %v", got, err)
	}
	if got, err := w.Append(mk(2, "dup")); err != nil || got != 3 {
		t.Fatalf("a duplicate must be dropped without error: ack %d, %v", got, err)
	}
	_, err = w.Append(mk(9, "gap"))
	if e, ok := AsError(err); !ok || e.Code != CodeStore {
		t.Fatalf("a gap must be E632, got %v", err)
	}
	if w.LastSeq() != 3 {
		t.Fatalf("last = %d after a refused gap", w.LastSeq())
	}
	_ = w.Close()
	got := readAll(t, dir)
	assertSeq(t, got, 3)
	if got[0].Text != "a" || got[2].Text != "c" {
		t.Fatalf("texts: %+v", got)
	}
}

func TestOpenResumesFromTheFileAndCutsATornTail(t *testing.T) {
	dir := t.TempDir()
	w, _ := Open(dir, Options{})
	_, _ = w.AppendBatch([]Line{mk(1, "a"), mk(2, "b")})
	_ = w.Close()
	f, _ := os.OpenFile(filepath.Join(dir, activeName), os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString(`{"ts":"t","stream":"out","seq":3,"te`) // kill -9 mid-write: no newline
	_ = f.Close()
	w, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if w.LastSeq() != 2 {
		t.Fatalf("tail seq %d, want 2", w.LastSeq())
	}
	if _, err = w.AppendBatch([]Line{mk(3, "c")}); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	assertSeq(t, readAll(t, dir), 3)
}

func TestOpenRefusesACorruptMiddle(t *testing.T) {
	dir := t.TempDir()
	body := `{"ts":"t","stream":"out","seq":1,"text":"a"}` + "\n" + "not json\n" + `{"ts":"t","stream":"out","seq":3,"text":"c"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, activeName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, Options{}); err == nil || !strings.Contains(err.Error(), CodeStore) {
		t.Fatalf("want E632, got %v", err)
	}
}

func TestRotationGzipsAndReadSpansSegments(t *testing.T) {
	dir := t.TempDir()
	w, _ := Open(dir, Options{SegmentBytes: 300})
	for i := int64(1); i <= 40; i++ {
		if _, err := w.Append(mk(i, strings.Repeat("x", 20))); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	segs, _ := segments(dir)
	if len(segs) < 3 {
		t.Fatalf("expected several segments, got %d", len(segs))
	}
	assertSeq(t, readAll(t, dir), 40)
	f, _ := os.Open(segs[0].Path)
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("segment is not gzip: %v", err)
	}
	if b, _ := io.ReadAll(zr); int64(len(b)) != segs[0].Bytes {
		t.Fatalf("segment name says %d raw bytes, holds %d", segs[0].Bytes, len(b))
	}
	var after []Line
	_ = Read(dir, 35, func(l Line) error { after = append(after, l); return nil })
	if len(after) != 5 || after[0].Seq != 36 {
		t.Fatalf("Read(after=35): %d lines from %d", len(after), after[0].Seq)
	}
	w, err = Open(dir, Options{SegmentBytes: 300})
	if err != nil || w.LastSeq() != 40 {
		t.Fatalf("reopen: last %d, %v", w.LastSeq(), err)
	}
}

// A crash after the segment was renamed but before the old active file went:
// the duplicate lines in the active file are recognised and dropped.
func TestOpenDropsAnArchivedActiveFile(t *testing.T) {
	dir := t.TempDir()
	w, _ := Open(dir, Options{})
	_, _ = w.AppendBatch([]Line{mk(1, "a"), mk(2, "b")})
	_ = w.Close()
	active := filepath.Join(dir, activeName)
	size := mustSize(t, active)
	if err := gzipFile(active, filepath.Join(dir, segmentName(1, 2, size))); err != nil {
		t.Fatal(err)
	}
	w, err := Open(dir, Options{})
	if err != nil || w.LastSeq() != 2 {
		t.Fatalf("last %d, %v", w.LastSeq(), err)
	}
	_, _ = w.Append(mk(3, "c"))
	_ = w.Close()
	assertSeq(t, readAll(t, dir), 3)
}

func mustSize(t *testing.T, p string) int64 {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}

func TestSizeCapEndsTheLogWithANote(t *testing.T) {
	dir := t.TempDir()
	w, _ := Open(dir, Options{MaxBytes: 400})
	for i := int64(1); i <= 30; i++ {
		_, _ = w.Append(mk(i, strings.Repeat("y", 30)))
	}
	if !w.Capped() {
		t.Fatal("not capped")
	}
	_ = w.Close()
	got := readAll(t, dir)
	last := got[len(got)-1]
	if last.Text != CapNote || last.Stream != "sys" {
		t.Fatalf("last line %+v", last)
	}
	if len(got) > 20 {
		t.Fatalf("the cap kept %d lines", len(got))
	}
	w, _ = Open(dir, Options{MaxBytes: 400})
	if !w.Capped() {
		t.Fatal("a reopened capped log must stay capped")
	}
}

func TestFilesAndDirsArePrivate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs", "run1", "att1")
	w, err := Open(root, Options{SegmentBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 10; i++ {
		_, _ = w.Append(mk(i, strings.Repeat("z", 30)))
	}
	_ = w.Close()
	_ = filepath.Walk(filepath.Dir(filepath.Dir(root)), func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if perm := fi.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s is %v: readable by others", p, perm)
		}
		return nil
	})
}

func TestValidIDRejectsPathTricks(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../x", "a/b", `a\b`, ".hidden", "a..b", strings.Repeat("a", 129)} {
		if ValidID(bad) {
			t.Errorf("%q must not be a valid id", bad)
		}
	}
	for _, ok := range []string{"r20261018T101010-ab12cd34", "r1.job-abcd1234.a1"} {
		if !ValidID(ok) {
			t.Errorf("%q must be valid", ok)
		}
	}
}

// A planted symlink must not redirect the log or its recovery truncation, and
// loose permissions on existing objects are tightened.
func TestOpenRefusesSymlinksAndTightensPermissions(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(victim, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "run", "att")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, activeName)); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, Options{}); err == nil {
		t.Fatal("an active.jsonl symlink must be refused")
	}
	if b, _ := os.ReadFile(victim); string(b) != "precious" {
		t.Fatalf("the link target was touched: %q", b)
	}
	_ = os.Remove(filepath.Join(dir, activeName))
	w, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	for _, p := range []string{dir, filepath.Dir(dir), filepath.Join(dir, activeName)} {
		if fi, _ := os.Stat(p); fi.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s stayed %v", p, fi.Mode().Perm())
		}
	}
	link := filepath.Join(root, "linkdir")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link, Options{}); err == nil {
		t.Fatal("a symlinked attempt directory must be refused")
	}
}

func TestRemoveRunDoesNotFollowASymlinkedLogsDirectory(t *testing.T) {
	home := t.TempDir()
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "r1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, Root(home)); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRun(home, "r1"); err == nil {
		t.Fatal("must refuse")
	}
	if _, err := os.Stat(filepath.Join(other, "r1")); err != nil {
		t.Fatalf("the linked directory lost data: %v", err)
	}
}
