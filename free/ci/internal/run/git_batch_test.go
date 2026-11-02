//go:build !windows

package run

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/gitbatch"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

func batchRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nself", "ci.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "x"}} {
		cmd := osexec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// countingCapture is the real funnel that remembers which git subcommands it was asked to run.
type countingCapture struct {
	mu   sync.Mutex
	seen []string
}

func (c *countingCapture) capture(ctx context.Context, argv ...string) (string, int, error) {
	c.mu.Lock()
	c.seen = append(c.seen, strings.Join(argv, " "))
	c.mu.Unlock()
	return report.Capture(ctx, argv...)
}

func (c *countingCapture) ran(sub string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.seen {
		if strings.Contains(s, " "+sub) {
			return true
		}
	}
	return false
}

func TestStartOfRunObjectReadsUseTheBatchSession(t *testing.T) {
	dir := batchRepo(t)
	cfg := filepath.Join(dir, ".nself", "ci.yaml")

	plain := &countingCapture{}
	want := GitRevision(context.Background(), dir, plain.capture)
	wantOutside := ConfigOutsideTree(context.Background(), dir, cfg, ".nself/ci.yaml", plain.capture)
	if want.SHA == "" || wantOutside || !plain.ran("rev-parse HEAD") || !plain.ran("ls-tree HEAD") {
		t.Fatalf("baseline: %+v outside=%v calls=%v", want, wantOutside, plain.seen)
	}

	set := gitbatch.NewSet(context.Background(), gitbatch.Options{})
	defer set.Close()
	ctx := gitbatch.WithSet(context.Background(), set)
	batched := &countingCapture{}
	got := GitRevision(ctx, dir, batched.capture)
	if got != want {
		t.Fatalf("GitRevision with a session = %+v, without = %+v", got, want)
	}
	if ConfigOutsideTree(ctx, dir, cfg, ".nself/ci.yaml", batched.capture) != wantOutside {
		t.Fatal("ConfigOutsideTree differs with a session")
	}
	if batched.ran("rev-parse HEAD") || batched.ran("ls-tree") {
		t.Fatalf("rev-parse HEAD and ls-tree must not spawn a process per read: %v", batched.seen)
	}
	if !batched.ran("status") || !batched.ran("hash-object") {
		t.Fatalf("status and hash-object are not object reads and still run through git: %v", batched.seen)
	}
	// A config edited after the commit is still outside the tree (hash-object stays the judge).
	if err := os.WriteFile(cfg, []byte("version: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ConfigOutsideTree(ctx, dir, cfg, ".nself/ci.yaml", batched.capture) {
		t.Fatal("an edited config must be outside the tree")
	}
	// A repository with no commit: no revision, as before.
	empty := t.TempDir()
	if out, err := osexec.Command("git", "-C", empty, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if rev := GitRevision(ctx, empty, batched.capture); rev.SHA != "" {
		t.Fatalf("no commit must give no revision: %+v", rev)
	}
}
