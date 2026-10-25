//go:build !windows

package internal

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// stopGrace is how long a stop waits after SIGTERM before it escalates to
// SIGKILL, and again after SIGKILL before it reports the runner as still up.
// A variable so tests can shorten it.
var stopGrace = 10 * time.Second

// stopPoll is the interval at which a stop re-checks whether the tree is gone.
const stopPoll = 50 * time.Millisecond

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
	return signalTree(pid, syscall.SIGTERM)
}

func signalTree(pid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pid, sig); err == nil {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}

// treeAlive reports whether any process of the group led by pid (or, for a
// runner that leads no group, the process itself) still exists.
func treeAlive(pid int) bool {
	return syscall.Kill(-pid, 0) == nil || syscall.Kill(pid, 0) == nil
}

// waitTreeGone polls until the tree is gone or d passes, and reports whether
// it is gone.
func waitTreeGone(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if !treeAlive(pid) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(stopPoll)
	}
}

// stopRunnerTree sends SIGTERM to the runner tree, waits up to stopGrace for
// it to exit, escalates to SIGKILL for survivors, and returns an error unless
// the whole tree is verified gone.
func stopRunnerTree(pid int) error {
	if err := signalRunnerTree(pid); err != nil {
		if !treeAlive(pid) {
			return nil
		}
		return err
	}
	if waitTreeGone(pid, stopGrace) {
		return nil
	}
	if err := signalTree(pid, syscall.SIGKILL); err != nil && !treeAlive(pid) {
		return nil
	}
	if waitTreeGone(pid, stopGrace) {
		return nil
	}
	return fmt.Errorf("runner process tree %d still running after SIGKILL", pid)
}
