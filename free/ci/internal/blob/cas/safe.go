package cas

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// storeRoot keeps all named file operations beneath the blob tree.
func (s *Store) storeRoot() (*os.Root, error) {
	root, err := os.OpenRoot(s.blobs())
	if err != nil {
		return nil, err
	}
	named, err := os.Lstat(s.blobs())
	if err != nil || !named.IsDir() || named.Mode()&os.ModeSymlink != 0 {
		_ = root.Close()
		return nil, ErrInvalid
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(named, opened) {
		_ = root.Close()
		return nil, ErrInvalid
	}
	return root, nil
}

func (s *Store) storeRel(path string) (string, error) {
	rel, err := filepath.Rel(s.blobs(), path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrInvalid
	}
	return rel, nil
}

// openStoreFile refuses a final symlink and confines intermediate links to blobs.
func (s *Store) openStoreFile(path string, flags int, mode os.FileMode) (*os.File, error) {
	rel, err := s.storeRel(path)
	if err != nil {
		return nil, err
	}
	root, err := s.storeRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile(rel, flags|syscall.O_NOFOLLOW, mode)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	current, err := root.Lstat(rel)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(current, opened) {
		_ = f.Close()
		return nil, ErrInvalid
	}
	return f, nil
}

func (s *Store) readStoreFile(path string) ([]byte, error) {
	f, err := s.openStoreFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func (s *Store) removeStore(path string) error {
	if s.beforeRemove != nil {
		if err := s.beforeRemove(path); err != nil {
			return err
		}
	}
	rel, err := s.storeRel(path)
	if err != nil {
		return err
	}
	root, err := s.storeRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.Remove(rel)
}

func (s *Store) renameStore(from, to string) error {
	a, err := s.storeRel(from)
	if err != nil {
		return err
	}
	b, err := s.storeRel(to)
	if err != nil {
		return err
	}
	root, err := s.storeRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.Rename(a, b)
}

func (s *Store) syncStoreDir(path string) error {
	rel, err := s.storeRel(path)
	if err != nil {
		return err
	}
	root, err := s.storeRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	d, err := root.Open(rel)
	if err != nil {
		return err
	}
	opened, err := d.Stat()
	if err != nil {
		_ = d.Close()
		return err
	}
	current, err := root.Lstat(rel)
	if err != nil || !current.IsDir() || !os.SameFile(current, opened) {
		_ = d.Close()
		return ErrInvalid
	}
	err = syncStoreFile(d)
	closeErr := d.Close()
	if err != nil {
		return err
	}
	return closeErr
}

var syncStoreFile = (*os.File).Sync

func (s *Store) readStoreDir(path string) ([]os.DirEntry, error) {
	rel := "."
	if path != s.blobs() {
		var err error
		rel, err = s.storeRel(path)
		if err != nil {
			return nil, err
		}
	}
	root, err := s.storeRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	current, err := root.Lstat(rel)
	if err != nil || !current.IsDir() || !os.SameFile(current, opened) {
		return nil, ErrInvalid
	}
	return f.ReadDir(-1)
}
