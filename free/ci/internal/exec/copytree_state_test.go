package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTree creates the files (path -> content) under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// exists reports whether path is present (a link counts).
func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

// A skip path inside the source, and everything under it, is left out of the copy. Paths outside the
// source, the source itself and an ancestor of it change nothing.
func TestCopyTreeSkipsStatePaths(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"a.txt": "a", "state/work/old/f.txt": "old", "state/db": "db", "statement.txt": "keep", "sub/b.txt": "b",
	})
	outside := t.TempDir()
	dst := filepath.Join(t.TempDir(), "ws")
	if err := os.Mkdir(dst, 0700); err != nil {
		t.Fatal(err)
	}
	// src itself, its parent and an unrelated dir are not skippable; "state" is.
	if err := copyTree(src, dst, filepath.Join(src, "state"), src, filepath.Dir(src), outside, ""); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	for _, p := range []string{"a.txt", "statement.txt", "sub/b.txt"} {
		if !exists(filepath.Join(dst, p)) {
			t.Errorf("%s missing from the copy", p)
		}
	}
	if exists(filepath.Join(dst, "state")) {
		t.Errorf("state dir was copied")
	}
}

// A symlinked spelling of the state dir (a temp dir is one on macOS) is matched too.
func TestCopyTreeSkipsStateThroughSymlink(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "repo")
	writeTree(t, src, map[string]string{"a.txt": "a", "state/x": "x"})
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(src, alias); err != nil {
		t.Skipf("cannot create link: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "ws")
	if err := os.Mkdir(dst, 0700); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, dst, filepath.Join(alias, "state")); err != nil {
		t.Fatalf("copyTree: %v", err)
	}
	if !exists(filepath.Join(dst, "a.txt")) || exists(filepath.Join(dst, "state")) {
		t.Fatalf("want a.txt copied and state skipped")
	}
}

// With the state directory inside the repo root the workspace lands inside the source. The run must
// finish (no copy of the workspace into itself), the job must not see the state dir, and the repo files
// must be there, including a config file beside the state dir.
func TestRunWithStateDirInsideRepoRoot(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, ".nself", "ci")
	writeTree(t, root, map[string]string{"README.md": "hi", ".nself/ci/work/older/leftover": "x", ".nself/ci/runs.db": "db", ".nself/ci.yaml": "cfg"})
	var out []byte
	e := &Executor{Isolation: BaselineIsolator{}}
	done := make(chan AttemptResult, 1)
	go func() {
		done <- e.Run(context.Background(), JobSpec{
			AttemptID: "inside", CoordinatorID: "coord", Home: home, SourceDir: root,
			Command: []string{"sh", "-c", "ls -A .nself; test -f README.md && test -f .nself/ci.yaml && test ! -e .nself/ci"},
			Output:  func(p []byte) { out = append(out, p...) },
		})
	}()
	select {
	case r := <-done:
		if r.Err != nil || r.ExitCode != 0 {
			t.Fatalf("run: %+v output %q", r, out)
		}
		if strings.TrimSpace(string(out)) != "ci.yaml" {
			t.Fatalf(".nself listing = %q; want only the repo's ci.yaml, without the state dir", out)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("run did not finish: the workspace copy recursed into itself")
	}
}
