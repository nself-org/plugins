package policy

// Purpose: the loosening paths a cross-family review found: a project row that
// stops matching, a guessed default branch, a name git accepts but a regex did
// not, git steered by the environment or by refs/replace, and malformed rows.
// Constraints: tests only; real git through testRunner.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tightConfig = baseConfig + "policy:\n  trust:\n    network: none\n"

func resolveOwner(t *testing.T, f *fixture, root string, project string, s Reader) Resolved {
	t.Helper()
	r := resolver(t, f, s)
	got, err := r.Resolve(context.Background(), Repo{Root: root, Project: project, Visibility: "unknown", Class: "owner"}, Revision{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A project row written for the real path applies through a symlink and from a
// linked worktree, and a monorepo subdirectory keeps its own identity.
func TestProjectRowFollowsAliasAndLinkedWorktree(t *testing.T) {
	f := newFixture(t)
	s := testStore(t)
	put(t, s, "project", ProjectID(f.work), "trust.network", `"none"`)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(f.work, alias); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, f.work, "worktree", "add", "-q", "--detach", linked)
	ctx := context.Background()
	for name, root := range map[string]string{"real": f.work, "alias": alias, "linked": linked} {
		id := ProjectIDGit(ctx, f.runner, root)
		if id != ProjectID(f.work) {
			t.Errorf("%s: project %q, want %q", name, id, ProjectID(f.work))
		}
		if got := resolveOwner(t, f, root, id, s); got.Values["trust.network"] != "none" {
			t.Errorf("%s: project row lost, network %v", name, got.Values["trust.network"])
		}
	}
	sub := filepath.Join(linked, "svc")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := ProjectIDGit(ctx, f.runner, sub), filepath.Join(ProjectID(f.work), "svc"); got != want {
		t.Errorf("subdirectory project %q, want %q", got, want)
	}
}

// A CI-style checkout (init, remote add, fetch) has no origin/HEAD. The real
// default branch is asked of the remote, so a looser decoy "main" is not read.
func TestDefaultBranchDiscoveredWithoutOriginHEAD(t *testing.T) {
	base := t.TempDir()
	bare, seed, clone := filepath.Join(base, "o.git"), filepath.Join(base, "seed"), filepath.Join(base, "clone")
	for _, d := range []string{bare, seed, clone} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(t, bare, "init", "-q", "--bare", "-b", "master")
	git(t, seed, "init", "-q", "-b", "master")
	write(t, filepath.Join(seed, ".nself/ci.yaml"), baseConfig)
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-q", "-m", "base")
	git(t, seed, "branch", "main")
	write(t, filepath.Join(seed, ".nself/ci.yaml"), tightConfig)
	git(t, seed, "commit", "-q", "-am", "tighten the protected default")
	git(t, seed, "push", "-q", bare, "master", "main")
	git(t, bare, "symbolic-ref", "HEAD", "refs/heads/master")
	git(t, clone, "init", "-q", "-b", "work")
	git(t, clone, "remote", "add", "origin", bare)
	git(t, clone, "fetch", "-q", "origin")
	f := &fixture{t: t, runner: testRunner}
	got := resolveOwner(t, f, clone, "p", nil)
	if got.PinnedCommit != git(t, seed, "rev-parse", "master") || got.Values["trust.network"] != "none" {
		t.Fatalf("read the wrong branch: pin %s network %v", got.PinnedCommit, got.Values["trust.network"])
	}
	for _, ig := range got.Ignored {
		if ig.Key == "default_branch" {
			t.Fatalf("default branch reported as assumed: %v", ig)
		}
	}
}

// A discovery that fails entirely falls back to main and says so.
func TestDefaultBranchAssumedIsReported(t *testing.T) {
	f := newFixture(t)
	git(t, f.work, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	git(t, f.work, "update-ref", "-d", "refs/remotes/origin/HEAD")
	got := resolveOwner(t, f, f.work, "p", nil)
	found := false
	for _, ig := range got.Ignored {
		found = found || ig.Key == "default_branch"
	}
	if !found {
		t.Fatalf("assumed main not reported: %v", got.Ignored)
	}
}

// An origin that was never reachable leaves no protected-branch scope; the run
// proceeds on operator rows and defaults, and the missing scope is reported.
func TestUnreachableOriginIsReported(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "work", root)
	git(t, root, "remote", "add", "origin", filepath.Join(t.TempDir(), "gone.git"))
	f := &fixture{t: t, runner: testRunner}
	got := resolveOwner(t, f, root, "p", nil)
	found := false
	for _, ig := range got.Ignored {
		found = found || ig.Key == "protected_branch"
	}
	if got.Fetch != FetchNone || !found {
		t.Fatalf("fetch %s ignored %v", got.Fetch, got.Ignored)
	}
}

// A branch name git accepts but a narrow regex rejected (+, non-ASCII) is used
// as is; it is never swapped for main.
func TestOddDefaultBranchNames(t *testing.T) {
	for _, name := range []string{"release+secure", "release/équipe"} {
		base := t.TempDir()
		bare, work := filepath.Join(base, "o.git"), filepath.Join(base, "w")
		git(t, base, "init", "-q", "--bare", "-b", name, bare)
		git(t, base, "init", "-q", "-b", name, work)
		git(t, work, "remote", "add", "origin", bare)
		write(t, filepath.Join(work, ".nself/ci.yaml"), tightConfig)
		git(t, work, "add", "-A")
		git(t, work, "commit", "-q", "-m", "tight")
		git(t, work, "push", "-q", "origin", name)
		git(t, work, "remote", "set-head", "origin", name)
		f := &fixture{t: t, runner: testRunner}
		got := resolveOwner(t, f, work, "p", nil)
		if got.Values["trust.network"] != "none" || got.Sources["trust.network"] != SourceProtectedBranch {
			t.Errorf("%s: branch policy not read: %v %v", name, got.Values, got.Sources)
		}
	}
}

// GIT_DIR in the environment must not redirect the policy read, and a
// refs/replace object must not replace the tip's tree.
func TestGitEnvironmentAndReplaceObjects(t *testing.T) {
	f := newFixture(t)
	tip := f.push(tightConfig)
	first := resolveOwner(t, f, f.work, "p", nil) // fetches the tip
	if first.PinnedCommit != tip || first.Values["trust.network"] != "none" {
		t.Fatalf("setup: pin %s network %v", first.PinnedCommit, first.Values["trust.network"])
	}
	base := git(t, f.work, "rev-parse", tip+"~1")
	git(t, f.work, "replace", tip, base) // the tip now reads as the policy-free base commit
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "elsewhere.git"))
	got := resolveOwner(t, f, f.work, "p", nil)
	if got.PinnedCommit != tip || got.Values["trust.network"] != "none" || got.Sources["trust.network"] != SourceProtectedBranch {
		t.Fatalf("environment or replace object steered the read: pin %s %v %v", got.PinnedCommit, got.Values, got.Sources)
	}
}

// Git configuration injected through the environment must not make remote
// discovery fail into "no origin"; and when remotes truly cannot be listed in
// a repository, the run refuses instead of reading no branch policy.
func TestRemoteListFailureIsNotNoOrigin(t *testing.T) {
	f := newFixture(t)
	f.push(tightConfig)
	t.Setenv("GIT_CONFIG_COUNT", "invalid")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.bogus'='x'; broken")
	got := resolveOwner(t, f, f.work, "p", nil)
	if got.Values["trust.network"] != "none" || got.Sources["trust.network"] != SourceProtectedBranch {
		t.Fatalf("environment config hid the branch policy: %v %v", got.Values, got.Sources)
	}
	r := resolver(t, f, nil)
	r.Run = func(ctx context.Context, argv ...string) (string, int, error) {
		for _, a := range argv {
			if a == "remote" {
				return "fatal: boom", 128, nil
			}
		}
		return f.runner(ctx, argv...)
	}
	if _, err := r.Resolve(context.Background(), Repo{Root: f.work, Visibility: "unknown", Class: "owner"}, Revision{}); err == nil || !strings.Contains(err.Error(), "E672") {
		t.Fatalf("remote list failure in a repository: %v", err)
	}
}

// Git records no path to a --separate-git-dir checkout, so its project id is
// its own path (never the git directory) from the main and a linked worktree.
func TestProjectIDSeparateGitDir(t *testing.T) {
	base := t.TempDir()
	main, meta, linked := filepath.Join(base, "main"), filepath.Join(base, "meta"), filepath.Join(base, "linked")
	git(t, base, "init", "-q", "-b", "main", "--separate-git-dir", meta, main)
	write(t, filepath.Join(main, "a.txt"), "x")
	git(t, main, "add", "-A")
	git(t, main, "commit", "-q", "-m", "x")
	git(t, main, "worktree", "add", "-q", "--detach", linked)
	for _, root := range []string{main, linked} {
		if got, want := ProjectIDGit(context.Background(), testRunner, root), ProjectID(root); got != want {
			t.Errorf("separate git dir: project %q, want %q", got, want)
		}
	}
}

// A stored row with trailing bytes after one valid value is malformed.
func TestMalformedStoredValueRejected(t *testing.T) {
	for _, raw := range []string{`"privileged"]`, `false}`, `"a" "b"`, `{`} {
		if _, err := decodeOne([]byte(raw)); err == nil || strings.TrimSpace(err.Error()) == "" {
			t.Errorf("%s accepted", raw)
		}
	}
}
