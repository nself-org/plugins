package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A hook (pre-commit, rebase) exports GIT_DIR and GIT_INDEX_FILE for its own
// repository; evidence git calls must ignore them (D-0988 item 2).
func TestGitCallsIgnoreInheritedGitEnv(t *testing.T) {
	dir, sha := newRepo(t, baseFiles)
	bogus := filepath.Join(t.TempDir(), "elsewhere")
	t.Setenv("GIT_DIR", bogus)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(bogus, "index"))
	t.Setenv("GIT_WORK_TREE", bogus)
	ctx := context.Background()
	if _, found := RepoID(ctx, nil, dir); !found {
		t.Fatal("RepoID must read the repository named by -C, not GIT_DIR")
	}
	w, err := AddWorktree(ctx, nil, dir, sha)
	if err != nil {
		t.Fatalf("AddWorktree under GIT_DIR: %v", err)
	}
	ok(t, w.Remove())
}

// A SIGKILLed verify leaves nself-ci-verify-* directories; AddWorktree sweeps
// the stale ones and never a recent one (D-0988 item 6).
func TestAddWorktreeSweepsStaleVerifyDirs(t *testing.T) {
	dir, sha := newRepo(t, baseFiles)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	mk := func(name string, age time.Duration) string {
		p := filepath.Join(tmp, name)
		ok(t, os.MkdirAll(filepath.Join(p, "tree"), 0o755))
		old := time.Now().Add(-age)
		ok(t, os.Chtimes(p, old, old))
		return p
	}
	stale := mk("nself-ci-verify-stale", 48*time.Hour)
	fresh := mk("nself-ci-verify-fresh", time.Minute)
	other := mk("unrelated-old", 48*time.Hour)
	w, err := AddWorktree(context.Background(), nil, dir, sha)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Remove() })
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale verify dir must be swept, stat err = %v", err)
	}
	for _, keep := range []string{fresh, other} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s must stay: %v", keep, err)
		}
	}
}
