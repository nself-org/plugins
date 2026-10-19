package exec

// Purpose: the container runtime's `info` is asked once per window, whoever asks
// (the orphan sweep, hygiene and the isolation probe each used to spawn their
// own `docker info`, about 60 ms each on the job's start path; D-1008).

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func infoStubDocker(t *testing.T) (calls func() int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	stub := "#!/bin/sh\necho \"$*\" >> " + log + "\necho '{\"ServerVersion\":\"x\"}'\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ResetRuntimeInfoCache()
	t.Cleanup(ResetRuntimeInfoCache)
	return func() int {
		b, _ := os.ReadFile(log)
		return strings.Count(string(b), "\n")
	}
}

func TestRuntimeInfoIsProbedOncePerWindow(t *testing.T) {
	calls := infoStubDocker(t)
	ctx := context.Background()
	forms := [][]string{{"info"}, {"info", "--format", "{{json .}}"}, {"info"}}
	var wg sync.WaitGroup
	for _, args := range forms {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := RuntimeQuery(ctx, "docker", args...)
			if err != nil || !strings.Contains(string(out), "ServerVersion") {
				t.Errorf("info %v: %q %v", args, out, err)
			}
		}()
	}
	wg.Wait()
	if _, err := RuntimeQuery(ctx, "docker", "info"); err != nil {
		t.Fatal(err)
	}
	if n := calls(); n != 1 {
		t.Fatalf("docker info ran %d times, want 1", n)
	}
	// ps is not cached: it is a live listing
	if _, err := RuntimeQuery(ctx, "docker", "ps", "-a"); err != nil {
		t.Fatal(err)
	}
	if n := calls(); n != 2 {
		t.Fatalf("docker calls %d, want 2 (one info, one ps)", n)
	}
}

func TestWarmRuntimeInfoLetsTheFirstRealQueryShareTheSpawn(t *testing.T) {
	calls := infoStubDocker(t)
	WarmRuntimeInfo()
	for range 3 {
		if _, err := RuntimeQuery(context.Background(), "docker", "info"); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls(); n != 1 {
		t.Fatalf("docker info ran %d times, want 1", n)
	}
}

// A different daemon (DOCKER_HOST, DOCKER_CONTEXT) is a different question.
func TestRuntimeInfoIsKeyedByTheDaemonTheCLIPicks(t *testing.T) {
	calls := infoStubDocker(t)
	ctx := context.Background()
	t.Setenv("DOCKER_HOST", "unix:///a.sock")
	if _, err := RuntimeQuery(ctx, "docker", "info"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_HOST", "unix:///b.sock")
	if _, err := RuntimeQuery(ctx, "docker", "info"); err != nil {
		t.Fatal(err)
	}
	if n := calls(); n != 2 {
		t.Fatalf("docker info ran %d times for two daemons, want 2", n)
	}
}
