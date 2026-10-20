//go:build !windows

package main

// Purpose: TestServeSecondSignalIsNotSwallowed (D-1308): after the first signal starts the drain, the
//          process stops catching signals, so a second one gets the default action and aborts a stuck drain.
// Inputs:  a re-executed test binary that catches SIGUSR1 through signalContext, then receives it twice.
// Outputs: pass or fail.
// Constraints: unix only (SIGUSR1); the child waits 3 s after the second signal, so a swallowed signal
//          shows as a clean exit, a default action as death by SIGUSR1.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestSignalContextChild(t *testing.T) {
	if os.Getenv("NSELF_CI_SIGNAL_CHILD") != "1" {
		t.Skip("child half of TestServeSecondSignalIsNotSwallowed")
	}
	ctx, stop := signalContext(context.Background(), syscall.SIGUSR1)
	defer stop()
	_ = syscall.Kill(os.Getpid(), syscall.SIGUSR1)
	<-ctx.Done()
	time.Sleep(300 * time.Millisecond) // the drain is under way
	_ = syscall.Kill(os.Getpid(), syscall.SIGUSR1)
	time.Sleep(3 * time.Second) // a swallowed second signal lets the child get here
}

func TestServeSecondSignalIsNotSwallowed(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestSignalContextChild$")
	cmd.Env = append(os.Environ(), "NSELF_CI_SIGNAL_CHILD=1")
	err := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("the second signal was swallowed: child exited %v", err)
	}
	ws, ok := ee.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGUSR1 {
		t.Fatalf("child ended %v, want death by SIGUSR1", ee)
	}
}
