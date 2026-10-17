package internal

import (
	"os"
	"os/exec"
	"syscall"
)

// The runner is a tree: run.sh starts Runner.Listener and the job processes,
// and run.sh does not forward signals to them. The runner therefore starts in
// its own process group and a stop signals the whole group, so stopping the
// wrapper cannot leave a listener that still takes jobs.

// ownProcessGroup makes cmd the leader of a new process group.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalRunnerTree sends SIGTERM to the process group led by pid, and falls
// back to the single process when pid does not lead a group (a runner started
// by another path).
func signalRunnerTree(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err == nil {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}
