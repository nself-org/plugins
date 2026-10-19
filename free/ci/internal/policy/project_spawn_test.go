package policy

// Purpose: ProjectIDGit costs two git spawns, not three, and they overlap (D-1008:
// every spawn through the exec funnel is about 11 ms on a job's start path).

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestProjectIDGitSpawnsTwiceAtMost(t *testing.T) {
	f := newFixture(t)
	var n atomic.Int32
	counting := func(ctx context.Context, argv ...string) (string, int, error) {
		n.Add(1)
		return testRunner(ctx, argv...)
	}
	if got, want := ProjectIDGit(context.Background(), counting, f.work), ProjectID(f.work); got != want {
		t.Fatalf("project %q, want %q", got, want)
	}
	if got := n.Load(); got > 2 {
		t.Fatalf("ProjectIDGit spawned git %d times, want at most 2", got)
	}
}
