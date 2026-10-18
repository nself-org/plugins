package evidence

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

func TestParseRevision(t *testing.T) {
	good := strings.Repeat("a1", 20)
	for _, s := range []string{good, strings.Repeat("0f", 32)} {
		if got, err := ParseRevision(s); err != nil || got != s {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"", "HEAD", "main", "origin/main", good[:12], good[:39], good + "0", strings.ToUpper(good), good + "\n", "v1.0.0", "HEAD~1", strings.Repeat("g", 40)} {
		_, err := ParseRevision(s)
		var e *Error
		if err == nil || !asErr(err, &e) || e.Code != CodeUsage {
			t.Errorf("%q accepted or wrong code: %v", s, err)
		}
	}
}

func asErr(err error, target **Error) bool {
	e, ok := AsError(err)
	*target = e
	return ok
}

func TestTreeFSReadsGitObjects(t *testing.T) {
	dir, sha := newRepo(t, map[string]string{"a.txt": "one\n", "sub/b.txt": "two\n", "sub/deep/c.txt": "three\n"})
	if err := os.Symlink("a.txt", filepath.Join(dir, "link")); err != nil {
		t.Skip("no symlinks")
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "link")
	sha = gitRun(t, dir, "rev-parse", "HEAD")
	write(t, dir, "a.txt", "edited in the working tree\n") // never visible through the tree
	tree, err := TreeFS(context.Background(), nil, dir, sha)
	ok(t, err)
	b, err := fs.ReadFile(tree, "a.txt")
	if err != nil || string(b) != "one\n" {
		t.Fatalf("a.txt = %q, %v", b, err)
	}
	var seen []string
	ok(t, fs.WalkDir(tree, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			seen = append(seen, p)
		}
		return err
	}))
	if strings.Join(seen, ",") != "a.txt,sub/b.txt,sub/deep/c.txt" { // the symlink is not a regular file
		t.Fatalf("walk: %v", seen)
	}
	if _, err = fs.ReadFile(tree, "missing"); err == nil {
		t.Fatal("a missing path opened")
	}
	if _, err = TreeFS(context.Background(), nil, dir, strings.Repeat("0", 40)); err == nil {
		t.Fatal("an unknown revision listed")
	}
}

func TestInputsFollowTheRevisionNotTheWorkingTree(t *testing.T) {
	ctx := context.Background()
	dir, sha := newRepo(t, map[string]string{"go.mod": "module x\n", "go.sum": "one\n", "web/package-lock.json": "{}\n"})
	base, err := InputsAt(ctx, nil, dir, sha)
	ok(t, err)
	if len(base.Lockfiles) != 2 || base.Lockfiles[0].Path != "go.sum" || base.Lockfiles[1].Path != "web/package-lock.json" {
		t.Fatalf("lockfiles: %+v", base.Lockfiles)
	}
	write(t, dir, "go.sum", "uncommitted\n")
	same, err := InputsAt(ctx, nil, dir, sha)
	ok(t, err)
	if !equalInputs(base, same) {
		t.Fatal("an uncommitted edit changed the digests of a revision")
	}
	locked := commitFiles(t, dir, map[string]string{"go.sum": "two\n"})
	moved, err := InputsAt(ctx, nil, dir, locked)
	ok(t, err)
	if f := attest.Fresh(evOf(base), moved); f.State != "stale" || f.Reason == nil || !strings.Contains(*f.Reason, "go.sum") {
		t.Fatalf("a changed lockfile must read stale: %+v", f)
	}
	cfg := commitFiles(t, dir, map[string]string{".nself/ci.yaml": "version: 1\njobs:\n  unit:\n    kind: test\n    run: [\"true\"]\n"})
	withCfg, err := InputsAt(ctx, nil, dir, cfg)
	ok(t, err)
	if withCfg.Config == moved.Config {
		t.Fatal("a config change left the config digest alone")
	}
	bad := commitFiles(t, dir, map[string]string{".nself/ci.yaml": "version: 1\nunknown_key: true\n"})
	if _, err = InputsAt(ctx, nil, dir, bad); err == nil {
		t.Fatal("an invalid config produced digests")
	}
}

func TestReadWorktree(t *testing.T) {
	ctx := context.Background()
	dir, _ := newRepo(t, map[string]string{"a.txt": "a\n", ".gitignore": "ignored.log\n"})
	if w := ReadWorktree(ctx, nil, dir); w.Dirty || w.Tracked != nil {
		t.Fatalf("clean tree: %+v", w)
	}
	write(t, dir, "ignored.log", "x")
	if w := ReadWorktree(ctx, nil, dir); w.Dirty {
		t.Fatal("an ignored file made the tree dirty")
	}
	write(t, dir, "new.txt", "untracked")
	if w := ReadWorktree(ctx, nil, dir); !w.Dirty {
		t.Fatal("an untracked file must make the tree dirty")
	}
	_ = os.Remove(filepath.Join(dir, "new.txt"))
	write(t, dir, "a.txt", "edit\n")
	w := ReadWorktree(ctx, nil, dir)
	diff, _ := git(ctx, nil, dir, "diff", "HEAD", "--binary")
	if !w.Dirty || w.Tracked == nil || *w.Tracked != digestString(diff) {
		t.Fatalf("tracked edit: %+v", w)
	}
	if w = ReadWorktree(ctx, nil, t.TempDir()); !w.Dirty { // not a repository: fail closed
		t.Fatal("an unreadable tree must read dirty")
	}
}

func TestWorktreeAddsAndRemoves(t *testing.T) {
	ctx := context.Background()
	t.Setenv("TMPDIR", t.TempDir())
	dir, sha := newRepo(t, baseFiles)
	write(t, dir, "a.txt", "uncommitted\n")
	wt, err := AddWorktree(ctx, nil, dir, sha)
	ok(t, err)
	b, err := os.ReadFile(filepath.Join(wt.Path, "a.txt"))
	if err != nil || string(b) != "a\n" {
		t.Fatalf("worktree a.txt = %q %v: uncommitted edits must not reach it", b, err)
	}
	if head := gitRun(t, wt.Path, "rev-parse", "HEAD"); head != sha {
		t.Fatalf("worktree HEAD %s", head)
	}
	if !strings.Contains(gitRun(t, dir, "worktree", "list"), filepath.Base(filepath.Dir(wt.Path))) {
		t.Fatal("the worktree is not registered")
	}
	ok(t, wt.Remove())
	ok(t, wt.Remove()) // idempotent
	if _, err = os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree still on disk: %v", err)
	}
	if n := len(strings.Split(gitRun(t, dir, "worktree", "list"), "\n")); n != 1 {
		t.Fatalf("git still lists %d worktrees", n)
	}
	if left, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "nself-ci-verify-*")); len(left) != 0 {
		t.Fatalf("leftover directories: %v", left)
	}
	if _, err = AddWorktree(ctx, nil, dir, strings.Repeat("0", 40)); err == nil {
		t.Fatal("an unknown revision checked out")
	}
	if left, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "nself-ci-verify-*")); len(left) != 0 {
		t.Fatalf("a failed add left directories: %v", left)
	}
}
