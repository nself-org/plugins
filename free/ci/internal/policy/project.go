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
	top, ok1 := g.git(ctx, "rev-parse", "--show-toplevel")
	common, ok2 := g.git(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !ok1 || !ok2 {
		return id
	}
	top, common = ProjectID(lastLine(top)), ProjectID(lastLine(common))
	if filepath.Base(common) != ".git" {
		return id // a bare main repository or an unusual layout: keep the path
	}
	rel, err := filepath.Rel(top, id)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return id
	}
	return filepath.Clean(filepath.Join(filepath.Dir(common), rel))
}
