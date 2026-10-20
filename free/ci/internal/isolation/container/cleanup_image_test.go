package container

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// The cleanup helper never runs the job's image: a distroless image has no sh and a hostile one can make its sh do
// nothing, either way leaving job-UID files behind (D-0963).
func TestCleanupRunsInThePinnedHelperImage(t *testing.T) {
	for _, jobImage := range []string{"gcr.io/distroless/static:nonroot", "alpine:3.19", "evil.example/sh-does-nothing:1"} {
		t.Run(jobImage, func(t *testing.T) {
			spec, err := newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootful}}, false).Wrap(exec.JobSpec{Network: "none", Slot: 1})
			if err != nil {
				t.Fatal(err)
			}
			run := exec.ContainerRun{Name: "nself-ci-x", Image: jobImage, Workspace: "/work/x", Workdir: "/repo", AttemptID: "x", CoordinatorID: "c", Slot: 1, CPU: 1, MemoryMB: 256}
			cmd, err := spec.Plan.Cleanup(run)
			if err != nil || cmd == nil {
				t.Fatalf("cleanup: %v %v", cmd, err)
			}
			if cmd.Image != CleanupImage {
				t.Errorf("Image = %q, want the pinned helper %q", cmd.Image, CleanupImage)
			}
			args := strings.Join(cmd.Args, " ")
			if strings.Contains(args, jobImage) || !strings.Contains(args, " "+CleanupImage+" ") {
				t.Errorf("cleanup argv runs the wrong image: %s", args)
			}
		})
	}
}

// The helper constant and images.json (ADR 0030) name the same digest and both platforms, so the daily image probe
// covers exactly what the cleanup starts.
func TestCleanupImageMatchesTheImageLock(t *testing.T) {
	raw, err := os.ReadFile("../../../../../images.json")
	if err != nil {
		t.Fatalf("images.json (plugins root) must be readable: %v", err)
	}
	var lock struct {
		Images map[string]struct {
			Kind      string   `json:"kind"`
			Image     string   `json:"image"`
			Platforms []string `json:"platforms"`
		} `json:"images"`
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	entry, ok := lock.Images["ci-cleanup"]
	if !ok || entry.Kind != "ci" {
		t.Fatalf("no ci-cleanup entry of kind ci: %+v", entry)
	}
	_, want, _ := strings.Cut(CleanupImage, "@")
	_, got, _ := strings.Cut(entry.Image, "@")
	if want == "" || want != got {
		t.Fatalf("digest differs: constant %q lock %q", want, got)
	}
	if strings.Join(entry.Platforms, ",") != "linux/amd64,linux/arm64" {
		t.Errorf("platforms %v", entry.Platforms)
	}
}

// When the pinned helper cannot start, the executor asks for the job's own image.
func TestCleanupCanRunInTheJobImage(t *testing.T) {
	spec, err := newProfile(&fakeRunner{info: map[string]string{"docker": dockerRootful}}, false).Wrap(exec.JobSpec{Network: "none", Slot: 1})
	if err != nil {
		t.Fatal(err)
	}
	run := exec.ContainerRun{Name: "nself-ci-x", Image: "alpine:3.19", Workspace: "/work/x", Workdir: "/repo", AttemptID: "x", CoordinatorID: "c", Slot: 1, CPU: 1, MemoryMB: 256, JobImageCleanup: true}
	cmd, err := spec.Plan.Cleanup(run)
	if err != nil || cmd == nil || cmd.Image != "alpine:3.19" || !strings.Contains(strings.Join(cmd.Args, " "), " alpine:3.19 ") {
		t.Fatalf("%+v %v", cmd, err)
	}
}
