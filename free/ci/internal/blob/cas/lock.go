package cas

import (
	"errors"
	"os"
	"syscall"
)

// locked serializes tree mutations across Store values and processes.
func (s *Store) locked(fn func() error) error {
	if err := s.secureDir(s.blobs()); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.blobs())
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile(".lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if errors.Is(err, os.ErrExist) {
		info, e := root.Lstat(".lock")
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return ErrInvalid
		}
		f, err = root.OpenFile(".lock", os.O_RDWR, 0)
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}
