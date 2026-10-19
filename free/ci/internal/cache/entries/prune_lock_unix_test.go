//go:build unix

package entries_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestPruneBudgetBoundsTheWaitForTheBlobTreeLock: another process holds the cas
// lock (blobs/.lock); Prune returns More when its budget ends instead of waiting.
func TestPruneBudgetBoundsTheWaitForTheBlobTreeLock(t *testing.T) {
	e := newEnv(t, envOpts{})
	e.save("lk1", trusted, "k", "data") // creates the blob tree and its lock file
	f, err := os.OpenFile(filepath.Join(e.home, "blobs", ".lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	res, err := e.es.Prune(e.ctx, 300*time.Millisecond)
	if err != nil || !res.More {
		t.Fatalf("a blocked sweep must report More, not fail: %+v %v", res, err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("Prune waited %v for a lock held elsewhere", d)
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if res, err = e.es.Prune(e.ctx, 10*time.Second); err != nil || res.More {
		t.Fatalf("after the lock is released: %+v %v", res, err)
	}
}
