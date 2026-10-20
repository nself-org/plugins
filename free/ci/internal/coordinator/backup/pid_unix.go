//go:build !windows

package backup

// Purpose: the default liveness probe of a coordinator process id.
// Inputs:  a pid. Outputs: whether a process with that id exists (signal 0; EPERM means it exists).
// Constraints: no signal is delivered.

import (
	"errors"
	"syscall"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
