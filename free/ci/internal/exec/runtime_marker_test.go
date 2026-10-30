//go:build !windows

package exec

// Purpose: the sweep skips a runtime only after one full sweep of the home and only while the home never
// started a container with it (runtime_marker.go, P7-CI-46). Constraints: fake runtime scripts only.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSweepSkipsOnlyUnmarkedRuntimesAfterAFullSweep(t *testing.T) {
	ResetRuntimeInfoCache()
	bin, home := t.TempDir(), t.TempDir()
	t.Setenv("PATH", bin)
	fakeSweepRuntime(t, bin, "docker", "d1 dead", "0")
	fakeSweepRuntime(t, bin, "podman", "p1 dead", "0")
	e := &Executor{Isolation: BaselineIsolator{}, Stale: sweepStale{"dead": true}}
	calls := func(binary string) int {
		data, _ := os.ReadFile(filepath.Join(bin, binary+".calls"))
		n := 0
		for _, c := range data {
			if c == '\n' {
				n++
			}
		}
		return n
	}
	// first sweep of the home: no stamp yet, every runtime is listed (containers of an older binary)
	if err := e.sweepContainers(context.Background(), "now", home); err != nil {
		t.Fatal(err)
	}
	if calls("docker") == 0 || calls("podman") == 0 {
		t.Fatalf("the first sweep skipped a runtime: docker %d podman %d", calls("docker"), calls("podman"))
	}
	if sweepSkips(home, "docker") != true || sweepSkips(home, "podman") != true {
		t.Fatal("after a full sweep an unused runtime is skipped")
	}
	if err := markRuntimeUsed(home, "podman"); err != nil {
		t.Fatal(err)
	}
	d, p := calls("docker"), calls("podman")
	if err := e.sweepContainers(context.Background(), "now", home); err != nil {
		t.Fatal(err)
	}
	if calls("docker") != d || calls("podman") == p {
		t.Fatalf("docker listed %d more times (want 0), podman %d (want more)", calls("docker")-d, calls("podman")-p)
	}
	// a failing sweep never writes the stamp
	other := t.TempDir()
	fakeSweepRuntime(t, bin, "docker", "", "1")
	_ = e.sweepContainers(context.Background(), "now", other)
	if sweepSkips(other, "podman") {
		t.Fatal("a failed sweep let a runtime be skipped")
	}
	if err := markRuntimeUsed(home, "../docker"); err == nil {
		t.Fatal("a runtime name outside the allowlist was marked")
	}
}
