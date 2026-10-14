package config

// Purpose: warn (E623) when `.nself/ci.yaml` is git-ignored, because `nself
// init` ignores `.nself/` and an ignored file is never committed (Epic D20).
// Inputs:  a repo root and a Runner that executes `git check-ignore`.
// Outputs: a Warning carrying the exact two-line fix, or nil.
// Constraints: the git process runs through the exec funnel (internal/exec),
// the only package allowed to start processes; the check never edits
// `.gitignore`; a directory that is not a git repository is not a warning.

import (
	"context"
	"fmt"
	"os"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// IgnoreFix is the two-line remedy of Epic D20.
const IgnoreFix = ".nself/*\n!.nself/ci.yaml"

// Runner runs argv to completion and returns its exit code.
type Runner func(ctx context.Context, argv []string) (int, error)

// ExecRunner runs argv in a throwaway funnel workspace.
func ExecRunner() Runner {
	return func(ctx context.Context, argv []string) (int, error) {
		home, err := os.MkdirTemp("", "nself-ci-config-")
		if err != nil {
			return -1, err
		}
		defer func() { _ = os.RemoveAll(home) }()
		var ex ciexec.Executor
		res := ex.Run(ctx, ciexec.JobSpec{
			AttemptID:     fmt.Sprintf("config-ignore-%d", time.Now().UnixNano()),
			CoordinatorID: "config",
			Job:           model.Job{Kind: "static"},
			Command:       argv,
			Home:          home,
			Timeout:       30 * time.Second,
		})
		return res.ExitCode, res.Err
	}
}

// CheckIgnored returns the E623 warning when root's `.nself/ci.yaml` is ignored.
func CheckIgnored(ctx context.Context, root string, run Runner) (*Warning, error) {
	if run == nil {
		return nil, nil
	}
	code, err := run(ctx, []string{"git", "-C", root, "check-ignore", "-q", "--", FileName})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, nil // 1: not ignored; 128: not a git repository
	}
	return &Warning{
		Code: CodeIgnored,
		Message: FileName + " is git-ignored, so it would never be committed. In .gitignore replace a `.nself/` line with these two lines " +
			"(a negation under an ignored directory has no effect):\n  .nself/*\n  !.nself/ci.yaml",
	}, nil
}
