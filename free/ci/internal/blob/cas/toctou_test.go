package cas

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockSwapRefusesSymlink(t *testing.T) {
	s := testStore(t)
	if err := s.locked(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	dummy := filepath.Join(s.blobs(), "dummy")
	if err := os.WriteFile(dummy, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	s.afterLockStat = func() {
		if err := os.Remove(filepath.Join(s.blobs(), ".lock")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("dummy", filepath.Join(s.blobs(), ".lock")); err != nil {
			t.Fatal(err)
		}
	}
	ran := false
	if err := s.locked(func() error { ran = true; return nil }); err == nil || ran {
		t.Fatalf("swapped lock accepted: %v, ran=%v", err, ran)
	}
}

func TestPartialTruncateSwapRefusesSymlink(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("truncate-swap")
	d := digestOf([]byte("good"))
	p, err := s.Partial(r, d, "writer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Write([]byte("go")); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p.data, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("uncommitted"); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err = os.WriteFile(outside, []byte("must survive"), 0600); err != nil {
		t.Fatal(err)
	}
	s.afterPartialStat = func() { swapToSymlink(t, p.data, outside) }
	if _, err = s.Partial(r, d, "writer"); err == nil {
		t.Fatal("swapped partial was truncated")
	}
	assertOutside(t, outside)
}

func TestPartialAppendSwapRefusesSymlink(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("append-swap")
	p, err := s.Partial(r, digestOf([]byte("good")), "writer")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err = os.WriteFile(outside, []byte("must survive"), 0600); err != nil {
		t.Fatal(err)
	}
	s.afterPartialStat = func() { swapToSymlink(t, p.data, outside) }
	if _, err = p.Write([]byte("evil")); err == nil {
		t.Fatal("swapped partial was appended")
	}
	assertOutside(t, outside)
}

func swapToSymlink(t *testing.T, path, target string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

func assertOutside(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || !strings.EqualFold(string(b), "must survive") {
		t.Fatalf("outside changed: %q, %v", b, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
