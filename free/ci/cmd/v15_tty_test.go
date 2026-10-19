//go:build !windows

package main

import (
	"os"
	"testing"
)

// TestIsTerminalRealDetector: a pipe, a regular file, /dev/null and /dev/zero
// are not terminals, whatever the mode bits say (the TestInteractiveRun seam
// mocks this away).
func TestIsTerminalRealDetector(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	file, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = null.Close() }()
	for name, f := range map[string]*os.File{"pipe": r, "file": file, "/dev/null": null} {
		if isTerminal(f) {
			t.Errorf("%s counted as a terminal", name)
		}
	}
	if zero, err := os.Open("/dev/zero"); err == nil {
		defer func() { _ = zero.Close() }()
		if isTerminal(zero) {
			t.Error("/dev/zero counted as a terminal")
		}
	}
}
