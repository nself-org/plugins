package evidence

// Purpose: the release gate end to end with the registrations this package
// makes (D-1074): a real git repository, the real coordinator, a test job and
// a release job, and no fake for run.ReleaseInputs or run.ReleaseWorktree.
// Constraints: V15 only; git is required (a missing git is a failure).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func releaseJobs() map[string]model.Job {
	no := false
	return map[string]model.Job{
		"build":   {Kind: "test", Run: []string{"sh", "-c", "true"}, Required: yes()},
		"publish": {Kind: "release", Run: []string{"sh", "-c", "true"}, Deps: []string{"build"}, Required: yes(), Idempotent: &no},
	}
}

// runRelease runs the release pipeline in dir and returns the result and the output.
func runRelease(t *testing.T, dir, rev string) (cirun.Result, string) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "")
	bound, err := InputsAt(t.Context(), nil, dir, rev)
	ok(t, err)
	var out strings.Builder
	c := &cirun.Coordinator{Opts: cirun.Options{Root: dir, Home: t.TempDir(), NoStatus: true, Full: true}, Plan: buildPlan(t, releaseJobs()),
		ConfigDigest: bound.Config, Out: &out, Capacity: roomy, Poll: 10 * 1e6}
	res, err := c.Run(t.Context())
	ok(t, err)
	return res, out.String()
}

func TestReleaseRunOnACleanTreeIsAdmitted(t *testing.T) {
	dir, rev := newRepo(t, baseFiles)
	res, out := runRelease(t, dir, rev)
	if res.Exit != 0 || strings.Contains(out, "E715") {
		t.Fatalf("a clean, bound release run was refused (exit %d):\n%s", res.Exit, out)
	}
}

func TestReleaseRunOnADirtyTreeIsRefused(t *testing.T) {
	dir, rev := newRepo(t, baseFiles)
	write(t, dir, "a.txt", "edited\n")
	res, out := runRelease(t, dir, rev)
	if res.Exit == 0 || !strings.Contains(out, "E715") {
		t.Fatalf("a dirty release run was admitted (exit %d):\n%s", res.Exit, out)
	}
}

// A sparse checkout leaves excluded paths absent: that hides nothing, and the
// run's own evidence calls the tree clean, so the gate must agree.
func TestReleaseRunOnASparseCheckoutIsAdmitted(t *testing.T) {
	dir, rev := newRepo(t, baseFiles)
	gitRun(t, dir, "update-index", "--skip-worktree", "a.txt")
	ok(t, os.Remove(filepath.Join(dir, "a.txt")))
	if w := ReadWorktree(t.Context(), nil, dir); w.Dirty {
		t.Fatal("fixture: evidence calls the sparse tree dirty")
	}
	res, out := runRelease(t, dir, rev)
	if res.Exit != 0 || strings.Contains(out, "E715") {
		t.Fatalf("a sparse release run was refused (exit %d):\n%s", res.Exit, out)
	}
}

func TestReleaseRunOnAHiddenEditIsRefused(t *testing.T) {
	dir, rev := newRepo(t, baseFiles)
	gitRun(t, dir, "update-index", "--assume-unchanged", "a.txt")
	write(t, dir, "a.txt", "edited\n")
	res, out := runRelease(t, dir, rev)
	if res.Exit == 0 || !strings.Contains(out, "E715") {
		t.Fatalf("a release run with a hidden edit was admitted (exit %d):\n%s", res.Exit, out)
	}
}
