package main

import (
	"strings"
	"testing"
	"time"
)

// A block that starts a job with a CPU limit of its own must not be refused because the block itself
// carries a lower one: an ad hoc `nself ci -- cmd` example runs a 30 minute inner job inside a block with a
// 10 minute bound. The inner `ulimit -t` raises the hard limit above the block's and fails with
// "Operation not permitted" when the block was started under a CPU ulimit.
func TestBlockCanStartJobWithHigherCPULimit(t *testing.T) {
	dir, cleanup, err := newFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	r := shellRunner{timeout: time.Minute}
	code, err := r.Run(dir, "sh -c 'ulimit -t 1800 || exit 42; echo inner-ok'")
	if err != nil || code != 0 {
		t.Fatalf("block exit %d, err %v; want 0 (the inner job sets a limit above the block's bound)", code, err)
	}
}

// The wall-clock bound still fails a hung block, and the error names the bound.
func TestBlockTimeoutStillFailsHungBlock(t *testing.T) {
	dir, cleanup, err := newFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	start := time.Now()
	code, err := shellRunner{timeout: 500 * time.Millisecond}.Run(dir, "sleep 30")
	if err == nil || code != -1 || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("exit %d, err %v; want -1 and a timeout error", code, err)
	}
	if time.Since(start) > 20*time.Second {
		t.Fatalf("a hung block ran %s past its bound", time.Since(start))
	}
}
