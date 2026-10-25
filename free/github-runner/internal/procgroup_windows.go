//go:build windows

package internal

import (
	"os"
	"os/exec"
)

// Windows has no process groups in the POSIX sense; stop the single process.
func ownProcessGroup(cmd *exec.Cmd) {}

func signalRunnerTree(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// stopRunnerTree kills the single process; Kill is already forceful.
func stopRunnerTree(pid int) error {
	return signalRunnerTree(pid)
}
