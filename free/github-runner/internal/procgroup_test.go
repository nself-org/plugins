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
