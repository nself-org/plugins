package cas

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewOpenReturnsVerifiedSnapshot(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("snapshot")
	d, _, err := s.Put(context.Background(), r, strings.NewReader("good bytes"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(r, d)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	p, _ := s.blobPath(r, d)
	if err := os.WriteFile(p, []byte("evil bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "good bytes" {
		t.Fatalf("served %q: %v", b, err)
	}
}

func TestReviewOpenHashDoesNotHoldMutationLock(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("parallel-reader")
	d, _, err := s.Put(context.Background(), r, strings.NewReader("reader"))
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	s.beforeHash = func() { close(entered); <-release }
	readDone := make(chan error, 1)
	go func() {
		f, e := s.Open(r, d)
		if e == nil {
			e = f.Close()
		}
		readDone <- e
	}()
	<-entered
	writeDone := make(chan error, 1)
	go func() { _, _, e := s.Put(context.Background(), r, strings.NewReader("writer")); writeDone <- e }()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("writer blocked on reader hashing")
	}
	close(release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
}

func TestReviewPartialRejectsChangedPublishedBytes(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("partial-race")
	d := digestOf([]byte("good bytes"))
	p, err := s.Partial(r, d, "writer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Write([]byte("good bytes")); err != nil {
		t.Fatal(err)
	}
	s.beforeRename = func() error { return os.WriteFile(p.data, []byte("evil bytes"), 0600) }
	if err := p.Finalize(); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("finalize: %v", err)
	}
	if _, err := s.Stat(r, d); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published corrupt blob: %v", err)
	}
}

func TestReviewPutRejectsSwappedShardSymlink(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("symlink-race")
	data := []byte("payload")
	d := digestOf(data)
	path, _ := s.blobPath(r, d)
	outside := t.TempDir()
	s.beforeRename = func() error {
		if err := os.Rename(filepath.Dir(path), filepath.Dir(path)+"-old"); err != nil {
			return err
		}
		return os.Symlink(outside, filepath.Dir(path))
	}
	if _, _, err := s.Put(context.Background(), r, bytes.NewReader(data)); err == nil {
		t.Fatal("published through swapped symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, d)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escaped: %v", err)
	}
}

func TestReviewSweepPreservesCrossStoreRefresh(t *testing.T) {
	s := testStore(t)
	other, _ := New(s.home)
	r := CacheRealm("refresh")
	data := []byte("refresh bytes")
	d, _, err := s.Put(context.Background(), r, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	path, _ := s.blobPath(r, d)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	sweepDone := make(chan error, 1)
	go func() {
		sweepDone <- s.Sweep(context.Background(), "cache", func(Realm, string) bool { close(entered); <-release; return false }, time.Now())
	}()
	<-entered
	putDone := make(chan error, 1)
	go func() { _, _, e := other.Put(context.Background(), r, bytes.NewReader(data)); putDone <- e }()
	close(release)
	if err := <-sweepDone; err != nil {
		t.Fatal(err)
	}
	if err := <-putDone; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(r, d); err != nil {
		t.Fatalf("refreshed blob absent: %v", err)
	}
}

func TestReviewHomeParentSymlinkAllowed(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	s, err := New(filepath.Join(link, "home"))
	if err != nil {
		t.Fatal(err)
	}
	r := CacheRealm("linked-parent")
	if _, _, err := s.Put(context.Background(), r, strings.NewReader("ok")); err != nil {
		t.Fatal(err)
	}
}

func TestReviewUnknownTempReportedAfterCleanup(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("cleanup")
	now := time.Now()
	dir := filepath.Join(s.blobs(), "tmp", "cache-invalid")
	if err := s.secureDir(dir); err != nil {
		t.Fatal(err)
	}
	p, err := s.Partial(r, digestOf([]byte("x")), "writer")
	if err != nil {
		t.Fatal(err)
	}
	old := now.Add(-25 * time.Hour)
	for _, name := range []string{p.data, p.offsetPath} {
		if err := os.Chtimes(name, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, now); err == nil || !strings.Contains(err.Error(), "cache-invalid") {
		t.Fatalf("unknown entry not reported: %v", err)
	}
	if _, err := os.Stat(p.data); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("valid temp not pruned: %v", err)
	}
}

func TestReviewQuarantineChecksInode(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("inode")
	d, _, err := s.Put(context.Background(), r, strings.NewReader("good"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.blobPath(r, d)
	old, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the old inode allocated; Linux may reuse it after Remove.
	if err := os.Rename(p, p+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.locked(func() error { return s.quarantine(p, old) }); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != "replacement" {
		t.Fatalf("moved replacement: %q %v", b, err)
	}
}

func TestReviewQuarantineFailureReturned(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("quarantine-failure")
	d, _, err := s.Put(context.Background(), r, strings.NewReader("good"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.blobPath(r, d)
	if err := os.WriteFile(p, []byte("evil"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(s.blobs(), "corrupt")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(r, d); !errors.Is(err, ErrInvalid) {
		t.Fatalf("quarantine failure hidden: %v", err)
	}
}

func TestReviewRestartTruncationFailureReturned(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("truncate-failure")
	d := digestOf([]byte("full"))
	p, err := s.Partial(r, d, "writer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Write([]byte("fu")); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p.data, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("uncommitted")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	want := errors.New("injected truncate failure")
	old := truncateFile
	truncateFile = func(string, int64) error { return want }
	t.Cleanup(func() { truncateFile = old })
	if _, err := s.Partial(r, d, "writer"); !errors.Is(err, want) {
		t.Fatalf("truncate failure hidden: %v", err)
	}
}

func TestReviewEmptyPartialResume(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("empty-resume")
	d := digestOf([]byte("later"))
	if _, err := s.Partial(r, d, "writer"); err != nil {
		t.Fatal(err)
	}
	old := truncateFile
	truncateFile = func(string, int64) error { return errors.New("unnecessary truncate") }
	t.Cleanup(func() { truncateFile = old })
	p, err := s.Partial(r, d, "writer")
	if err != nil || p.Offset() != 0 {
		t.Fatalf("empty restart: %v", err)
	}
}

func TestReviewPublishFaultLeavesNoBlob(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("publish-fault")
	data := []byte("fault")
	want := errors.New("injected publish fault")
	s.beforeRename = func() error { return want }
	if _, _, err := s.Put(context.Background(), r, bytes.NewReader(data)); !errors.Is(err, want) {
		t.Fatalf("fault hidden: %v", err)
	}
	if _, err := s.Stat(r, digestOf(data)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blob visible: %v", err)
	}
}

func TestReviewSweepWalkAndPruneErrors(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("walk-error")
	data := []byte("old")
	d, _, err := s.Put(context.Background(), r, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.blobPath(r, d)
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	want := errors.New("injected prune failure")
	prior := removeFile
	removeFile = func(string) error { return want }
	if err := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, time.Now()); !errors.Is(err, want) {
		t.Fatalf("prune failure hidden: %v", err)
	}
	removeFile = prior
	t.Cleanup(func() { removeFile = prior })
	if err := os.Symlink(t.TempDir(), filepath.Join(filepath.Dir(p), "bad")); err != nil {
		t.Fatal(err)
	}
	if err := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return true }, time.Now()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("walk failure hidden: %v", err)
	}
}

func TestReviewErrorCodeAndReasons(t *testing.T) {
	for _, e := range []*Error{ErrDigestMismatch, ErrSizeMismatch, ErrPartialConflict} {
		if e.Code() != "E710" || !strings.Contains(e.Error(), e.Reason) || !errors.Is(e, &Error{Reason: e.Reason}) || !errors.Is(e, e.Err) {
			t.Fatalf("invalid code: %v", e)
		}
		if errors.Is(e, &Error{Reason: "different"}) {
			t.Fatalf("reason collision: %v", e)
		}
	}
}

func TestReviewExactSweepExpiryBoundaries(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("expiry-boundary")
	now := time.Now().Truncate(time.Second)
	for _, tc := range []struct {
		name    string
		age     time.Duration
		expires bool
	}{
		{"young", blobGrace - time.Nanosecond, false}, {"exact", blobGrace, true},
	} {
		d, _, err := s.Put(context.Background(), r, strings.NewReader(tc.name))
		if err != nil {
			t.Fatal(err)
		}
		p, _ := s.blobPath(r, d)
		if err := os.Chtimes(p, now.Add(-tc.age), now.Add(-tc.age)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return false }, now); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		expired bool
	}{{"young", false}, {"exact", true}} {
		_, err := s.Stat(r, digestOf([]byte(tc.name)))
		if (err == nil) == tc.expired {
			t.Fatalf("blob %s boundary: %v", tc.name, err)
		}
	}
	for _, tc := range []struct {
		name    string
		age     time.Duration
		expired bool
	}{
		{"young", partialTTL - time.Nanosecond, false}, {"exact", partialTTL, true},
	} {
		p, err := s.Partial(r, digestOf([]byte(tc.name)), tc.name)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{p.data, p.offsetPath} {
			if err := os.Chtimes(path, now.Add(-tc.age), now.Add(-tc.age)); err != nil {
				t.Fatal(err)
			}
		}
	}
	corrupt := filepath.Join(s.blobs(), "corrupt")
	if err := s.secureDir(corrupt); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		age  time.Duration
	}{{"young", corruptTTL - time.Nanosecond}, {"exact", corruptTTL}} {
		p := filepath.Join(corrupt, tc.name)
		if err := os.WriteFile(p, []byte(tc.name), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-tc.age), now.Add(-tc.age)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Sweep(context.Background(), "cache", func(Realm, string) bool { return true }, now); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{filepath.Join(s.blobs(), "tmp", string(r)), corrupt} {
		for _, name := range []string{"young", "exact"} {
			p := filepath.Join(base, name)
			if base != corrupt {
				p = filepath.Join(base, digestOf([]byte(name))+"."+name+".partial")
			}
			_, err := os.Stat(p)
			if (err == nil) != (name == "young") {
				t.Fatalf("%s boundary: %v", p, err)
			}
		}
	}
}
