//go:build unix

package run

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

// finalizeRuntimeScripts installs empty runtime listings with separate invocation and completion
// records. Inputs: the test and listing delay. Outputs: the record directory. Constraints: PATH
// contains only the fakes; an absolute sleep path avoids dependence on the host runtime installation.
func finalizeRuntimeScripts(t *testing.T, delay string) string {
	t.Helper()
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, binary := range []string{"docker", "podman"} {
		script := fmt.Sprintf(`#!/bin/sh
FINALIZE_CALLS_DIR='%s'
case "$1" in ps|network|volume) ;; *) exit 42 ;; esac
printf '%%s\n' "$1" >> "$FINALIZE_CALLS_DIR/%s.calls"
'%s' %s
: > "$FINALIZE_CALLS_DIR/%s.$1.done"
`, strings.ReplaceAll(dir, "'", "'\"'\"'"), binary, strings.ReplaceAll(sleep, "'", "'\"'\"'"), delay, binary)
		if err := os.WriteFile(filepath.Join(dir, binary), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

// checkFinalizeListings verifies all six listings completed exactly once before Finalize returned.
// Inputs: the test and record directory. Outputs: test failures. Constraints: per-binary call order
// is deliberately ignored because concurrent completion order is not part of the runtime contract.
func checkFinalizeListings(t *testing.T, dir string) {
	t.Helper()
	for _, binary := range []string{"docker", "podman"} {
		data, err := os.ReadFile(filepath.Join(dir, binary+".calls"))
		if err != nil {
			t.Fatal(err)
		}
		calls := strings.Fields(string(data))
		if len(calls) != 3 {
			t.Fatalf("%s: got calls %q, want exactly three listings", binary, calls)
		}
		for _, kind := range []string{"ps", "network", "volume"} {
			count := 0
			for _, call := range calls {
				if call == kind {
					count++
				}
			}
			if count != 1 {
				t.Errorf("%s %s: got %d calls, want one", binary, kind, count)
			}
			if _, err := os.Stat(filepath.Join(dir, binary+"."+kind+".done")); err != nil {
				t.Errorf("Finalize returned before %s %s completed: %v", binary, kind, err)
			}
		}
	}
}

func TestHygieneFinalizeOverlapsRuntimeListingsAndJoinsThem(t *testing.T) {
	dir := finalizeRuntimeScripts(t, "0.2")
	home := t.TempDir()
	h := &hygieneHook{
		byRun: map[string][]seenAttempt{"parallel": {{id: "a1", home: home}}},
		rt:    &hygiene.ExecRuntime{},
	}
	start := time.Now()
	err := h.Finalize(context.Background(), RunInfo{RunID: "parallel"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	checkFinalizeListings(t, dir)
	if elapsed >= 400*time.Millisecond {
		t.Fatalf("six 200 ms listings took %s, want less than 400 ms", elapsed)
	}
}

func TestHygieneFinalizeCachesListingsAcrossAttempts(t *testing.T) {
	dir := finalizeRuntimeScripts(t, "0")
	home := t.TempDir()
	h := &hygieneHook{
		byRun: map[string][]seenAttempt{"cached": {
			{id: "a1", home: home}, {id: "a2", home: home},
		}},
		rt: &hygiene.ExecRuntime{},
	}
	if err := h.Finalize(context.Background(), RunInfo{RunID: "cached"}); err != nil {
		t.Fatal(err)
	}
	checkFinalizeListings(t, dir)
}
