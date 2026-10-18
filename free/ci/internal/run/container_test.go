//go:build container

package run

// Container-tagged tests: they need a Docker daemon and fail (never skip)
// when it is missing. Jobs run in the alpine:3.19 image that the host
// already has (no pull).

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func needDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatalf("docker is required for the container tests: %v", err)
	}
	if out, err := exec.Command("docker", "image", "inspect", "alpine:3.19", "--format", "{{.Id}}").CombinedOutput(); err != nil {
		t.Fatalf("image alpine:3.19 is required: %v %s", err, out)
	}
}

type alpine struct{}

func (alpine) Image(context.Context, AttemptInfo) (string, error) { return "alpine:3.19", nil }

// TestFirstRun: a fresh repository with no remote runs its gates, says why no
// status was posted and exits on the gate result, in a container too.
func TestFirstRun(t *testing.T) {
	needDocker(t)
	for _, container := range []bool{false, true} {
		root := t.TempDir()
		for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=a@b", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"}} {
			if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
		r := newRig(t, map[string]model.Job{"gate": sh("true"), "second": sh("exit 4", "gate")}, gitFake{})
		r.C.Capture, r.C.Reporter = nil, nil
		r.C.Opts.Root, r.C.Opts.Container = root, container
		r.C.Hooks.Image = alpine{}
		res := r.run(t)
		if res.Exit != 1 || !strings.Contains(r.Out.String(), "nself-ci: status not posted: no GitHub remote") {
			t.Fatalf("container=%v exit %d\n%s", container, res.Exit, r.Out)
		}
		if outcome(res, "gate").State != StatePassed || outcome(res, "second").ExitCode != 4 {
			t.Fatalf("container=%v jobs %+v", container, res.Summary.Jobs)
		}
	}
}

// TestTwoCPU: on a 2-CPU host a foreground run finishes every required job
// (one slot, jobs queue behind each other).
func TestTwoCPU(t *testing.T) {
	needDocker(t)
	r := newRig(t, map[string]model.Job{"a": sh("true"), "b": sh("true"), "c": sh("true"), "d": sh("true", "a", "b")}, gitFake{})
	r.C.Capacity = func() ciexec.Capacity {
		return ciexec.Capacity{CPUs: 2, MemoryMB: 2048, FreeMemoryMB: 900, BatteryPercent: -1, PluggedIn: true, Interactive: true}
	}
	r.C.Opts.Container, r.C.Hooks.Image = true, alpine{}
	res := r.run(t)
	if res.Exit != 0 || len(res.Summary.Jobs) != 4 {
		t.Fatalf("exit %d\n%s", res.Exit, r.Out)
	}
	if d := ciexec.Admit(model.Job{}, ciexec.Foreground, r.C.Capacity(), ciexec.AdmissionPolicy{}); !d.Allowed || d.Slots != 1 {
		t.Fatalf("2-CPU decision %+v", d)
	}
}

// TestBattery: at 20% battery off power a deep job is refused in the
// background but a foreground run still completes every required job.
func TestBattery(t *testing.T) {
	needDocker(t)
	low := func() ciexec.Capacity {
		return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: 20, PluggedIn: false, Interactive: true}
	}
	deep := sh("true")
	deep.Path = "deep"
	if d := ciexec.Admit(deep, ciexec.Background, low(), ciexec.AdmissionPolicy{}); d.Allowed || d.Reason != "admission.battery" {
		t.Fatalf("background decision %+v", d)
	}
	r := newRig(t, map[string]model.Job{"deep": deep, "plain": sh("true")}, gitFake{})
	r.C.Capacity = low
	res := r.run(t)
	if res.Exit != 0 || outcome(res, "deep").State != StatePassed {
		t.Fatalf("exit %d\n%s", res.Exit, r.Out)
	}
}
