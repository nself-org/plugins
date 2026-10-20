//go:build unix

package tart

// Purpose: advisory file locks that the kernel drops when the holder dies, even by kill -9.
// Constraints: the lock is held on an open file description that is never inherited (Go opens close-on-exec).

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes an exclusive lock without waiting; false means another process holds it.
func tryLock(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}
