//go:build !windows

package slots

import (
	"errors"
	"syscall"
)

// alive reports whether a process with this pid exists (EPERM means it does).
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
