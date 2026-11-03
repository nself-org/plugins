//go:build !windows

package exec

import (
	"errors"
	"os"
	"syscall"
)

// lockWorkspace tries an exclusive lock without waiting; contention returns false.
func lockWorkspace(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

// unlockWorkspace releases the lock before its file is closed.
func unlockWorkspace(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
