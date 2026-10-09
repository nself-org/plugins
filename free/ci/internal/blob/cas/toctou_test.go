package cas

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSweepTempSwappedWalkRootRefusesPublishedBlob(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("swapped-temp-walk-root")
	d, _, err := s.Put(context.Background(), r, strings.NewReader("published"))
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := s.blobPath(r, d)
	now := time.Now()
	old := now.Add(-25 * time.Hour)
	if err := os.Chtimes(blob, old, old); err != nil {
		t.Fatal(err)
	}
	tmpRealm := filepath.Join(s.blobs(), "tmp", string(r))
	if err := s.secureDir(tmpRealm); err != nil {
		t.Fatal(err)
	}
	s.afterTempReadDir = func() {
		if err := os.Rename(tmpRealm, tmpRealm+"-saved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", string(r), "sha256"), tmpRealm); err != nil {
			t.Fatal(err)
		}
	}
	err = s.Sweep(context.Background(), "cache", func(Realm, string) bool { return true }, now)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("swapped temp walk root accepted: %v", err)
	}
	if _, err := os.Stat(blob); err != nil {
		t.Fatalf("published blob deleted: %v", err)
	}
}

func TestSweepTempNestedSymlinkRefusesPublishedBlob(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("nested-temp-walk-link")
	d, _, err := s.Put(context.Background(), r, strings.NewReader("published"))
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := s.blobPath(r, d)
	now := time.Now()
	old := now.Add(-25 * time.Hour)
	if err := os.Chtimes(blob, old, old); err != nil {
		t.Fatal(err)
	}
	tmpRealm := filepath.Join(s.blobs(), "tmp", string(r))
	if err := s.secureDir(tmpRealm); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", string(r), "sha256"), filepath.Join(tmpRealm, "nested")); err != nil {
		t.Fatal(err)
	}
	err = s.Sweep(context.Background(), "cache", func(Realm, string) bool { return true }, now)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("nested temp link accepted: %v", err)
	}
	if _, err := os.Stat(blob); err != nil {
		t.Fatalf("published blob deleted: %v", err)
	}
}

func TestSweepTempSwappedNestedEntryRefusesPublishedBlob(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("swapped-nested-temp-entry")
	d, _, err := s.Put(context.Background(), r, strings.NewReader("published"))
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := s.blobPath(r, d)
	now := time.Now()
	old := now.Add(-25 * time.Hour)
	if err := os.Chtimes(blob, old, old); err != nil {
		t.Fatal(err)
	}
	tmpRealm := filepath.Join(s.blobs(), "tmp", string(r))
	nested := filepath.Join(tmpRealm, "nested")
	if err := s.secureDir(nested); err != nil {
		t.Fatal(err)
	}
	s.afterWalkReadDir = func(path string) {
		if path != tmpRealm {
			return
		}
		if err := os.Rename(nested, nested+"-saved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "..", string(r), "sha256"), nested); err != nil {
			t.Fatal(err)
		}
	}
	err = s.Sweep(context.Background(), "cache", func(Realm, string) bool { return true }, now)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("swapped nested entry accepted: %v", err)
	}
	if _, err := os.Stat(blob); err != nil {
		t.Fatalf("published blob deleted: %v", err)
	}
}

func TestSweepTempSwappedAncestorRefusesPrune(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("swapped-temp-ancestor")
	tmpRealm := filepath.Join(s.blobs(), "tmp", string(r))
	nested := filepath.Join(tmpRealm, "nested")
	if err := s.secureDir(nested); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(nested, "orphan")
	if err := os.WriteFile(oldFile, []byte("must survive"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-25 * time.Hour)
	if err := os.Chtimes(oldFile, old, old); err != nil {
		t.Fatal(err)
	}
	s.afterWalkReadDir = func(path string) {
		if path != tmpRealm {
			return
		}
		if err := os.Rename(tmpRealm, tmpRealm+"-saved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Base(tmpRealm)+"-saved", tmpRealm); err != nil {
			t.Fatal(err)
		}
	}
	err := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, now)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("swapped walk ancestor accepted: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(tmpRealm+"-saved", "nested", "orphan")); err != nil || string(b) != "must survive" {
		t.Fatalf("orphan outside requested walk removed: %q, %v", b, err)
	}
}

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
