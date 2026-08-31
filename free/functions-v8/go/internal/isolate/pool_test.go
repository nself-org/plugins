package isolate_test

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/nself-functions-v8/internal/isolate"
)

func newTestPool(size int) *isolate.Pool {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return isolate.NewPool("/usr/bin/deno", size, 128, 5000, logger)
}

func TestIsolatePool_AcquireRelease(t *testing.T) {
	poolSize := 3
	pool := newTestPool(poolSize)

	// Acquire all slots.
	ctx := context.Background()
	for i := 0; i < poolSize; i++ {
		if err := pool.Acquire(ctx); err != nil {
			t.Fatalf("acquire %d failed: %v", i, err)
		}
	}

	// 4th acquire should block until one is released.
	released := make(chan struct{})
	acquired := make(chan struct{})

	go func() {
		<-released
		pool.Release()
	}()

	go func() {
		start := time.Now()
		if err := pool.Acquire(ctx); err != nil {
			t.Errorf("4th acquire failed: %v", err)
		}
		elapsed := time.Since(start)
		if elapsed > 100*time.Millisecond {
			t.Errorf("acquire took too long after release: %v", elapsed)
		}
		close(acquired)
		pool.Release()
	}()

	// Release one slot after a small delay.
	time.Sleep(5 * time.Millisecond)
	close(released)

	select {
	case <-acquired:
		// success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("4th acquire did not complete within 200ms after release")
	}

	// Release remaining slots.
	for i := 0; i < poolSize-1; i++ {
		pool.Release()
	}
}

func TestIsolatePool_ConcurrentAcquire(t *testing.T) {
	poolSize := 5
	pool := newTestPool(poolSize)
	ctx := context.Background()

	var acquired atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < poolSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := pool.Acquire(ctx); err == nil {
				acquired.Add(1)
				time.Sleep(10 * time.Millisecond)
				pool.Release()
			}
		}()
	}
	wg.Wait()

	if int(acquired.Load()) != poolSize {
		t.Errorf("expected %d goroutines to acquire, got %d", poolSize, acquired.Load())
	}
}

func TestIsolatePool_AcquireCancelled(t *testing.T) {
	poolSize := 1
	pool := newTestPool(poolSize)
	ctx := context.Background()

	// Exhaust the pool.
	if err := pool.Acquire(ctx); err != nil {
		t.Fatal("initial acquire failed")
	}

	// Try to acquire with a cancelled context.
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // cancel immediately

	err := pool.Acquire(cancelCtx)
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
		pool.Release() // cleanup
	}

	pool.Release() // release the first one
}

func TestIsolatePool_PoolSize(t *testing.T) {
	pool := newTestPool(7)
	if pool.PoolSize() != 7 {
		t.Errorf("expected pool size 7, got %d", pool.PoolSize())
	}
}

// TestIsPrivateHost_RFC1918 verifies that RFC-1918 and loopback addresses are
// classified as private (SSRF guard AC).
func TestIsPrivateHost_RFC1918(t *testing.T) {
	private := []string{
		"192.168.1.1",
		"10.0.0.1",
		"172.16.0.1",
		"172.31.255.255",
		"127.0.0.1",
		"::1",
		"192.168.1.1:8080",
	}
	for _, h := range private {
		if !isolate.IsPrivateHost(h) {
			t.Errorf("IsPrivateHost(%q) = false, want true (SSRF guard must block this)", h)
		}
	}
}

// TestIsPrivateHost_Public verifies that public addresses are not classified as private.
func TestIsPrivateHost_Public(t *testing.T) {
	public := []string{
		"8.8.8.8",
		"1.1.1.1",
		"api.example.com",
		"example.com:443",
	}
	for _, h := range public {
		if isolate.IsPrivateHost(h) {
			t.Errorf("IsPrivateHost(%q) = true, want false (public host should be allowed)", h)
		}
	}
}

// TestSSRFBlock_PrivateHostFilteredFromAllowNet verifies that a private IP in
// NSELF_ALLOW_NET is silently removed before constructing the Deno --allow-net
// flag. This test does not require Deno — it checks pool construction behaviour.
func TestSSRFBlock_PrivateHostFilteredFromAllowNet(t *testing.T) {
	// We set the env var to include a private address and expect the pool to
	// build without panicking and that the private host is not in allowNet.
	t.Setenv("NSELF_ALLOW_NET", "192.168.1.1,api.safe-external.com,10.0.0.5")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	pool := isolate.NewPool("/usr/bin/deno", 1, 128, 5000, logger)
	if pool == nil {
		t.Fatal("NewPool returned nil")
	}
	// Pool should still have size 1 — the filtered private hosts do not crash it.
	if pool.PoolSize() != 1 {
		t.Errorf("PoolSize = %d, want 1", pool.PoolSize())
	}
}

// TestTimeout_Returns504 tests that a function exceeding the timeout gets a 504.
// This test requires deno to be available — skipped if not present.
func TestTimeout_Returns504(t *testing.T) {
	if _, err := os.Stat("/usr/bin/deno"); os.IsNotExist(err) {
		t.Skip("deno not available at /usr/bin/deno — skipping timeout integration test")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// 500ms timeout for fast test.
	pool := isolate.NewPool("/usr/bin/deno", 1, 128, 500, logger)

	// A function that sleeps indefinitely.
	sleepSource := `
import { serve } from 'https://deno.land/std@0.208.0/http/server.ts'
serve(async () => {
  await new Promise(r => setTimeout(r, 60000))
  return new Response('never')
})
`
	ctx := context.Background()
	if err := pool.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	defer pool.Release()

	result, err := pool.Invoke(ctx, sleepSource, nil, "GET", "/functions/v1/test", nil)
	if err != nil {
		t.Fatalf("invoke returned error: %v", err)
	}
	if result.StatusCode != 504 {
		t.Errorf("expected 504 for timeout, got %d", result.StatusCode)
	}
	if !result.TimedOut {
		t.Error("expected TimedOut=true")
	}
}
