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

// A symlink into a subdirectory of a repository is still inside it: a git that
// refuses the checkout must not read as "no origin" through the alias.
func TestGitFailureThroughSymlinkToSubdirectory(t *testing.T) {
	f := newFixture(t)
	f.push(tightConfig)
	sub := filepath.Join(f.work, "svc")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(sub, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	_, err := resolver(t, f, nil).Resolve(context.Background(), Repo{Root: alias, Visibility: "unknown", Class: "owner"}, Revision{})
	wantE672(t, "alias into a subdirectory", err)
}

// Directories "svc" and " svc" are different projects, in the main and a linked worktree.
func TestProjectIDKeepsSpacesInPrefix(t *testing.T) {
	f := newFixture(t)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, f.work, "worktree", "add", "-q", "--detach", linked)
	ctx := context.Background()
	for _, base := range []string{f.work, linked} {
		a, b := filepath.Join(base, "svc"), filepath.Join(base, " svc")
		for _, d := range []string{a, b} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		ia, ib := ProjectIDGit(ctx, f.runner, a), ProjectIDGit(ctx, f.runner, b)
		if ia == ib || ib != filepath.Join(ProjectID(f.work), " svc") || ia != filepath.Join(ProjectID(f.work), "svc") {
			t.Errorf("%s: ids %q and %q", base, ia, ib)
		}
	}
}

// Newlines and quote characters in paths neither truncate the prefix nor hide
// the main worktree from a linked one.
func TestProjectIDOddPaths(t *testing.T) {
	f := newFixture(t)
	odd := "q\"uo\nte"
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, f.work, "worktree", "add", "-q", "--detach", linked)
	ctx := context.Background()
	for _, base := range []string{f.work, linked} {
		a, b := filepath.Join(base, "svc\nchild"), filepath.Join(base, "svc")
		for _, d := range []string{a, b} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		ia, ib := ProjectIDGit(ctx, f.runner, a), ProjectIDGit(ctx, f.runner, b)
		if ia == ib || ia != filepath.Join(ProjectID(f.work), "svc\nchild") {
			t.Errorf("%s: ids %q and %q", base, ia, ib)
		}
	}
	// A main worktree whose own path needs quoting still anchors its linked worktrees.
	main := filepath.Join(t.TempDir(), odd)
	git(t, filepath.Dir(main), "init", "-q", "-b", "main", main)
	write(t, filepath.Join(main, "a.txt"), "x")
	git(t, main, "add", "-A")
	git(t, main, "commit", "-q", "-m", "x")
	lw := filepath.Join(t.TempDir(), "lw")
	git(t, main, "worktree", "add", "-q", "--detach", lw)
	if got, want := ProjectIDGit(ctx, f.runner, lw), ProjectID(main); got != want {
		t.Errorf("quoted main path: %q, want %q", got, want)
	}
}

// A missing path under an alias into a repository cannot be placed outside it.
func TestGitFailureThroughMissingPathUnderAlias(t *testing.T) {
	f := newFixture(t)
	f.push(tightConfig)
	sub := filepath.Join(f.work, "svc")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(sub, alias); err != nil {
		t.Fatal(err)
	}
	if !inRepository(filepath.Join(alias, "missing")) {
		t.Fatal("a missing path under a repository alias counts as outside it")
	}
}
