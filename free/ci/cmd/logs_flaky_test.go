package main

// D-1050: after `--confirm-flaky` the run keeps the original failure, and the
// check's log_ref names that failed attempt. `ci logs <job>` must read the log
// the evidence names, not the newer passing rerun.

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLogsDefaultsToTheAttemptTheCheckNames(t *testing.T) {
	r := newEvRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	r.commit(".nself/ci.yaml", "version: 1\njobs:\n  unit:\n    kind: test\n    run: [\"sh\", \"-c\", \"if [ -f "+marker+" ]; then echo rerun-passed; exit 0; fi; touch "+marker+"; echo first-failed; exit 1\"]\n")
	if _, errOut, code := r.ci("--no-status", "--confirm-flaky", "1"); code == 0 {
		t.Fatalf("a failed-then-passed job read as a pass (stderr %s)", errOut)
	}
	out, errOut, code := r.ci("logs", "unit")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.TrimSpace(out) != "first-failed" {
		t.Fatalf("ci logs unit printed %q, want the failed attempt named by log_ref (first-failed)", out)
	}
	out, _, code = r.ci("logs", "unit", "--attempt", "2")
	if code != 0 || strings.TrimSpace(out) != "rerun-passed" {
		t.Fatalf("--attempt 2 printed %q (exit %d), want rerun-passed", out, code)
	}
}
