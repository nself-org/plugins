//go:build !windows

package slots

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on f; unlock releases it. This is
// the only call of syscall.Flock in the agent.
func lockFile(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			return err
		}
	}
}

func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
