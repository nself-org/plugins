//go:build unix

// Purpose: the unix run-lock primitive, a non-blocking advisory flock(2) on an open file.
// Inputs: an open file. Outputs: nil when locked; isBusy tells "someone else holds it".
// Constraints: the kernel drops the lock when the holder exits or dies.

package readiness

import (
	"errors"
	"os"
	"syscall"
)

func flock(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) }

func isBusy(err error) bool { return errors.Is(err, syscall.EWOULDBLOCK) }
