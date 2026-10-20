package specs

// Remove cache trees without following links outside their pinned root.

import (
	"errors"
	"os"
	"path"
)

// RemoveTree deletes dir and everything in it, first making every directory
// owner-writable (a Go module cache holds read-only directories).
func RemoveTree(dir string) error {
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(dir) // a link is unlinked, its target never walked
	}
	root, err := os.OpenRoot(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = chmodDirs(root, ".")
	_ = root.Close()
	return os.RemoveAll(dir)
}

// RemoveIn deletes the entry rel of base, making directories owner-writable
// first, without ever leaving base: a rel that is a symlink out of base is
// refused (its link is removed, its target never touched).
func RemoveIn(base *os.Root, rel string) error { return removeIn(base, rel) }

// removeIn deletes the directory rel of base (same permission rule as RemoveTree)
// without ever leaving base.
func removeIn(base *os.Root, rel string) error {
	info, err := base.Lstat(rel)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return base.Remove(rel) // unlink it; never walk or chmod what it points at
	}
	if err == nil && info.IsDir() {
		if afterLstat != nil {
			afterLstat()
		}
		if sub, err := base.OpenRoot(rel); err == nil {
			// OpenRoot follows an in-root symlink, so a directory swapped for one since the
			// Lstat would be chmodded in its target's place: only the inode seen above is touched.
			if got, err := sub.Stat("."); err == nil && os.SameFile(info, got) {
				_ = chmodDirs(sub, ".")
			}
			_ = sub.Close()
		}
	}
	return removeAll(base, rel)
}

// afterLstat is a test seam between the Lstat and the OpenRoot of removeIn.
var afterLstat func()

func chmodDirs(root *os.Root, dir string) error {
	_ = root.Chmod(dir, 0o700)
	f, err := root.Open(dir)
	if err != nil {
		return err
	}
	ents, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() {
			_ = chmodDirs(root, path.Join(dir, e.Name()))
		}
	}
	return nil
}
