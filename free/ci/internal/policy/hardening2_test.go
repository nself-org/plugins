package policy

// Purpose: regressions for the adversarial review: git failing in a repository
// (foreign owner, spawn error), a project reached through a different-case
// path, and a config directory that is a symlink.
// Constraints: tests only; real git through testRunner.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wantE672(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "E672") {
		t.Fatalf("%s: err %v, want E672", what, err)
	}
}

// git refusing the checkout (foreign owner) or failing to start must not read
// as "no origin": the protected branch's policy cannot be dropped silently.
func TestGitFailureInRepositoryRefuses(t *testing.T) {
	f := newFixture(t)
	f.push(tightConfig)
	repo := Repo{Root: f.work, Visibility: "unknown", Class: "owner"}
	t.Run("foreign owner", func(t *testing.T) {
		t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
		_, err := resolver(t, f, nil).Resolve(context.Background(), repo, Revision{})
		wantE672(t, "foreign owner", err)
	})
	t.Run("spawn error", func(t *testing.T) {
		r := resolver(t, f, nil)
		r.Run = func(context.Context, ...string) (string, int, error) { return "", -1, errors.New("no git") }
		_, err := r.Resolve(context.Background(), repo, Revision{})
		wantE672(t, "spawn error", err)
	})
	t.Run("not a repository", func(t *testing.T) {
		dir := t.TempDir()
		got, err := resolver(t, f, nil).Resolve(context.Background(), Repo{Root: dir, Visibility: "unknown", Class: "owner"}, Revision{})
		if err != nil || got.Fetch != FetchNone {
			t.Fatalf("plain directory: fetch %s err %v", got.Fetch, err)
		}
	})
}

// On a case-insensitive file system the same project reached as .../WORK keeps
// the project id, so its operator rows still apply.
func TestProjectIDIgnoresPathCase(t *testing.T) {
	f := newFixture(t)
	variant := filepath.Join(filepath.Dir(f.work), strings.ToUpper(filepath.Base(f.work)))
	if _, err := os.Stat(variant); err != nil {
		t.Skipf("case-sensitive file system: %v", err)
	}
	s := testStore(t)
	put(t, s, "project", ProjectIDGit(context.Background(), f.runner, f.work), "trust.network", `"none"`)
	id := ProjectIDGit(context.Background(), f.runner, variant)
	if got := resolveOwner(t, f, variant, id, s); got.Values["trust.network"] != "none" {
		t.Fatalf("project row lost through %s: id %q network %v", variant, id, got.Values["trust.network"])
	}
}

// A symlinked .nself on the protected branch is refused, not read as "no policy";
// a symlinked .nself in the worktree is refused too, so both readers agree.
func TestSymlinkedConfigDirRefused(t *testing.T) {
	f := newFixture(t)
	f.ensureOther()
	if err := os.RemoveAll(filepath.Join(f.other, ".nself")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.other, "conf/ci.yaml"), tightConfig)
	if err := os.Symlink("conf", filepath.Join(f.other, ".nself")); err != nil {
		t.Fatal(err)
	}
	git(t, f.other, "add", "-A")
	git(t, f.other, "commit", "-q", "-m", "symlinked config dir")
	git(t, f.other, "push", "-q", "origin", "main")
	_, err := resolver(t, f, nil).Resolve(context.Background(), Repo{Root: f.work, Visibility: "unknown", Class: "owner"}, Revision{})
	wantE672(t, "symlinked dir on the branch", err)

	g := newFixture(t)
	root := g.work
	if err := os.RemoveAll(filepath.Join(root, ".nself")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "conf/ci.yaml"), baseConfig)
	if err := os.Symlink("conf", filepath.Join(root, ".nself")); err != nil {
		t.Fatal(err)
	}
	_, err = resolver(t, g, nil).Resolve(context.Background(), Repo{Root: root, Visibility: "unknown", Class: "owner"}, Revision{Dir: root})
	wantE672(t, "symlinked dir in the worktree", err)
}
