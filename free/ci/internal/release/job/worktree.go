package job

// Purpose: the worktree probe of the release gate: which commit is checked out
// under a root and whether any file differs from it.
// Inputs:  a Capture that runs git (the run's exec funnel; this package never
// starts a process) and a root.
// Outputs: a Worktree: the full HEAD sha and a dirty flag.
// Constraints: dirty when `git status` shows any change, when the index hides a
// file from status (assume-unchanged is a lower-case tag in `ls-files -v`,
// skip-worktree an S; the run's own evidence treats both as unbound), and when
// any git call fails (unknown is not clean); an unreadable HEAD gives sha "".

import (
	"context"
	"regexp"
	"strings"
)

// Capture runs a process and returns its merged output and exit code.
type Capture func(ctx context.Context, argv ...string) (out string, exit int, err error)

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// GitWorktree returns a Worktree probe over capture.
func GitWorktree(capture Capture) Worktree {
	return func(ctx context.Context, root string) (string, bool) {
		git := func(args ...string) (string, bool) {
			out, exit, err := capture(ctx, append([]string{"git", "-c", "core.fsmonitor=false", "-C", root}, args...)...)
			return strings.TrimSpace(out), err == nil && exit == 0
		}
		sha, ok := git("rev-parse", "HEAD")
		if !ok || !fullSHA.MatchString(sha) {
			return "", true
		}
		if status, ok := git("status", "--porcelain", "--untracked-files=normal"); !ok || status != "" {
			return sha, true
		}
		files, ok := git("ls-files", "-v")
		if !ok {
			return sha, true
		}
		for _, line := range strings.Split(files, "\n") {
			if line != "" && (line[0] == 'S' || line[0] >= 'a' && line[0] <= 'z') {
				return sha, true
			}
		}
		return sha, false
	}
}
