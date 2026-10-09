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
	root, err := s.storeRoot()
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
		if s.afterLockStat != nil {
			s.afterLockStat()
		}
		f, err = root.OpenFile(".lock", os.O_RDWR|syscall.O_NOFOLLOW, 0)
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	current, err := root.Lstat(".lock")
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(current, opened) {
		return ErrInvalid
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	return fn()
}
