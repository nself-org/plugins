package main

// Purpose: regression tests for D-1002, a SIGKILLed agent must not leave its
// job running past the next agent start.
// Constraints: unix only; the "agent" is this test binary in child mode (see
// TestMain), killed with SIGKILL while its job runs.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// pidAlive reports a running process. A zombie counts as gone: on Linux the executor makes this
// test binary a child subreaper once any earlier test ran a job in-process, so the orphan is
// adopted here and stays a zombie (kill(pid, 0) still succeeds) until reaped.
func pidAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	state := strings.TrimSpace(string(out))
	return err != nil || (state != "" && !strings.HasPrefix(state, "Z"))
}

func waitFor(t *testing.T, what string, limit time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func specFile(t *testing.T, attempt string, run ...string) string {
	t.Helper()
	b, _ := json.Marshal(execSpec{AttemptID: attempt, CoordinatorID: "local", JobID: "fixture", Revision: rev40, Job: model.Job{Kind: "static", Run: run}})
	p := filepath.Join(t.TempDir(), attempt+".json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestNextStartKillsTheJobOfAKilledAgent starts an agent whose job outlives it
// (SIGKILL cannot be caught), then runs another job in the same work area: the
// orphan's process group must be gone.
func TestNextStartKillsTheJobOfAKilledAgent(t *testing.T) {
	skipUnixOnly(t)
	useState(t)
	pidFile := filepath.Join(t.TempDir(), "job.pid")
	spec := specFile(t, "orphan-1", "sh", "-c", "echo $$ > "+pidFile+"; exec sleep 300")
	child := exec.Command(os.Args[0], "exec", "--spec", spec, "--evidence", filepath.Join(t.TempDir(), "ev.json"))
	child.Env = append(os.Environ(), "NSELF_AGENT_TEST_CHILD=1")
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
	waitFor(t, "the job to start", 10*time.Second, func() bool {
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		jobPID, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
		return jobPID > 0
	})
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = child.Process.Wait()
	if !pidAlive(jobPID) {
		t.Fatal("fixture: the job died with its agent; nothing to sweep")
	}
	code, _, stderr := runCmd(t, "exec", "--spec", specFile(t, "next-1", "true"), "--evidence", filepath.Join(t.TempDir(), "ev2.json"))
	if code != 0 {
		t.Fatalf("next run exited %d: %s", code, stderr)
	}
	waitFor(t, "the orphaned job to be killed", 5*time.Second, func() bool { return !pidAlive(jobPID) })
}
