//go:build !windows

package run

// D-1008: a recovery must not make the run wait for the container daemon. The
// orphan sweep asks `docker info` and `docker ps`; behind a slow daemon that was
// the longest step before the first job could start.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// dockerShim puts a docker on PATH that logs each call and answers `info` after delay.
func dockerShim(t *testing.T, delay string) (calls func() []string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$1\" >> " + log + "\nif [ \"$1\" = info ]; then sleep " + delay + "; echo '{}'; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ciexec.ResetRuntimeInfoCache()
	t.Cleanup(ciexec.ResetRuntimeInfoCache)
	return func() []string {
		data, _ := os.ReadFile(log)
		return strings.Fields(string(data))
	}
}

func recoveryStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, home
}

func TestRecoverBesideDoesNotWaitForTheDaemon(t *testing.T) {
	calls := dockerShim(t, "0.6")
	st, home := recoveryStore(t)
	start := time.Now()
	rep, join := RecoverBeside(context.Background(), st, "coord-a", home)
	if took := time.Since(start); took > 400*time.Millisecond {
		t.Fatalf("RecoverBeside waited %v for the daemon", took)
	}
	if len(rep.Errors) != 0 {
		t.Fatalf("recovery errors: %v", rep.Errors)
	}
	if err := join(); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if took := time.Since(start); took < 500*time.Millisecond {
		t.Fatalf("join returned after %v, before the slow daemon answered", took)
	}
	if got := calls(); len(got) == 0 || got[0] != "info" {
		t.Fatalf("docker calls %v: the sweep never asked the daemon", got)
	}
}

// The container sweep of a recovery stands for the first job's sweep in the same
// window: one `docker ps` per coordinator, not two.
func TestRecoverSweepStandsForTheJobStartSweep(t *testing.T) {
	calls := dockerShim(t, "0")
	st, home := recoveryStore(t)
	rep := Recover(context.Background(), st, "coord-window", home)
	if len(rep.Errors) != 0 {
		t.Fatalf("recovery errors: %v", rep.Errors)
	}
	exec := ciexec.Executor{Stale: staleChecker{st, bootID()}}
	if err := exec.SweepContainers(context.Background(), safeID("coord-window")); err != nil {
		t.Fatal(err)
	}
	ps := 0
	for _, c := range calls() {
		if c == "ps" {
			ps++
		}
	}
	if ps != 1 {
		t.Fatalf("docker ps ran %d times in one window, want 1 (calls %v)", ps, calls())
	}
}
