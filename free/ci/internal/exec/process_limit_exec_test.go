//go:build !windows

package exec

// Purpose: pin the guarantee every exec-funnel child keeps: its CPU limit is in
// force before the job's first instruction, and a limit that cannot be set stops
// the job instead of letting it run unlimited.
// Constraints: these tests go through limitedCommand, so a change of the launch
// mechanism (shell wrapper, re-exec trampoline) has to keep them green.

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A job that spins is ended by the kernel inside its CPU budget, with no help from the executor's
// own timeout (the context here has no deadline), and the signal is named in the result.
func TestLimitedChildThatSpinsIsKilledByItsCPULimit(t *testing.T) {
	argv := limitedCommand(JobSpec{Timeout: time.Second, Command: []string{"sh", "-c", "while :; do :; done"}})
	started := time.Now()
	code, sig, err := (&Executor{Redactor: NewOutputRedactor()}).runCommandSig(context.Background(), nil, argv, t.TempDir(), nil, nil, nil, nil, "")
	took := time.Since(started)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code == 0 || (sig != "SIGKILL" && sig != "signal 24") {
		t.Fatalf("spinning child was not ended by its CPU limit: code=%d signal=%q", code, sig)
	}
	if took > 6*time.Second {
		t.Fatalf("CPU limit of 1s took %v to fire", took)
	}
}

// The limit is set before the job runs: a job that reads its own limit sees it, and a limit the
// host refuses (here a hard limit already lowered by an outer limit below the request) exits 125 without running the job.
func TestLimitedChildSeesItsLimitAndRefusalStopsTheJob(t *testing.T) {
	var out []byte
	sink := func(p []byte) { out = append(out, p...) }
	argv := limitedCommand(JobSpec{Timeout: 90 * time.Second, Command: []string{"sh", "-c", "ulimit -t"}})
	code, _, err := (&Executor{Redactor: NewOutputRedactor()}).runCommandSig(context.Background(), nil, argv, t.TempDir(), nil, sink, nil, nil, "")
	if err != nil || code != 0 || string(out) != "90\n" {
		t.Fatalf("limit not visible to the job: code=%d err=%v out=%q", code, err, out)
	}
	out = nil
	lowered := append([]string{"sh", "-c", "ulimit -t 5 || exit 99; exec \"$@\"", "lower"},
		limitedCommand(JobSpec{Timeout: 600 * time.Second, Command: []string{"sh", "-c", "echo job-ran"}})...)
	code, _, err = (&Executor{Redactor: NewOutputRedactor()}).runCommandSig(context.Background(), nil, lowered, t.TempDir(), nil, sink, nil, nil, "")
	if err != nil || code != 125 || strings.Contains(string(out), "job-ran") {
		t.Fatalf("refused limit must stop the job: code=%d err=%v out=%q", code, err, out)
	}
}
