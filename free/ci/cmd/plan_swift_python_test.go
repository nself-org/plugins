package main

// Purpose: `nself ci plan` of the Swift/Xcode and Python fixtures (P7-CI-64):
// the Xcode job carries platform darwin/arm64 and requires.tools [xcode], and
// a host that cannot run it gets unplaceable.* with the missing item named.
// Constraints: fixtures come from internal/detect/testdata; the world is
// built by hand, so the test does not depend on the host's tools.

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func fixturePlan(t *testing.T, group, name string, w plan.World) *plan.Plan {
	t.Helper()
	root := filepath.Join("..", "internal", "detect", "testdata", group, name)
	cfg := model.PipelineConfig{Version: 1}
	facts, checks, err := planChecks(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, err := plan.Build(plan.Input{Config: cfg, Facts: facts, Checks: checks, World: w, Revision: plan.Revision{DefaultBranch: true}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func jobNamed(t *testing.T, p *plan.Plan, id string) plan.PlannedJob {
	t.Helper()
	for _, j := range p.Jobs {
		if j.ID == id || strings.HasPrefix(j.ID, id+"@") {
			return j
		}
	}
	t.Fatalf("no job %s in %+v", id, p.Jobs)
	return plan.PlannedJob{}
}

func TestPlanXcodeFixture(t *testing.T) {
	mac := plan.World{Platform: "darwin/arm64", Tools: []string{"xcode", "swift", "gitleaks"}, CPU: 8, MemMB: 16384}
	j := jobNamed(t, fixturePlan(t, "swift", "xcode-app", mac), "xcode-test.App")
	if j.State != plan.StatePlanned || j.Platform != "darwin/arm64" {
		t.Fatalf("xcode-test on a Mac: %+v", j)
	}
	if j.Job.Requirements == nil || !reflect.DeepEqual(j.Job.Requirements.Tools, []string{"xcode"}) {
		t.Fatalf("requires.tools: %+v", j.Job.Requirements)
	}
	if j.Job.Required == nil || !*j.Job.Required {
		t.Fatalf("xcode-test must be required: %+v", j.Job.Required)
	}
	if !strings.Contains(strings.Join(j.Job.Run, " "), "-scheme App") {
		t.Fatalf("argv: %v", j.Job.Run)
	}
}

func TestPlanXcodeWithoutXcodeIsUnplaceableTool(t *testing.T) {
	bare := plan.World{Platform: "darwin/arm64", Tools: []string{"swift"}, CPU: 8, MemMB: 16384}
	j := jobNamed(t, fixturePlan(t, "swift", "xcode-app", bare), "xcode-test.App")
	if j.State != plan.StateUnplaceable || j.Blocked != "unplaceable.tool" || j.Detail != "tool xcode is not on PATH" {
		t.Fatalf("a Mac without Xcode: %+v", j)
	}
}

func TestPlanXcodeOnLinuxIsUnplaceablePlatform(t *testing.T) {
	linux := plan.World{Platform: "linux/amd64", Tools: []string{"xcode"}, CPU: 8, MemMB: 16384}
	j := jobNamed(t, fixturePlan(t, "swift", "xcode-app", linux), "xcode-test.App")
	if j.State != plan.StateUnplaceable || j.Blocked != "unplaceable.platform" {
		t.Fatalf("Linux host: %+v", j)
	}
}

func TestPlanSwiftPMNeedsSwift(t *testing.T) {
	without := fixturePlan(t, "swift", "swiftpm", plan.World{Platform: "linux/amd64", CPU: 4, MemMB: 8192})
	if j := jobNamed(t, without, "swift-test"); j.State != plan.StateUnplaceable || j.Blocked != "unplaceable.tool" || !strings.Contains(j.Detail, "swift") {
		t.Fatalf("no swift on PATH must wait with unplaceable.tool: %+v", j)
	}
	with := fixturePlan(t, "swift", "swiftpm", plan.World{Platform: "linux/amd64", Tools: []string{"swift"}, CPU: 4, MemMB: 8192})
	if j := jobNamed(t, with, "swift-build"); j.State != plan.StatePlanned {
		t.Fatalf("swift-build with swift on PATH: %+v", j)
	}
}

func TestPlanPythonUV(t *testing.T) {
	w := plan.World{Platform: "linux/amd64", Tools: []string{"uv", "ruff"}, CPU: 4, MemMB: 8192}
	p := fixturePlan(t, "python", "uv", w)
	if j := jobNamed(t, p, "python-test"); j.State != plan.StatePlanned || j.Job.Required == nil || !*j.Job.Required {
		t.Fatalf("python-test: %+v", j)
	}
	bare := fixturePlan(t, "python", "uv", plan.World{Platform: "linux/amd64", CPU: 4, MemMB: 8192})
	if j := jobNamed(t, bare, "python-test"); j.State != plan.StateUnplaceable || j.Blocked != "unplaceable.tool" || !strings.Contains(j.Detail, "uv") {
		t.Fatalf("no uv: %+v", j)
	}
	for _, j := range bare.Jobs {
		if j.ID == "python-lint" {
			t.Fatalf("advisory python-lint without ruff must be skipped, not planned: %+v", j)
		}
	}
}
