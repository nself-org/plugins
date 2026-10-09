package cas

import (
	"os"
	"path/filepath"
	"strings"
)

// walkFiles opens the requested directory and checks that its name still denotes
// the same inode. Each child directory is pinned separately before traversal.
func (s *Store) walkFiles(path string, fn func(string, os.FileInfo, func() error) error) error {
	rel, err := s.storeRel(path)
	if err != nil {
		return err
	}
	root, err := s.storeRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	parts := strings.Split(rel, string(os.PathSeparator))
	parent := root
	currentPath := s.blobs()
	for _, part := range parts[:len(parts)-1] {
		currentPath = filepath.Join(currentPath, part)
		before, err := parent.Lstat(part)
		if err != nil {
			return err
		}
		if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		next, err := parent.OpenRoot(part)
		if err != nil {
			return err
		}
		defer func() { _ = next.Close() }()
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(before, opened) {
			return ErrInvalid
		}
		named, err := os.Lstat(currentPath)
		if err != nil || !os.SameFile(named, opened) {
			return ErrInvalid
		}
		parent = next
	}
	return s.walkDirectory(parent, parts[len(parts)-1], path, fn)
}

func (s *Store) walkDirectory(parent *os.Root, name, path string, fn func(string, os.FileInfo, func() error) error) error {
	if err := s.checkDir(path); err != nil {
		return err
	}
	before, err := parent.Lstat(name)
	if err != nil {
		return err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return ErrInvalid
	}
	dir, err := parent.OpenRoot(name)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	opened, err := dir.Stat(".")
	if err != nil {
		return err
	}
	current, err := parent.Lstat(name)
	if err != nil || !os.SameFile(before, opened) || !os.SameFile(before, current) {
		return ErrInvalid
	}
	named, err := os.Lstat(path)
	if err != nil || !os.SameFile(named, opened) {
		return ErrInvalid
	}
	f, err := dir.Open(".")
	if err != nil {
		return err
	}
	entries, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		return err
	}
	if s.afterWalkReadDir != nil {
		s.afterWalkReadDir(path)
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		info, err := dir.Lstat(entry.Name())
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		if info.IsDir() {
			if err := s.walkDirectory(dir, entry.Name(), child, fn); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return ErrInvalid
		}
		remove := func() error {
			if err := s.checkDir(path); err != nil {
				return err
			}
			currentDir, err := os.Lstat(path)
			if err != nil || !os.SameFile(currentDir, opened) {
				return ErrInvalid
			}
			currentFile, err := dir.Lstat(entry.Name())
			if err != nil || !os.SameFile(currentFile, info) {
				return ErrInvalid
			}
			return dir.Remove(entry.Name())
		}
		if err := fn(child, info, remove); err != nil {
			return err
		}
	}
	return nil
}
