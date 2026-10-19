package run

// Purpose: the git facts a run binds to: the revision and whether the
// worktree is clean. Every call goes through the exec funnel.
// Inputs:  the repository root. Outputs: a Revision.
// Constraints: a missing git or a repository with no commit gives an empty
// SHA, never an error: the run still gates.

import (
	"context"
	"regexp"
	"strings"
	"sync"
)

// Capture runs a process and returns merged output and its exit code.
type Capture func(ctx context.Context, argv ...string) (string, int, error)

// Revision is the commit a run is bound to.
type Revision struct {
	SHA   string
	Dirty bool
}

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// GitRevision reads HEAD and the worktree state of root. The two questions are
// independent, so they run side by side: each is a funnel spawn on the start
// path of every run (D-1008). The state of a repository with no commit is not
// used.
func GitRevision(ctx context.Context, root string, capture Capture) Revision {
	git := func(args ...string) (string, bool) {
		out, exit, err := capture(ctx, append([]string{"git", "-c", "core.fsmonitor=false", "-C", root}, args...)...)
		return strings.TrimSpace(out), err == nil && exit == 0
	}
	var rev Revision
	var status string
	var statusOK bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		status, statusOK = git("status", "--porcelain", "--untracked-files=normal")
	}()
	sha, ok := git("rev-parse", "HEAD")
	wg.Wait()
	if !ok || !fullSHA.MatchString(sha) {
		return rev
	}
	rev.SHA = sha
	rev.Dirty = !statusOK || status != ""
	return rev
}
