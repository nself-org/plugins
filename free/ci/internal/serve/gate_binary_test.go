package serve

// Purpose: the serve job runs the gate by exec of os.Executable(), never by
// PATH lookup or go build (P7-CANON-09 constraint).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGateBinaryIsTheRunningExecutable ignores PATH entirely.
func TestGateBinaryIsTheRunningExecutable(t *testing.T) {
	dir := t.TempDir()
	// A decoy nself-ci first on PATH must never be chosen.
	if err := os.WriteFile(filepath.Join(dir, "nself-ci"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, err := gateBinary()
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	want, _ := filepath.EvalSymlinks(exe)
	if got != want {
		t.Fatalf("gateBinary = %q, want the running executable %q", got, want)
	}
	if filepath.Dir(got) == dir {
		t.Fatal("gateBinary resolved through PATH")
	}
}

// TestJobArgsUseNoStatus keeps the job argv free of --check (core framing
// would change the summary line the job extracts).
func TestJobArgsUseNoStatus(t *testing.T) {
	b, err := os.ReadFile("serve_job.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, `"--no-status", "--no-gitleaks"`) || strings.Contains(src, `"--check"`) {
		t.Fatal("serve_job.go must run the gate with --no-status --no-gitleaks and never --check")
	}
}
