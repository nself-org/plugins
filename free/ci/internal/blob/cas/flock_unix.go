//go:build !windows

package cas

import (
	"os"
	"syscall"
)

// noFollow makes an open refuse a final symlink.
const noFollow = syscall.O_NOFOLLOW

// lockFile takes an exclusive advisory lock on f; unlockFile releases it.
func lockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }

func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
