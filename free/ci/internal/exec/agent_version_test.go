package exec

// Purpose: tests of RunAgentVersion (D-1385) against real child processes: a binary that answers, one that exits non-zero,
//          one that never ends, one that prints without end, and a path that does not exist.
// Inputs:  small shell scripts written to a temp dir (Unix only).
// Outputs: pass or fail.
// Constraints: no network; each case ends in well under the production time box.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func script(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	p := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunAgentVersionReturnsWhatTheBinaryPrints(t *testing.T) {
	out, err := RunAgentVersion(context.Background(), script(t, `[ "$1" = version ] && [ $# -eq 1 ] && echo "nself-ci-agent 1.5.0 linux/amd64"`))
	if err != nil || strings.TrimSpace(out) != "nself-ci-agent 1.5.0 linux/amd64" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestRunAgentVersionDoesNotPassTheCallersSecrets(t *testing.T) {
	t.Setenv("NSELF_TEST_SECRET", "s3cret")
	out, err := RunAgentVersion(context.Background(), script(t, `echo "[${NSELF_TEST_SECRET}]"`))
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatalf("out %q err %v", out, err)
	}
}

func TestRunAgentVersionRefusesABinaryThatFails(t *testing.T) {
	for name, p := range map[string]string{
		"exit 3":  script(t, `echo broken; exit 3`),
		"missing": filepath.Join(t.TempDir(), "absent"),
	} {
		_, err := RunAgentVersion(context.Background(), p)
		var e *Error
		if !errors.As(err, &e) || e.Code != "E610" {
			t.Fatalf("%s: err %v, want E610", name, err)
		}
	}
	if _, err := RunAgentVersion(context.Background(), "relative/agent"); err == nil {
		t.Fatal("a relative path was run")
	}
}

func TestRunAgentVersionBoundsOutputAndTime(t *testing.T) {
	start := time.Now()
	_, err := RunAgentVersion(context.Background(), script(t, `yes nself-ci-agent`))
	if err == nil || !strings.Contains(err.Error(), "more than") || time.Since(start) > 5*time.Second {
		t.Fatalf("endless output: err %v after %v", err, time.Since(start))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start = time.Now()
	_, err = RunAgentVersion(ctx, script(t, `exec sleep 30`))
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("hung binary: err %v after %v", err, time.Since(start))
	}
}
