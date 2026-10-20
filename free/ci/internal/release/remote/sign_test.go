package remote

// Purpose: the small helpers of Sign: text from the coordinator is made printable, and a work
// directory an earlier killed run left behind is swept once it is stale.
// Constraints: temp dirs from t.TempDir only.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestShownMakesCoordinatorTextPrintable(t *testing.T) {
	got := shown("\x1b[2J\x1b[31mlocal:abc\r\n" + strings.Repeat("é", 200))
	if strings.ContainsAny(got, "\x1b\r\n") || utf8.RuneCountInString(got) != 80 || !utf8.ValidString(got) {
		t.Fatalf("%q", got)
	}
}

func TestSweepStaleRemovesOnlyOldWorkDirectories(t *testing.T) {
	home := t.TempDir()
	mk := func(name string, age time.Duration) string {
		p := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Join(p, "blobs"), 0o700); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-age)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stale, fresh := mk(workPrefix+"old", 7*time.Hour), mk(workPrefix+"new", time.Minute)
	other := mk("trust", 30*24*time.Hour) // not ours, however old
	file := filepath.Join(home, workPrefix+"file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sweepStale(home, time.Now())
	for p, want := range map[string]bool{stale: false, fresh: true, other: true, file: true} {
		if _, err := os.Lstat(p); (err == nil) != want {
			t.Errorf("%s: exists=%v, want %v", filepath.Base(p), err == nil, want)
		}
	}
}
