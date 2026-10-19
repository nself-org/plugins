//go:build !windows

package cas

import (
	"os"
	"syscall"
)

// oNoFollow makes an open refuse a final symlink.
const oNoFollow = syscall.O_NOFOLLOW

// flock takes an exclusive advisory lock on f and returns the function that releases it.
func flock(f *os.File) (func(), error) {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }, nil
}
