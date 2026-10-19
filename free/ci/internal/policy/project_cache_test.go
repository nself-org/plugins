package policy

import (
	"context"
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
