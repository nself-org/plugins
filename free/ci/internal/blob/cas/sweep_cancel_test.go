package cas

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// D-1010: Sweep must give up on a contended cross-process lock when its context ends.
func TestSweepLockWaitHonoursContext(t *testing.T) {
	dir := t.TempDir()
	holder, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	sweeper, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	held, release := make(chan struct{}), make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- holder.locked(func() error { close(held); <-release; return nil })
	}()
	<-held
	defer func() { close(release); <-holderDone }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- sweeper.Sweep(ctx, "cache", func(Realm, string) bool { return false }, time.Now())
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Sweep error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Sweep kept waiting on the lock after its context ended")
	}
}

// D-1010: Sweep must notice cancellation per file, not only between realms.
func TestSweepStopsMidRealmOnCancel(t *testing.T) {
	s := testStore(t)
	r := CacheRealm("big")
	now := time.Now()
	for i := 0; i < 5; i++ {
		d, _, err := s.Put(context.Background(), r, strings.NewReader(strings.Repeat("x", i+1)))
		if err != nil {
			t.Fatal(err)
		}
		p, _ := s.blobPath(r, d)
		if err = os.Chtimes(p, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := s.Sweep(ctx, "cache", func(Realm, string) bool { calls++; cancel(); return true }, now)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Sweep error = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("keep called %d times after cancel, want 1", calls)
	}
}
