//go:build docker

package exec

import (
	"context"
	"encoding/json"
	"os"
	osexec "os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func dockerPrerequisite(t *testing.T) string {
	t.Helper()
	if out, err := osexec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("Docker required: %v: %s", err, out)
	}
	image := os.Getenv("CI32_TEST_IMAGE")
	if image == "" {
		image = "alpine:3.19"
	}
	if out, err := osexec.Command("docker", "image", "inspect", image).CombinedOutput(); err != nil {
		if pulled, pullErr := osexec.Command("docker", "pull", image).CombinedOutput(); pullErr != nil {
			t.Fatalf("image %s unavailable: %v: %s; pull: %v: %s", image, err, out, pullErr, pulled)
		}
	}
	return image
}

func inspect(t *testing.T, name string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out, err := osexec.Command("docker", "inspect", name).Output()
		if err == nil {
			var records []map[string]any
			if json.Unmarshal(out, &records) == nil && len(records) == 1 {
				return records[0]
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("container %s not found", name)
	return nil
}

func TestContainerBaselineFlags(t *testing.T) {
	image := dockerPrerequisite(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &Executor{Grace: 100 * time.Millisecond}
	done := make(chan AttemptResult, 1)
	go func() {
		done <- e.Run(ctx, JobSpec{AttemptID: "baseline-flags", CoordinatorID: "test-coord", Home: t.TempDir(), Job: model.Job{Isolation: "container", Container: &model.ContainerConfig{Image: image}}, Command: []string{"sleep", "30"}, CPU: 1.5, MemoryMB: 256})
	}()
	defer osexec.Command("docker", "rm", "-f", "nself-ci-baseline-flags").Run()
	record := inspect(t, "nself-ci-baseline-flags")
	host := record["HostConfig"].(map[string]any)
	if host["ReadonlyRootfs"] != true || host["Privileged"] != false || host["NetworkMode"] != "none" {
		t.Fatalf("unsafe host config: %+v", host)
	}
	if host["NanoCpus"].(float64) <= 0 || host["Memory"].(float64) <= 0 {
		t.Fatalf("missing limits: %+v", host)
	}
	for _, mount := range record["Mounts"].([]any) {
		m := mount.(map[string]any)
		if strings.Contains(m["Source"].(string), "docker.sock") {
			t.Fatalf("socket mount: %+v", m)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("container cancellation hung")
	}
}

type staleMap map[string]bool

func (m staleMap) Stale(_ context.Context, id string) (bool, error) { return m[id], nil }

func TestContainerOrphanSweep(t *testing.T) {
	image := dockerPrerequisite(t)
	for _, id := range []string{"live-ci32", "stale-ci32"} {
		name := "nself-ci-" + id
		defer osexec.Command("docker", "rm", "-f", name).Run()
		out, err := osexec.Command("docker", "run", "-d", "--name", name, "--label", "nself.ci.attempt="+id, "--label", "nself.ci.coordinator="+id, image, "sleep", "30").CombinedOutput()
		if err != nil {
			t.Fatalf("docker run %s: %v: %s", id, err, out)
		}
	}
	e := &Executor{Stale: staleMap{"live-ci32": false, "stale-ci32": true}}
	if err := e.Sweep(context.Background(), "current-ci32"); err != nil {
		t.Fatal(err)
	}
	inspect(t, "nself-ci-live-ci32")
	if err := osexec.Command("docker", "inspect", "nself-ci-stale-ci32").Run(); err == nil {
		t.Fatal("stale container survived")
	}
}
