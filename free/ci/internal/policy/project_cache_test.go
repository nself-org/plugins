package policy

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// A run asks for its project id on three start paths: the git spawns happen once.
func TestProjectIDGitCachedSpawnsOncePerWindow(t *testing.T) {
	root := t.TempDir()
	var spawns atomic.Int32
	run := Runner(func(_ context.Context, argv ...string) (string, int, error) {
		spawns.Add(1)
		return "", 128, nil // not a repository: ProjectIDGit falls back to the filesystem id
	})
	first := ProjectIDGitCached(context.Background(), run, root)
	n := spawns.Load()
	if n == 0 {
		t.Fatal("the first call never asked git")
	}
	for range 3 {
		if got := ProjectIDGitCached(context.Background(), run, root); got != first {
			t.Fatalf("id %q, want %q", got, first)
		}
	}
	if spawns.Load() != n {
		t.Fatalf("%d git spawns after the first call, want none", spawns.Load()-n)
	}
	if !strings.HasPrefix(first, "/") {
		t.Fatalf("id %q is not absolute", first)
	}
}

// An answer built while git could not be asked is the loosened fallback id: it is
// never kept, so the next call asks again. Two roots never share an answer.
func TestProjectIDGitCachedKeepsOnlyDefinitiveAnswers(t *testing.T) {
	root := t.TempDir()
	var spawns atomic.Int32
	failing := true
	run := Runner(func(_ context.Context, argv ...string) (string, int, error) {
		spawns.Add(1)
		if failing {
			return "", -1, errors.New("git: spawn failed")
		}
		return "", 128, nil
	})
	ProjectIDGitCached(context.Background(), run, root)
	n := spawns.Load()
	failing = false
	ProjectIDGitCached(context.Background(), run, root)
	if spawns.Load() == n {
		t.Fatal("a fallback id built from a failed spawn was kept")
	}
	n = spawns.Load()
	ctx, cancel := context.WithCancel(context.Background())
	other := t.TempDir()
	cancel()
	ProjectIDGitCached(ctx, run, other)
	m := spawns.Load()
	ProjectIDGitCached(context.Background(), run, other)
	if spawns.Load() == m {
		t.Fatal("an answer built under a cancelled context was kept")
	}
	if a, b := ProjectIDGitCached(context.Background(), run, root), ProjectIDGitCached(context.Background(), run, other); a == b {
		t.Fatalf("two roots share the id %q", a)
	}
	_ = n
}
