package policy

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// failListing is the real runner with `git worktree list` failing, as when a
// worktree admin directory is unreadable right after rev-parse succeeded.
func failListing(listCalls *int) Runner {
	return func(ctx context.Context, argv ...string) (string, int, error) {
		if slices.Contains(argv, "worktree") && slices.Contains(argv, "list") {
			*listCalls++
			return "fatal: worktree list failed\n", 128, nil
		}
		return testRunner(ctx, argv...)
	}
}

// A failed worktree listing must not move a linked checkout to its own path as
// project id (D-1127): the common git directory still names the main worktree.
func TestProjectIDGitLinkedCheckoutSurvivesFailedWorktreeList(t *testing.T) {
	f := newFixture(t)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, f.work, "worktree", "add", "-q", "--detach", linked)
	var calls int
	got := ProjectIDGit(context.Background(), failListing(&calls), linked)
	if calls == 0 {
		t.Fatal("the worktree listing was never asked")
	}
	if want := ProjectID(f.work); got != want {
		t.Fatalf("linked checkout id %q after a failed listing, want main worktree %q", got, want)
	}
}

// When neither the listing nor the common git directory can be read, the own-path
// id is returned once and never kept by the cache.
func TestProjectIDGitCachedDropsIncompleteListing(t *testing.T) {
	f := newFixture(t)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, f.work, "worktree", "add", "-q", "--detach", linked)
	var spawns int
	run := Runner(func(ctx context.Context, argv ...string) (string, int, error) {
		spawns++
		if slices.Contains(argv, "--git-common-dir") || (slices.Contains(argv, "worktree") && slices.Contains(argv, "list")) {
			return "fatal\n", 128, nil
		}
		return testRunner(ctx, argv...)
	})
	ProjectIDGitCached(context.Background(), run, linked)
	n := spawns
	ProjectIDGitCached(context.Background(), run, linked)
	if spawns == n {
		t.Fatal("an id built without the worktree layout was kept")
	}
}
