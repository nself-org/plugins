//go:build integration

package sim

// Purpose: TestPlatformSteering (P7-NODE-17): an explicit platform on a job's matrix entry steers a bare foreground
//          run to a node of that platform, so one host can use amd64 and arm64 nodes in one run; a platform no online
//          node has is refused, and `ci explain` says why. A job without a platform keeps the invoking host's platform.
// Inputs:  INTEGRATION=1. No Docker: in-process node sessions behind the real dispatch, scheduler and store.
// Outputs: none.
// Constraints: a refused job never runs on a node of another architecture.

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/run"
)

// startConfig begins a run of cfg against the given runner platforms and returns its result channel.
func (f *fixture) startConfig(ctx context.Context, cfg model.PipelineConfig, runners []string) (<-chan run.Result, *bytes.Buffer) {
	f.t.Helper()
	pl, err := plan.Build(plan.Input{Config: cfg, World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384, Runners: runners, Remote: true}})
	if err != nil {
		f.t.Fatal(err)
	}
	out := &bytes.Buffer{}
	c := &run.Coordinator{Opts: run.Options{Root: f.repo, Home: f.home, NoStatus: true}, Plan: pl, ConfigDigest: "cfg", Out: out, Capture: f.git, Reporter: noReport{},
		Capacity: func() ciexec.Capacity {
			return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
		}, Poll: 10 * time.Millisecond}
	ch := make(chan run.Result, 1)
	go func() {
		res, err := c.Run(ctx)
		if err != nil {
			f.t.Errorf("Run: %v\n%s", err, out)
		}
		ch <- res
	}()
	return ch, out
}

func matrixConfig(platforms ...any) model.PipelineConfig {
	j := remoteJob(1)
	j.Matrix = &model.JobMatrix{Platform: platforms}
	sup := make([]string, 0, len(platforms))
	for _, p := range platforms {
		sup = append(sup, p.(string))
	}
	return model.PipelineConfig{Version: 1, Support: sup, Jobs: map[string]model.Job{"unit": j}}
}

func TestPlatformSteering(t *testing.T) {
	pass := passRunner("steered job")
	f := newFixtureWith(t, []nodeSpec{
		{id: "n-s390x", host: "10.0.1.1", shape: shapePlatform("linux", "s390x")},
		{id: "n-ppc64le", host: "10.0.1.2", shape: shapePlatform("linux", "ppc64le")},
	}, map[string]func(context.Context, *node, session.JobRun) (session.JobResult, error){"n-s390x": pass, "n-ppc64le": pass})
	amd, arm := f.nodes[0], f.nodes[1] // s390x and ppc64le: platforms no CI host has, so the host default never matches them
	ctx := context.Background()

	t.Run("a matrix of two platforms uses a node of each from one host", func(t *testing.T) {
		ch, out := f.startConfig(ctx, matrixConfig("linux/s390x", "linux/ppc64le"), []string{"linux/s390x", "linux/ppc64le"})
		res := wait(t, ch, "the run")
		if res.Exit != 0 || res.Summary.Result != "pass" {
			t.Fatalf("exit %d result %s\n%s", res.Exit, res.Summary.Result, out)
		}
		if len(amd.ran()) != 1 || len(arm.ran()) != 1 {
			t.Fatalf("s390x node ran %v, ppc64le node ran %v; each matrix entry must run once on the node of its platform", amd.ran(), arm.ran())
		}
		for entry, other := range map[string]string{"unit@linux/s390x": "n-ppc64le", "unit@linux/ppc64le": "n-s390x"} {
			lines := explainJob(t, f, res.RunID, entry)
			if !strings.Contains(lines, other) || !strings.Contains(lines, string(model.PlatformMismatch)) {
				t.Errorf("explain %s does not list %s with %s:\n%s", entry, other, model.PlatformMismatch, lines)
			}
		}
	})

	t.Run("a platform no online node has is refused with a reason and runs nowhere", func(t *testing.T) {
		before := len(amd.ran()) + len(arm.ran())
		ch, out := f.startConfig(ctx, matrixConfig("linux/riscv64"), []string{"linux/riscv64"})
		res := wait(t, ch, "the run")
		if res.Exit == 0 || res.Summary.Result == "pass" {
			t.Fatalf("a job for linux/riscv64 passed with no such node: exit %d\n%s", res.Exit, out)
		}
		if after := len(amd.ran()) + len(arm.ran()); after != before {
			t.Fatalf("the riscv64 job ran on a node of another architecture (%d -> %d)", before, after)
		}
		lines := explainJob(t, f, res.RunID, "unit@linux/riscv64")
		t.Logf("run output:\n%s\nci explain:\n%s", out, lines)
		if !strings.Contains(lines, string(model.PlatformMismatch)) || !strings.Contains(lines, "linux/riscv64") {
			t.Errorf("explain does not give the platform reason:\n%s", lines)
		}
	})

	t.Run("a job without a platform keeps the host platform", func(t *testing.T) {
		host := runtime.GOOS + "/" + runtime.GOARCH
		// two nodes of other platforms are online; the job names none, so it asks for the host platform and no node has it
		ch, out := f.startConfig(ctx, model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"unit": remoteJob(1)}}, nil)
		res := wait(t, ch, "the run")
		if res.Exit == 0 {
			t.Fatalf("a job with no platform ran on a node that is not %s\n%s", host, out)
		}
		if len(amd.ran()) != 1 || len(arm.ran()) != 1 {
			t.Fatalf("a node of another platform took the job: s390x %v ppc64le %v", amd.ran(), arm.ran())
		}
	})
}
