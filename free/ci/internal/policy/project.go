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

// ProjectIDGit is ProjectID, except a linked worktree maps to the matching
// path in its main worktree.
func ProjectIDGit(ctx context.Context, run Runner, root string) string {
	id := ProjectID(root)
	if run == nil {
		return id
	}
	g := gitRunner{run: run, root: id}
	top, ok := g.git(ctx, "rev-parse", "--show-toplevel")
	if !ok {
		return id // not a repository: one failed spawn and done
	}
	listed, ok := g.git(ctx, "worktree", "list", "--porcelain")
	if !ok {
		return id
	}
	// The first entry is the main worktree, whatever the layout (separate git
	// dirs included); a bare main repository has no checkout to map to.
	first, _, _ := strings.Cut(strings.ReplaceAll(listed, "\r\n", "\n"), "\n\n")
	var main string
	for _, line := range strings.Split(first, "\n") {
		if p, found := strings.CutPrefix(line, "worktree "); found {
			main = p
		}
		if line == "bare" {
			return id
		}
	}
	if main == "" {
		return id
	}
	// --separate-git-dir lists the git directory as the main worktree and git
	// records no path to the checkout, so a candidate without a .git entry cannot
	// be mapped: keep the path as is (the checkout itself is unaffected).
	if _, err := os.Lstat(filepath.Join(main, ".git")); err != nil {
		return id
	}
	top, main = ProjectID(lastLine(top)), ProjectID(main)
	rel, err := filepath.Rel(top, id)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return id
	}
	return filepath.Clean(filepath.Join(main, rel))
}
