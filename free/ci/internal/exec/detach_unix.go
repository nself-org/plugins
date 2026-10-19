//go:build !windows

package exec

// Purpose: the unix half of SpawnDetached (P7-CI-63): the child starts a new
// session, so it has no controlling terminal and is in no process group of
// the caller (a SIGKILL to the caller's group does not reach it).
// Limit (D-1062): a cgroup or pidfd based killer (systemd KillMode=control-group,
// `docker stop`) still stops the child; the operator guide, .github/wiki/plugins/CI.md
// "Detached runs", says how to avoid that.

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
