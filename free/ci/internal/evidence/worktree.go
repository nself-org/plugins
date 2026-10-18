package evidence

// Purpose: the temporary git worktree `ci evidence verify <rev>` runs the
// required pipeline in, so uncommitted edits and ignored files never
// influence that run.
// Inputs:  a repository root and a full revision.
// Outputs: a detached checkout of exactly that commit in a new directory.
// Constraints: `git worktree add --detach <tmp> <rev>`; Remove is idempotent
// and is called on every exit path (the caller defers it and also calls it
// from its signal handler); removal uses a context that survives Ctrl-C;
// git runs through the exec funnel.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// TempWorktree is a detached checkout that Remove deletes.
type TempWorktree struct {
	// Path is the checkout; run the pipeline here.
	Path string

	dir, repo string
	c         Capture
	once      sync.Once
	err       error
}

var worktreeSeq = func() func() int {
	var mu sync.Mutex
	n := 0
	return func() int { mu.Lock(); defer mu.Unlock(); n++; return n }
}()

// slowCapture is the funnel with the time a large checkout needs.
func slowCapture(ctx context.Context, argv ...string) (string, int, error) {
	home, err := os.MkdirTemp("", "nself-ci-cap-")
	if err != nil {
		return "", -1, err
	}
	defer func() { _ = os.RemoveAll(home) }()
	var buf strings.Builder
	var ex ciexec.Executor
	res := ex.Run(ctx, ciexec.JobSpec{
		AttemptID: fmt.Sprintf("wt-%d-%d", time.Now().UnixNano(), worktreeSeq()), CoordinatorID: "capture",
		Job: model.Job{Kind: "static"}, Home: home, Timeout: 10 * time.Minute, Command: argv,
		Output: func(b []byte) { buf.Write(b) },
	})
	return buf.String(), res.ExitCode, res.Err
}

// AddWorktree checks rev out into a new temporary directory.
func AddWorktree(ctx context.Context, c Capture, repo, rev string) (*TempWorktree, error) {
	if c == nil {
		c = slowCapture
	}
	dir, err := os.MkdirTemp("", "nself-ci-verify-")
	if err != nil {
		return nil, fail(CodeWorktree, ExitInternal, "cannot create a temporary directory: %v", err)
	}
	w := &TempWorktree{Path: filepath.Join(dir, "tree"), dir: dir, repo: repo, c: c}
	out, exit, err := c(ctx, "git", "-C", repo, "worktree", "add", "--detach", w.Path, rev)
	if err != nil || exit != 0 {
		_ = w.Remove()
		return nil, fail(CodeWorktree, ExitInternal, "git worktree add failed for %s: %s", short(rev), firstLine(strings.TrimSpace(out)))
	}
	return w, nil
}

// Remove deletes the checkout and its git bookkeeping. It is safe to call
// more than once and from a signal handler.
func (w *TempWorktree) Remove() error {
	w.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), time.Minute)
		defer cancel()
		_, _, _ = w.c(ctx, "git", "-C", w.repo, "worktree", "remove", "--force", w.Path)
		if err := os.RemoveAll(w.dir); err != nil {
			w.err = fail(CodeWorktree, ExitInternal, "cannot remove %s: %v", w.dir, err)
		}
		_, _, _ = w.c(ctx, "git", "-C", w.repo, "worktree", "prune")
	})
	return w.err
}
