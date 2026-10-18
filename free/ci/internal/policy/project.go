package policy

// Purpose: the project-scope subject of a repository root, so an operator row
// written for a project applies however the repository is reached.
// Inputs: a root path (and a Runner for the git-aware form).
// Outputs: an absolute, symlink-free path; with git, a linked worktree maps to
// the same path inside its main worktree, so monorepo sub-projects keep their
// own identity.
// Constraints: ProjectID is pure filesystem; ProjectIDGit adds two git reads
// through the Runner. A project row that fails to match loosens the policy, so
// the write side (the operator command) must use ProjectIDGit too.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	// Raw output, one value per call: a path may start or end with a space or
	// hold a newline, so only the final newline git adds is removed.
	topOut, ok := g.raw(ctx, "rev-parse", "--show-toplevel")
	if !ok {
		return id // not a repository: one failed spawn and done
	}
	prefixOut, ok := g.raw(ctx, "rev-parse", "--show-prefix")
	top, prefix := strings.TrimSuffix(topOut, "\n"), strings.TrimSuffix(prefixOut, "\n")
	if !ok || top == "" {
		return id
	}
	base := top
	// NUL-terminated records: nothing in a path needs quoting.
	if listed, ok := g.raw(ctx, "worktree", "list", "--porcelain", "-z"); ok {
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
