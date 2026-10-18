package authz

// Purpose: where `nself ci artifacts get` may write (R-SEC-29): a fresh, empty
// directory that is not the working directory and not under a .git directory.
// Inputs:  the requested destination and the caller's working directory.
// Outputs: the absolute, symlink-resolved destination, or a *PathError (E683).
// Constraints: the only I/O in this package (stat, read the directory, resolve
// symlinks). It creates nothing: the caller creates a missing destination with
// os.Mkdir (not MkdirAll) mode 0700, which fails if anything appeared in the
// meantime, and passes the returned path on. A leaf that is a symlink is
// refused, so the check and the write name the same directory.

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	DestEmpty       PathReason = "dest.empty"
	DestCwd         PathReason = "dest.cwd"
	DestUnderGit    PathReason = "dest.under_git"
	DestNotEmpty    PathReason = "dest.not_empty"
	DestNotDir      PathReason = "dest.not_dir"
	DestSymlink     PathReason = "dest.symlink"
	DestNoParent    PathReason = "dest.no_parent"
	DestUnreachable PathReason = "dest.unreachable"
)

// ArtifactGetDest validates dest. A relative dest is taken relative to cwd
// (which must be absolute). The result is absolute with every symlink in the
// existing part resolved.
func ArtifactGetDest(dest, cwd string) (string, error) {
	if dest == "" || cwd == "" || !filepath.IsAbs(cwd) {
		return "", reject(DestEmpty, dest)
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(cwd, dest)
	}
	dest = filepath.Clean(dest)
	realCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", reject(DestUnreachable, cwd)
	}
	if underGit(dest) {
		return "", reject(DestUnderGit, dest)
	}
	resolved, exists, err := resolveDest(dest)
	if err != nil {
		return "", err
	}
	if underGit(resolved) {
		return "", reject(DestUnderGit, dest)
	}
	if resolved == realCwd || resolved == filepath.Clean(cwd) {
		return "", reject(DestCwd, dest)
	}
	if exists {
		if err := requireEmptyDir(resolved, dest); err != nil {
			return "", err
		}
	}
	return resolved, nil
}

// resolveDest resolves symlinks in dest's existing part. For a missing leaf
// the parent must exist and be a directory.
func resolveDest(dest string) (resolved string, exists bool, err error) {
	info, lerr := os.Lstat(dest)
	switch {
	case lerr == nil:
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", false, reject(DestSymlink, dest)
		}
		real, rerr := filepath.EvalSymlinks(dest)
		if rerr != nil {
			return "", false, reject(DestUnreachable, dest)
		}
		return real, true, nil
	case !errors.Is(lerr, fs.ErrNotExist):
		return "", false, reject(DestUnreachable, dest)
	}
	parent, rerr := filepath.EvalSymlinks(filepath.Dir(dest))
	if rerr != nil {
		return "", false, reject(DestNoParent, dest)
	}
	if pi, serr := os.Stat(parent); serr != nil || !pi.IsDir() {
		return "", false, reject(DestNoParent, dest)
	}
	return filepath.Join(parent, filepath.Base(dest)), false, nil
}

func requireEmptyDir(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return reject(DestUnreachable, name)
	}
	if !info.IsDir() {
		return reject(DestNotDir, name)
	}
	f, err := os.Open(path)
	if err != nil {
		return reject(DestUnreachable, name)
	}
	defer f.Close()
	if names, err := f.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) || len(names) > 0 {
		return reject(DestNotEmpty, name)
	}
	return nil
}

// underGit reports whether any path component is ".git" (any letter case, since
// macOS and Windows volumes fold case).
func underGit(p string) bool {
	for _, c := range strings.Split(filepath.ToSlash(p), "/") {
		if strings.EqualFold(c, ".git") {
			return true
		}
	}
	return false
}
