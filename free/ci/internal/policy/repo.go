package policy

// Purpose: questions about the repository itself that must not fail open: does
// it have an origin, and is the config directory a plain directory in both the
// protected branch and the worktree.
// Inputs: a gitRunner (the Runner) and a root or commit.
// Outputs: answers or an E672 refusal.
// Constraints: a git failure inside a repository is an error, never "no origin"
// or "no policy"; only a directory with no repository above it has no remote.

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/config"
)

// hasOrigin says whether the repository has an origin. A directory that is not
// in a repository has none; a repository whose remotes cannot be listed (git
// failed, was refused, or could not start) is an error.
func (g gitRunner) hasOrigin(ctx context.Context) (bool, error) {
	out, exit, err := g.run(ctx, append(append([]string(nil), gitScrub...), "-C", g.root, "remote")...)
	if err != nil {
		return false, refuse("cannot run git in %s: %v", g.root, err)
	}
	if exit != 0 {
		if inRepository(g.root) {
			return false, refuse("git could not read the repository at %s: %s", g.root, lastLine(out))
		}
		return false, nil
	}
	for _, r := range strings.Fields(out) {
		if r == "origin" {
			return true, nil
		}
	}
	return false, nil
}

// inRepository says whether dir is inside a work tree or is a bare repository,
// judged from the file system so a git that refuses to run still counts.
func inRepository(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return true // unknown: treat as a repository so the caller refuses
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return true // cannot place it (a missing path under an alias): treat as a repository
	}
	abs = real // a symlink into a subdirectory still sits inside its repository
	if _, err := os.Lstat(filepath.Join(abs, "HEAD")); err == nil {
		if _, err := os.Lstat(filepath.Join(abs, "objects")); err == nil {
			return true
		}
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		if filepath.Dir(d) == d {
			return false
		}
	}
}

// configDirIsPlain refuses a commit whose config directory is anything but a
// tree (a symlink there reads as "no policy" to ls-tree).
func (g gitRunner) configDirIsPlain(ctx context.Context, sha string) error {
	dir := filepath.Dir(config.FileName)
	out, ok := g.git(ctx, "ls-tree", sha, "--", dir)
	if !ok {
		return refuse("cannot list %s at %s", dir, sha[:12])
	}
	if out != "" && !strings.HasPrefix(out, "040000 tree ") {
		return refuse("%s at %s is not a plain directory", dir, sha[:12])
	}
	return nil
}

// worktreeDirIsPlain refuses a revision whose config directory is a symlink, so
// the worktree reader and the branch reader agree about the same file.
func worktreeDirIsPlain(root string) error {
	dir := filepath.Join(root, filepath.Dir(config.FileName))
	fi, err := os.Lstat(dir)
	if err != nil {
		return nil // absent: no revision policy
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return refuse("%s is a symlink", dir)
	}
	return nil
}
