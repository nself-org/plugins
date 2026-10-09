package cas

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type emptyReader struct{ reads int }

func (r *emptyReader) Read([]byte) (int, error) {
	r.reads++
	if r.reads > 200 {
		return 0, io.EOF
	}
	return 0, nil
}

func TestPutRejectsNoProgressReader(t *testing.T) {
	s := testStore(t)
	reader := &emptyReader{}
	if _, _, err := s.Put(context.Background(), CacheRealm("no-progress"), reader); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("zero progress: %v", err)
	}
	if reader.reads != 100 {
		t.Fatalf("progress limit: %d reads", reader.reads)
	}
}

func TestPartialRestartTruncatesUncommittedBytes(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("recovery")
	p, err := s.Partial(r, digestOf([]byte("good")), "writer")
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
	p, err = s.Partial(r, p.digest, "writer")
	if err != nil || p.Offset() != 2 {
		t.Fatalf("restart: %v", err)
	}
	b, err := os.ReadFile(p.data)
	if err != nil || string(b) != "go" {
		t.Fatalf("uncommitted bytes remained: %q, %v", b, err)
	}
}

func TestPartialFinalizeReportsPublishFailure(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("partial-fault")
	p, err := s.Partial(r, digestOf([]byte("good")), "writer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Write([]byte("good")); err != nil {
		t.Fatal(err)
	}
	want := errors.New("publish failure")
	s.beforeRename = func() error { return want }
	if err = p.Finalize(); !errors.Is(err, want) {
		t.Fatalf("publish failure hidden: %v", err)
	}
	if _, err = s.Stat(r, p.digest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blob visible: %v", err)
	}
}

func TestOpenReportsLockFailureDuringQuarantine(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("quarantine-lock")
	d, _, err := s.Put(context.Background(), r, strings.NewReader("good"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.blobPath(r, d)
	if err = os.WriteFile(p, []byte("evil"), 0600); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(s.blobs(), ".lock")
	if err = os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink("missing", lock); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Open(r, d); err == nil || errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("lock failure hidden: %v", err)
	}
}

func TestPutReportsLockFailure(t *testing.T) {
	s := testStore(t)
	if err := s.locked(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(s.blobs(), ".lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", lock); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Put(context.Background(), CacheRealm("put-lock"), strings.NewReader("data")); err == nil {
		t.Fatal("lock failure hidden")
	}
}

func TestSweepContinuesAfterMissingRealmData(t *testing.T) {
	s := testStore(t)
	a, b := CacheRealm("missing-a"), CacheRealm("missing-b")
	if string(a) > string(b) {
		a, b = b, a
	}
	if err := s.secureDir(filepath.Join(s.blobs(), string(a))); err != nil {
		t.Fatal(err)
	}
	d, _, err := s.Put(context.Background(), b, strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.blobPath(b, d)
	now := time.Now()
	if err = os.Chtimes(p, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, now); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Stat(b, d); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old blob remained: %v", err)
	}
}

func TestBlobRootSymlinkRefused(t *testing.T) {
	s := testStore(t)
	outside := t.TempDir()
	r := CacheRealm("outside")
	d := digestOf([]byte("x"))
	if err := os.MkdirAll(filepath.Join(outside, string(r), "sha256", d[:2]), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, s.blobs()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(r, d); !errors.Is(err, ErrInvalid) {
		t.Fatalf("followed blob root: %v", err)
	}
}

func TestInvalidSweepKindRejected(t *testing.T) {
	s := testStore(t)
	for _, kind := range []string{"artifact", "other", ""} {
		if err := s.Sweep(context.Background(), kind, func(Realm, string) bool { return false }, time.Now()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("kind %q: %v", kind, err)
		}
	}
}

func TestTemporaryNameCollisionLimit(t *testing.T) {
	s := testStore(t)
	root, err := s.tmpRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	_ = root.Close()
	prior := randomName
	calls := 0
	randomName = func() (string, error) {
		calls++
		if calls > 3 {
			return "d", nil
		}
		return []string{"a", "b", "c"}[calls-1], nil
	}
	t.Cleanup(func() { randomName = prior })
	if _, _, err := s.temp(); err == nil || calls != 3 {
		t.Fatalf("collision limit: %v, calls=%d", err, calls)
	}
}

func TestSnapshotNameCollisionLimit(t *testing.T) {
	s := testStore(t)
	root, err := s.tmpRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		f, err := root.OpenFile("snapshot-"+name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	_ = root.Close()
	prior := randomName
	calls := 0
	randomName = func() (string, error) {
		calls++
		if calls > 3 {
			return "d", nil
		}
		return []string{"a", "b", "c"}[calls-1], nil
	}
	t.Cleanup(func() { randomName = prior })
	if _, err := s.snapshot(); err == nil || calls != 3 {
		t.Fatalf("snapshot collision limit: %v, calls=%d", err, calls)
	}
}
