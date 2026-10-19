package logs

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nself-org/plugins/free/ci/internal/redact"
)

func newTestSink(t *testing.T, opt Options) (*Sink, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "att")
	w, err := Open(dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	return NewSink(w, "out"), dir
}

func texts(ls []Line) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
}

func TestSinkSplitsChunksIntoLines(t *testing.T) {
	s, dir := newTestSink(t, Options{})
	for _, c := range []string{"one\r\ntw", "o\n\nthr", "ee"} {
		_, _ = s.Write([]byte(c))
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	got := readAll(t, dir)
	assertSeq(t, got, 4)
	if want := []string{"one", "two", "", "three"}; strings.Join(texts(got), "|") != strings.Join(want, "|") {
		t.Fatalf("lines %q, want %q", texts(got), want)
	}
	if got[0].Stream != "out" || got[0].TS == "" {
		t.Fatalf("line fields: %+v", got[0])
	}
}

func TestSinkSplitsLongLinesOnRuneBoundaries(t *testing.T) {
	s, dir := newTestSink(t, Options{MaxLine: 10})
	_, _ = s.Write([]byte(strings.Repeat("é", 12) + "\n")) // 24 bytes of 2-byte runes
	_ = s.Close()
	got := readAll(t, dir)
	if len(got) < 3 {
		t.Fatalf("a 24-byte line with MaxLine 10 gave %d lines", len(got))
	}
	var all string
	for _, l := range got {
		if len(l.Text) > 10 || !utf8.ValidString(l.Text) {
			t.Fatalf("bad piece %q", l.Text)
		}
		all += l.Text
	}
	if all != strings.Repeat("é", 12) {
		t.Fatalf("pieces lost bytes: %q", all)
	}
}

func TestSinkFlushesOnATimerAndPartialLinesLater(t *testing.T) {
	old1, old2 := FlushEvery, PartialAfter
	FlushEvery, PartialAfter = 20*time.Millisecond, 80*time.Millisecond
	defer func() { FlushEvery, PartialAfter = old1, old2 }()
	s, dir := newTestSink(t, Options{})
	_, _ = s.Write([]byte("done line\nprogress 50%"))
	deadline := time.Now().Add(2 * time.Second)
	var got []Line
	for time.Now().Before(deadline) {
		got = nil
		_ = Read(dir, 0, func(l Line) error { got = append(got, l); return nil })
		if len(got) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) != 2 || got[0].Text != "done line" || got[1].Text != "progress 50%" {
		t.Fatalf("before Close the log holds %q", texts(got))
	}
	_ = s.Close()
}

func TestSinkSurvivesAStorageErrorWithoutFailingTheJob(t *testing.T) {
	s, dir := newTestSink(t, Options{})
	_, _ = s.Write([]byte("kept\n"))
	s.mu.Lock()
	s.flush(false)
	_ = s.w.f.Close() // the disk goes away
	s.mu.Unlock()
	if n, err := s.Write([]byte("lost\n")); n != 5 || err != nil {
		t.Fatalf("Write must never fail the job: %d %v", n, err)
	}
	if err := s.Close(); err == nil {
		t.Fatal("Close must report the storage error")
	}
	if got := readAll(t, dir); len(got) != 1 || got[0].Text != "kept" {
		t.Fatalf("stored %q", texts(got))
	}
}

// slurp returns every stored byte of dir: the active file and the unzipped segments.
func slurp(t *testing.T, dir string) string {
	t.Helper()
	var all bytes.Buffer
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(e.Name(), segSuffix) {
			zr, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			if b, err = io.ReadAll(zr); err != nil {
				t.Fatal(err)
			}
		}
		all.Write(b)
	}
	return all.String()
}

// The sink sits behind the redactor: a planted value, raw or split across
// chunks, never reaches a stored byte. The control writes the same bytes past
// no redactor and finds the value, so the scan can see it.
func TestPlantedSecretNeverReachesStoredLogs(t *testing.T) {
	const secret = "planted-token-5e8c1b77d2"
	reg := redact.New()
	if err := reg.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	chunks := []string{"start\nraw=" + secret + "\n", "split=planted-tok", "en-5e8c1b77d2\nend\n"}

	clean, cleanDir := newTestSink(t, Options{SegmentBytes: 120})
	f := reg.NewFilter()
	for _, c := range chunks {
		_, _ = clean.Write(f.Redact([]byte(c)))
	}
	_, _ = clean.Write(f.Flush())
	_ = clean.Close()
	if got := slurp(t, cleanDir); strings.Contains(got, secret) || strings.Contains(got, "planted-tok") || !strings.Contains(got, redact.Placeholder) {
		t.Fatalf("stored log after the redactor: %q", got)
	}

	control, controlDir := newTestSink(t, Options{})
	for _, c := range chunks {
		_, _ = control.Write([]byte(c))
	}
	_ = control.Close()
	if !strings.Contains(slurp(t, controlDir), secret) {
		t.Fatal("positive control: the scan cannot see an unredacted secret")
	}
}
