package main

// Purpose: the hook and audit modes of `nself ci policy protect` (P7-CI-07).
// Inputs:  protectFlags and the protectEnv of policy_protect.go.
// Outputs: text or one v1 JSON envelope; audit exits 1 when a commit has no
// green nself-ci, 0 when all are green.
// Constraints: the hook goes to the git hooks path of the root (core.hooksPath
// respected) and is never overwritten; the audit only reads.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/protect"
)

type hookResult struct {
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	Installed bool   `json:"installed"`
}

func (e protectEnv) installHook(ctx context.Context, f protectFlags) error {
	path, err := e.hookPath(ctx)
	if err != nil {
		return err
	}
	branch := f.branch
	if branch == "" {
		branch = e.localDefaultBranch(ctx)
	}
	changed, err := protect.InstallHook(path, branch)
	if err != nil {
		return err
	}
	if f.json {
		return output.WriteData(e.out, "ci policy protect", hookResult{Path: path, Branch: branch, Installed: changed}, nil)
	}
	if changed {
		fmt.Fprintf(e.out, "pre-push hook installed at %s: pushes to %s need current, bound, passing evidence (bypass once with `git push --no-verify`)\n", path, branch)
	} else {
		fmt.Fprintf(e.out, "pre-push hook at %s is already installed\n", path)
	}
	return nil
}

// hookPath asks git where pre-push lives (worktrees and core.hooksPath included).
func (e protectEnv) hookPath(ctx context.Context) (string, error) {
	out, exit, err := e.run(ctx, "git", "-C", e.root, "rev-parse", "--git-path", "hooks/pre-push")
	if err != nil || exit != 0 {
		return "", errors.New("policy protect: --hook needs a git repository (run it inside one, or set NSELF_CI_REPO)")
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(e.root, p)
	}
	return p, nil
}

// localDefaultBranch reads origin/HEAD; main when it is not set.
func (e protectEnv) localDefaultBranch(ctx context.Context) string {
	out, exit, err := e.run(ctx, "git", "-C", e.root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if b := strings.TrimPrefix(strings.TrimSpace(out), "origin/"); err == nil && exit == 0 && b != "" {
		return b
	}
	return "main"
}

func (e protectEnv) audit(ctx context.Context, f protectFlags, repo string) int {
	c := protect.Client{Run: e.run}
	branch, err := e.branchOf(ctx, c, f, repo)
	var rep protect.AuditReport
	if err == nil {
		rep, err = c.Audit(ctx, repo, branch, f.audit, e.now())
	}
	if err != nil {
		return protectExit(e.noPlanHint(err), f.json)
	}
	if f.json {
		_ = output.WriteData(e.out, "ci policy protect", rep, nil)
	} else {
		fmt.Fprintf(e.out, "%s %s: %d commit(s) in the last %d day(s), %d without a green nself-ci\n", rep.Repo, rep.Branch, rep.Checked, rep.Days, len(rep.Missing))
		for _, m := range rep.Missing {
			fmt.Fprintf(e.out, "  %.12s %s\n", m.SHA, m.Subject)
		}
		if rep.Truncated {
			fmt.Fprintf(e.out, "  (only the newest %d commits were looked at)\n", protect.MaxCommits)
		}
	}
	if len(rep.Missing) > 0 {
		return 1
	}
	return 0
}
