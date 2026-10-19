//go:build unix

package readiness

import (
	"errors"
	"os"
	"syscall"
)

func flock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) }

func isBusy(err error) bool { return errors.Is(err, syscall.EWOULDBLOCK) }
