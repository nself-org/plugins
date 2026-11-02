package run

// Purpose: decide whether the config file a run read is the one the commit
// holds, so evidence built from it can be bound to that commit (D-1118).
// Inputs:  the repository root, the config path in use, its repo-relative name
//          and a Capture for git.
// Outputs: true when the file cannot be vouched for by HEAD.
// Constraints: a missing file reads nothing and is not outside. A file is
// inside only when its path is the repo's own, it and its directories are not
// symlinks, HEAD holds it as a regular blob and the working copy hashes to that
// blob (a git status can miss an ignored, assume-unchanged or skip-worktree
// file, and a tracked symlink can point out of the tree).

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/gitbatch"
)

// headEntry is `ls-tree HEAD -- rel` as its three fields (mode, type, id); none when HEAD has no such
// path. The batch session answers when the run has one, else git.
func headEntry(ctx context.Context, root, rel string, git func(...string) (string, bool)) ([]string, bool) {
	if e, found, ok, used := gitbatch.LsTree(ctx, root, "HEAD", rel); used {
		if !ok || !found {
			return nil, ok
		}
		return []string{e.Mode, e.Type, e.OID}, true
	}
	tree, ok := git("ls-tree", "HEAD", "--", rel)
	meta, _, _ := strings.Cut(tree, "\t")
	return strings.Fields(meta), ok
}

// ConfigOutsideTree reports whether the config at path is not the regular file
// rel of commit HEAD in root, byte for byte.
func ConfigOutsideTree(ctx context.Context, root, path, rel string, capture Capture) bool {
	abs, err := filepath.Abs(path)
	if err != nil || filepath.Clean(abs) != filepath.Join(root, rel) {
		return true
	}
	fi, err := os.Lstat(abs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false
	case err != nil || !fi.Mode().IsRegular():
		return true // unreadable, a symlink, a directory or a device
	}
	real, err := filepath.EvalSymlinks(abs)
	realRoot, rerr := filepath.EvalSymlinks(root)
	if err != nil || rerr != nil || real != filepath.Join(realRoot, rel) {
		return true // a directory on the way is a link
	}
	git := func(args ...string) (string, bool) {
		out, exit, err := capture(ctx, append([]string{"git", "-c", "core.fsmonitor=false", "-C", root}, args...)...)
		return strings.TrimSpace(out), err == nil && exit == 0
	}
	fields, ok := headEntry(ctx, root, rel, git)
	if !ok || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" {
		return true
	}
	have, ok := git("hash-object", "--", rel)
	return !ok || have != fields[2]
}
