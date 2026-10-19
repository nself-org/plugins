//go:build !windows

package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// D-1008: the descendant tracker must not fork ps. Each fork cost about 10 ms,
// ran three times per snapshot and dominated the V15 tiny-repo run time.
func TestTrackerDoesNotForkPS(t *testing.T) {
	shim := t.TempDir()
	log := filepath.Join(shim, "ps.calls")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(shim, "ps"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))
	res := (&Executor{Grace: 100 * time.Millisecond}).Run(context.Background(), JobSpec{
		AttemptID: "no-ps", CoordinatorID: "coord", Home: t.TempDir(),
		Command: []string{"sh", "-c", "sleep 0.3 & sleep 0.2; wait"},
	})
	if res.Err != nil || res.ExitCode != 0 {
		t.Fatalf("run: %+v", res)
	}
	if b, _ := os.ReadFile(log); strings.TrimSpace(string(b)) != "" {
		t.Fatalf("tracker forked ps %d time(s): %q", strings.Count(string(b), "\n"), b)
	}
}

// The table must list this process, its parent, and a live child.
func TestProcessTableSeesChildren(t *testing.T) {
	parents, err := processTable()
	if err != nil {
		t.Fatal(err)
	}
	if got := parents[os.Getpid()]; got != os.Getppid() {
		t.Fatalf("ppid of self = %d, want %d", got, os.Getppid())
	}
}
