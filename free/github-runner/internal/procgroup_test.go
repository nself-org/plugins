//go:build !windows

package internal

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A stop must reach the wrapper's children: run.sh does not forward signals.
func TestSignalRunnerTree_KillsChildren(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30 & echo $!; wait")
	ownProcessGroup(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, _ := out.Read(buf)
	child, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		t.Fatalf("child pid: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL); _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })

	if err := signalRunnerTree(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("wrapper did not exit")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(child, 0); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("child process survived the group stop")
}

// startStubbornGroup starts a process group whose members all ignore SIGTERM
// and returns the leader pid. The leader is reaped in the background, as the
// Start handler does.
func startStubbornGroup(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", `trap "" TERM; sleep 30 & sleep 30; wait`)
	ownProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	time.Sleep(200 * time.Millisecond) // let the shell install its trap
	return pid
}

func shortGrace(t *testing.T) {
	t.Helper()
	old := stopGrace
	stopGrace = 300 * time.Millisecond
	t.Cleanup(func() { stopGrace = old })
}

// SIGTERM alone leaves a runner that ignores it running; the stop must
// escalate to SIGKILL and only return once the group is gone.
func TestStopRunnerTree_EscalatesToKill(t *testing.T) {
	shortGrace(t)
	pid := startStubbornGroup(t)

	if err := stopRunnerTree(pid); err != nil {
		t.Fatalf("stopRunnerTree: %v", err)
	}
	if treeAlive(pid) {
		t.Fatal("process group still alive after stopRunnerTree returned")
	}
}

// Stop must not report stopped while the runner survives, and must report it
// once the runner is gone.
func TestStop_VerifiesTermination(t *testing.T) {
	shortGrace(t)
	pid := startStubbornGroup(t)
	state := NewRunnerState()
	state.Running, state.PID = true, pid
	h := NewHandler(state)

	rec, body := call(h.Stop, "POST")
	if rec.Code != 200 || body["stopped"] != true {
		t.Fatalf("stop: %d %v", rec.Code, body)
	}
	if treeAlive(pid) {
		t.Fatal("Stop reported stopped but the process group is still alive")
	}
}
