package serve

// Purpose: Tests for the T32 fail-closed Docker-absent behavior in
//
//	runGateInDocker — a job must refuse to run unsandboxed on the host
//	when Docker is unavailable, unless the operator explicitly opts in.
//
// Inputs:  a PATH manipulated to guarantee `docker` is not resolvable
// Outputs: pass/fail refusal behavior of runGateInDocker
// Constraints: does not require Docker or the nself-ci binary to be present;
//
//	exercises only the LookPath("docker") branch and its two outcomes.
//
// SPORT: CLI-CMD-CI-SERVE-001

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withNoDockerOnPath points PATH at an empty temp dir for the duration of
// the test, guaranteeing exec.LookPath("docker") fails regardless of what
// is actually installed on the host running this test suite.
func withNoDockerOnPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// TestRunGateInDocker_RefusesWhenDockerAbsentAndNotAllowed (T32) is the
// core fail-closed assertion: Docker missing + AllowUnsandboxed=false must
// refuse the job with a clear error, never silently fall back to running
// the cloned repo's gate commands directly on the host.
func TestRunGateInDocker_RefusesWhenDockerAbsentAndNotAllowed(t *testing.T) {
	withNoDockerOnPath(t)

	cfg := ServeConfig{JobTimeout: 5, AllowUnsandboxed: false}
	passed, summary, err := runGateInDocker("/usr/local/bin/nself-ci", t.TempDir(), cfg, "main")

	if err == nil {
		t.Fatal("runGateInDocker with Docker absent and AllowUnsandboxed=false: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to run gate unsandboxed") {
		t.Errorf("unexpected error: %v", err)
	}
	if passed {
		t.Error("passed = true, want false for a refused job")
	}
	if summary != "Docker not available" {
		t.Errorf("summary = %q, want %q", summary, "Docker not available")
	}
}

// TestRunGateInDocker_FallsBackWhenAllowUnsandboxedSet (T32) proves the
// opt-in path still works for legitimate no-Docker deployments: with
// AllowUnsandboxed=true, runGateInDocker must NOT return the "refusing to
// run gate unsandboxed" error — it proceeds to runGateDirect instead (which
// will itself fail here because the binary path is fake, but that failure
// is a different, unrelated error, proving the fallback branch was taken).
func TestRunGateInDocker_FallsBackWhenAllowUnsandboxedSet(t *testing.T) {
	withNoDockerOnPath(t)

	cfg := ServeConfig{JobTimeout: 5, AllowUnsandboxed: true}
	_, _, err := runGateInDocker("/nonexistent/nself-ci-binary", t.TempDir(), cfg, "main")

	if err == nil {
		t.Fatal("expected an error from runGateDirect against a nonexistent binary, got nil")
	}
	if strings.Contains(err.Error(), "refusing to run gate unsandboxed") {
		t.Errorf("AllowUnsandboxed=true still hit the refusal branch: %v", err)
	}
}

func TestServedRefReachesChild(t *testing.T) {
	for _, docker := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "docker"}[docker], func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "captured")
			t.Setenv("PATH", dir)
			t.Setenv("NSELF_CI_REF", "inherited-wrong-ref")
			t.Setenv("REF_CAPTURE", output)
			binary := filepath.Join(dir, "gate")
			script := "#!/bin/sh\nprintf '%s' \"$NSELF_CI_REF\" > \"$REF_CAPTURE\"\n"
			if docker {
				binary = filepath.Join(dir, "docker")
				script = "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$REF_CAPTURE\"\n"
			}
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			passed, _, err := runGateInDocker(binary, dir, ServeConfig{JobTimeout: 5, AllowUnsandboxed: true}, "refs/heads/served")
			if err != nil || !passed {
				t.Fatalf("passed %v error %v", passed, err)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			want := "refs/heads/served"
			if docker {
				want = "--env\nNSELF_CI_REF=" + want + "\n"
			}
			if !strings.Contains(string(got), want) {
				t.Fatalf("child got %q want %q", got, want)
			}
		})
	}
}
