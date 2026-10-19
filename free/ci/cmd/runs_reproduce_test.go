package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/reproduce"
)

// worktreeCount is the number of worktrees git lists for the repository.
func (r *evRepo) worktreeCount() int {
	r.t.Helper()
	return strings.Count(git(r.t, r.dir, "worktree", "list", "--porcelain"), "worktree ")
}

// A reproduction whose run engine fails (the recorded commit holds a pipeline
// file that no longer loads) exits non-zero, saves no link and leaves neither
// a temporary directory nor a git worktree behind.
func TestReproduceEngineErrorCleansUp(t *testing.T) {
	r := newEvRepo(t)
	good := ciYAML
	put(t, r.dir, ".nself/ci.yaml", "version: 1\njobs: [not-a-map]\n")
	git(t, r.dir, "add", "-A")
	git(t, r.dir, "commit", "-q", "-m", "break the pipeline file")
	put(t, r.dir, ".nself/ci.yaml", good) // the original ran on a working tree that still loads
	if _, errOut, code := r.ci("--no-status"); code != 0 {
		t.Fatalf("the original run should pass: exit %d\n%s", code, errOut)
	}

	_, errOut, code := r.ci("runs", "reproduce", "unit")
	if code == 0 {
		t.Fatalf("a failed engine must not exit 0\n%s", errOut)
	}
	t.Logf("stderr: %s", errOut)
	if !strings.Contains(errOut, "reproduce unit at") {
		t.Fatalf("the drift report must print before the run starts:\n%s", errOut)
	}
	if left := r.leftovers(); len(left) != 0 {
		t.Fatalf("temporary directories left behind: %v", left)
	}
	if n := r.worktreeCount(); n != 1 {
		t.Fatalf("git still lists %d worktrees", n)
	}
}

// reproduceRun hands preparePlan's error back unchanged and runs nothing.
func TestReproduceRunReturnsPlanError(t *testing.T) {
	r := newEvRepo(t)
	put(t, r.dir, ".nself/ci.yaml", "version: 1\njobs: [not-a-map]\n")
	_, err := reproduceRun(r.home, io.Discard)(context.Background(), reproduce.RunSpec{Tree: r.dir, Job: "unit"})
	if err == nil {
		t.Fatal("an unloadable pipeline file must be an error")
	}
}
