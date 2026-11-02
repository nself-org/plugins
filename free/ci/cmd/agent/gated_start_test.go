package main

// Purpose: regression tests for D-1531, a job must not run its payload unless
// the process marker the orphan sweep reads exists. An agent SIGKILLed between
// starting the job and writing the marker used to leave a running job nobody
// could find.
// Constraints: unix only; the "agent" is this test binary in child mode (see
// TestMain), parked by ciexec.BeforeMarkerHook and then killed.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestKilledAgentBeforeMarkerNeverRunsTheJob kills the agent in the window between the job's start and its
// marker write: the gated job must exit without running its payload, with no marker to sweep.
func TestKilledAgentBeforeMarkerNeverRunsTheJob(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	ran := filepath.Join(t.TempDir(), "payload.ran")
	paused := filepath.Join(t.TempDir(), "paused.pid")
	spec := specFile(t, "gated-1", "sh", "-c", "echo ran > "+ran+"; exec sleep 300")
	child := exec.Command(os.Args[0], "exec", "--spec", spec, "--evidence", filepath.Join(t.TempDir(), "ev.json"))
	child.Env = append(os.Environ(), "NSELF_AGENT_TEST_CHILD=1", "NSELF_AGENT_TEST_PAUSE="+paused)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	var jobPID int
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
		if jobPID > 0 {
			_ = syscall.Kill(-jobPID, syscall.SIGKILL)
		}
	})
	waitFor(t, "the agent to start the job and park before the marker", 10*time.Second, func() bool {
		raw, err := os.ReadFile(paused)
		if err != nil {
			return false
		}
		jobPID, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
		return jobPID > 0
	})
	if matches, _ := filepath.Glob(filepath.Join(os.Getenv("NSELF_CI_AGENT_WORK"), "exec-home", "processes", "*.json")); len(matches) != 0 {
		t.Fatalf("fixture: a marker exists before the hook returned: %v", matches)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = child.Process.Wait()
	// End of file on the gate, not a sweep, ends the job: nothing has swept anything yet.
	waitFor(t, "the gated job to exit once its agent died", 5*time.Second, func() bool { return !pidAlive(jobPID) })
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("the payload ran although the agent died before writing the marker")
	}
}

// TestGatedJobRunsOnceTheMarkerIsWritten is the positive control: the same spec, not killed, runs its payload.
func TestGatedJobRunsOnceTheMarkerIsWritten(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	ran := filepath.Join(t.TempDir(), "payload.ran")
	code, _, stderr := runCmd(t, "exec", "--spec", specFile(t, "gated-2", "sh", "-c", "echo ran > "+ran), "--evidence", filepath.Join(t.TempDir(), "ev.json"))
	if code != 0 {
		t.Fatalf("exec exited %d: %s", code, stderr)
	}
	if _, err := os.Stat(ran); err != nil {
		t.Fatalf("the payload did not run: %v", err)
	}
}
