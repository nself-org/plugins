package internal

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAdvisoryLock_FileFallback tests that FileLock works correctly when DB is unavailable.
func TestAdvisoryLock(t *testing.T) {
	// FileLock is the unit-testable layer (no real DB needed).
	fl := NewFileLock(t.TempDir())

	ctx := context.Background()
	acquired, release, err := fl.TryLock(ctx, "test-job")
	if err != nil {
		t.Fatalf("TryLock returned unexpected error: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}
	if release == nil {
		t.Fatal("expected non-nil release func")
	}
	release()
}

// TestAdvisoryLockConcurrent verifies that the second concurrent run is skipped.
func TestAdvisoryLockConcurrent(t *testing.T) {
	fl := NewFileLock(t.TempDir())
	ctx := context.Background()

	// First acquisition.
	acq1, rel1, err := fl.TryLock(ctx, "job-concurrent")
	if err != nil {
		t.Fatalf("first TryLock error: %v", err)
	}
	if !acq1 {
		t.Fatal("first lock should be acquired")
	}
	defer rel1()

	// Second attempt while first is held — should not acquire.
	acq2, rel2, err := fl.TryLock(ctx, "job-concurrent")
	if err != nil {
		t.Fatalf("second TryLock error: %v", err)
	}
	if acq2 {
		t.Error("second lock should NOT be acquired (previous instance running)")
		if rel2 != nil {
			rel2()
		}
	}
}

// TestAdvisoryLockFallback verifies that the OverlapCounter increments when overlap is detected.
func TestAdvisoryLockFallback(t *testing.T) {
	counter := NewOverlapCounter()

	var hookCalls int64
	counter.IncrementHook = func(jobName string) {
		atomic.AddInt64(&hookCalls, 1)
	}

	counter.Increment("overlap-job")
	counter.Increment("overlap-job")

	if counter.Count("overlap-job") != 2 {
		t.Errorf("expected count=2, got %d", counter.Count("overlap-job"))
	}
	if atomic.LoadInt64(&hookCalls) != 2 {
		t.Errorf("expected 2 hook calls, got %d", hookCalls)
	}
}

// TestAdvisoryLockReleaseAllowsReacquire verifies that after release, the lock can be re-acquired.
func TestAdvisoryLockReleaseAllowsReacquire(t *testing.T) {
	fl := NewFileLock(t.TempDir())
	ctx := context.Background()

	acq1, rel1, _ := fl.TryLock(ctx, "release-job")
	if !acq1 {
		t.Fatal("first lock should be acquired")
	}
	rel1()

	// After release, re-acquire should succeed.
	acq2, rel2, _ := fl.TryLock(ctx, "release-job")
	if !acq2 {
		t.Fatal("re-acquire after release should succeed")
	}
	rel2()
}

// TestSkipIfOverlapping integration: simulates LockManager with FileLock fallback.
func TestSkipIfOverlapping(t *testing.T) {
	counter := NewOverlapCounter()

	// Use a FileLock-backed LockManager (no DB).
	fm := &LockManager{
		advisory: &AdvisoryLock{Pool: nil}, // nil pool → will always fail
		file:     NewFileLock(t.TempDir()),
		counter:  counter,
	}

	ctx := context.Background()

	// First run should proceed.
	jobCtx := JobRunContext{Lock: fm, JobName: "test-job"}
	run1, rel1 := SkipIfOverlapping(ctx, jobCtx)
	if !run1 {
		t.Fatal("first run should proceed")
	}
	// Don't release yet — simulate long-running job.

	// Second run while first is active should be skipped.
	run2, _ := SkipIfOverlapping(ctx, jobCtx)
	if run2 {
		t.Error("second run should be skipped (previous instance running)")
	}

	// Overlap counter should have been incremented.
	if counter.Count("test-job") != 1 {
		t.Errorf("expected overlap count=1, got %d", counter.Count("test-job"))
	}

	rel1()
}

// TestHashJobDeterministic ensures hashJob returns stable values.
func TestHashJobDeterministic(t *testing.T) {
	if hashJob("foo") != hashJob("foo") {
		t.Error("hashJob must be deterministic")
	}
	if hashJob("foo") == hashJob("bar") {
		t.Error("hashJob must produce different keys for different names (unlikely collision)")
	}
}

// TestParallelJobsDontInterfere verifies that two different job names can run concurrently.
func TestParallelJobsDontInterfere(t *testing.T) {
	fl := NewFileLock(t.TempDir())
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan string, 2)

	for _, name := range []string{"job-a", "job-b"} {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			acq, rel, err := fl.TryLock(ctx, name)
			if err != nil {
				errors <- "error for " + name + ": " + err.Error()
				return
			}
			if !acq {
				errors <- "expected " + name + " to be acquired"
				return
			}
			time.Sleep(10 * time.Millisecond)
			rel()
		}()
	}

	wg.Wait()
	close(errors)
	for e := range errors {
		t.Error(e)
	}
}
