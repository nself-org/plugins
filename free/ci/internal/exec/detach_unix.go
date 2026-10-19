//go:build !windows

package exec

// Purpose: the unix half of SpawnDetached (P7-CI-63): the child starts a new
// session, so it has no controlling terminal and is in no process group of
// the caller (a SIGKILL to the caller's group does not reach it).

import (
	"os/exec"
	"syscall"
)

// applyDetach puts the child in its own session (and so its own process group).
func applyDetach(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return nil
}

// TerminateDetached asks a detached child and everything in its session to
// stop (SIGTERM to the process group the child leads).
func TerminateDetached(pid int) error {
	if pid <= 1 {
		return coded(detachCode, "spawn: invalid pid")
	}
	return syscall.Kill(-pid, syscall.SIGTERM)
}
