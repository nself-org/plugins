package policy

// Purpose: the project-scope subject of a repository root, so an operator row
// written for a project applies however the repository is reached.
// Inputs: a root path (and a Runner for the git-aware form).
// Outputs: an absolute, symlink-free path; with git, a linked worktree maps to
// the same path inside its main worktree, so monorepo sub-projects keep their
// own identity.
// Constraints: ProjectID is pure filesystem; ProjectIDGit adds two git spawns
// through the Runner. A project row that fails to match loosens the policy, so
// the write side (the operator command) must use ProjectIDGit too.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ProjectID is the project-scope subject of a repository root: absolute, with
// symlinks resolved. A path that does not exist is cleaned only.
func ProjectID(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(real)
	}
	return filepath.Clean(abs)
}

// projectIDWindow is how long a ProjectIDGitCached answer is reused. A run asks
// for its project id three times on its start path (plan, policy hook, dispatch),
// each with three git spawns; the repository's worktree layout does not change
// inside a run's start (D-1008).
const projectIDWindow = 10 * time.Second

var projectIDs = struct {
	sync.Mutex
	byRoot map[string]cachedProjectID
}{byRoot: map[string]cachedProjectID{}}

type cachedProjectID struct {
	id string
	at time.Time
}

// ProjectIDGitCached is ProjectIDGit with the answer kept for projectIDWindow,
// keyed by the root's filesystem id. Production start paths use it; tests that
// swap the Runner use ProjectIDGit.
func ProjectIDGitCached(ctx context.Context, run Runner, root string) string {
	key := ProjectID(root)
	projectIDs.Lock()
	hit, ok := projectIDs.byRoot[key]
	projectIDs.Unlock()
	if ok && time.Since(hit.at) < projectIDWindow {
		return hit.id
	}
	id := ProjectIDGit(ctx, run, root)
	projectIDs.Lock()
	projectIDs.byRoot[key] = cachedProjectID{id, time.Now()}
	projectIDs.Unlock()
	return id
}

// ProjectIDGit is the project id built from the paths git prints (its
// canonical case and real paths): the main worktree's top level joined with the
// path of root below the top. A linked worktree maps to the matching path in
// its main worktree. Outside a repository it is ProjectID.
func ProjectIDGit(ctx context.Context, run Runner, root string) string {
	id := ProjectID(root)
	if run == nil {
		return id
	}
	g := gitRunner{run: run, root: id}
	// The worktree list does not depend on the top level, so it runs beside it:
	// both are funnel spawns on a run's start path (D-1008). Outside a repository
	// it is one extra failed spawn that is ignored.
	type listing struct {
		out string
		ok  bool
	}
	listed := make(chan listing, 1)
	go func() {
		out, ok := g.raw(ctx, "worktree", "list", "--porcelain", "-z")
		listed <- listing{out, ok}
	}()
	top, prefix, ok := g.topAndPrefix(ctx)
	wt := <-listed
	if !ok || top == "" {
		return id // not a repository
	}
	base := top
	// NUL-terminated records: nothing in a path needs quoting.
	if listed, ok := wt.out, wt.ok; ok {
		// The first record group is the main worktree, whatever the layout. A bare
		// main repository has no checkout, and --separate-git-dir lists the git
		// directory (git records no path to that checkout), so a candidate without a
		// .git entry is not used: the worktree's own top level stays.
		main, bare := "", false
		for _, field := range strings.Split(listed, "\x00") {
			if field == "" {
				break // the empty field ends the first group
			}
			if p, found := strings.CutPrefix(field, "worktree "); found {
				main = p
			}
			bare = bare || field == "bare"
		}
		if _, err := os.Lstat(filepath.Join(main, ".git")); main != "" && !bare && err == nil {
			base = main
		}
	}
	return filepath.Clean(filepath.Join(base, filepath.FromSlash(prefix)))
}

// topAndPrefix is `rev-parse --show-toplevel` and `--show-prefix` in one spawn.
// Raw output: a path may start or end with a space, so only the final newline
// git adds is removed. A path holding a newline makes the two values ambiguous
// in one stream, and then each is asked on its own.
func (g gitRunner) topAndPrefix(ctx context.Context) (top, prefix string, ok bool) {
	out, ok := g.raw(ctx, "rev-parse", "--show-toplevel", "--show-prefix")
	if !ok {
		return "", "", false
	}
	if t, p, found := strings.Cut(strings.TrimSuffix(out, "\n"), "\n"); found && !strings.Contains(p, "\n") {
		return t, p, true
	}
	topOut, ok := g.raw(ctx, "rev-parse", "--show-toplevel")
	if !ok {
		return "", "", false
	}
	prefixOut, ok := g.raw(ctx, "rev-parse", "--show-prefix")
	return strings.TrimSuffix(topOut, "\n"), strings.TrimSuffix(prefixOut, "\n"), ok
}
