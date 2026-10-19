//go:build !windows

package cas

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

// noFollow makes an open refuse a final symlink.
const noFollow = syscall.O_NOFOLLOW

// lockFile takes an exclusive advisory lock on f; unlockFile releases it.
func lockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }

func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

// lockPoll is how often a contended lock is retried while waiting.
const lockPoll = 10 * time.Millisecond

// lockFileCtx is lockFile, but polls a non-blocking attempt so a cancelled
// context ends the wait instead of blocking in the kernel.
func lockFileCtx(ctx context.Context, f *os.File) error {
	fd := int(f.Fd())
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		t := time.NewTimer(lockPoll)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
