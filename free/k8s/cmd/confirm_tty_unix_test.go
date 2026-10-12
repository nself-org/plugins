//go:build linux || darwin

// Purpose: tests for isTerminal and the uninstall gate on a non-terminal
// character device (D-0447).
//
// Inputs: /dev/null, /dev/zero, /dev/ptmx, a temp file, a pipe; the binary
// from cli_harness_test.go.
//
// Outputs: test results.
//
// Constraints: no network, no cluster. A pty master is the positive control:
// it answers the termios ioctl, so a check that always says false fails here.
package main

import (
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestIsTerminal(t *testing.T) {
	for _, dev := range []string{os.DevNull, "/dev/zero"} {
		f, err := os.Open(dev)
		if err != nil {
			t.Fatalf("open %s: %v", dev, err)
		}
		if isTerminal(f.Fd()) {
			t.Errorf("isTerminal(%s) = true, want false", dev)
		}
		f.Close()
	}
	file, err := os.CreateTemp(t.TempDir(), "plain")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if isTerminal(file.Fd()) {
		t.Error("isTerminal(regular file) = true, want false")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r.Fd()) {
		t.Error("isTerminal(pipe) = true, want false")
	}
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pty for the positive control: %v", err)
	}
	defer master.Close()
	slave, err := openPTYSlave(master)
	if err != nil {
		t.Fatalf("opening the pty terminal side: %v", err)
	}
	defer slave.Close()
	if !isTerminal(slave.Fd()) {
		t.Error("isTerminal(pty) = false, want true")
	}
}

// A stdin of /dev/zero is a character device but no person: uninstall must
// refuse without prompting or reading it (before D-0447 it prompted and read).
func TestUninstallRefusesDevZeroWithoutPrompt(t *testing.T) {
	zero, err := os.Open("/dev/zero")
	if err != nil {
		t.Fatal(err)
	}
	defer zero.Close()
	r := runCLI(t, cliRun{Helm: "ok", Stdin: zero}, "uninstall")
	if r.Exit != 4 || !strings.Contains(r.Stderr, "needs confirmation") {
		t.Errorf("exit = %d, want 4 with the refusal\nstderr:\n%s", r.Exit, r.Stderr)
	}
	if strings.Contains(r.Stderr, "Type the release name") {
		t.Errorf("prompted a non-terminal stdin:\n%s", r.Stderr)
	}
	if calls := argvCalls(t, r.ArgvFile); calls != nil {
		t.Fatalf("a refused uninstall ran helm: %v", calls)
	}
}
