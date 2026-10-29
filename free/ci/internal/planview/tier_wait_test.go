package planview

// Purpose: under the `auto` preset `ci plan` and `ci explain` show the chosen
// runner's tier and bias and, for a held job, the tier.waiting detail with the
// wait left (P7-AUTO-05); every other preset's lines stay as they were.
// Inputs: a two-runner world (busy better tier, free lower tier) and the
// fixture's linux jobs. Outputs: pass or fail.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// tierWorld puts a busy runner in the better tier and a free one a tier lower;
// the tier index comes from Bias, the way internal/tiers writes it.
func tierWorld(t *testing.T, preset string) sched.World {
	t.Helper()
	w, _, _ := fixture(t)
	busy, free := w.Runners[0], w.Runners[0]
	busy.ID, busy.Avail.Slots = "busy-local", 0
	busy.Capability.Identity.ID = busy.ID
	free.ID = "free-local"
	free.Capability.Identity.ID = free.ID
	w.Runners = []sched.Runner{busy, free}
	w.Policy.Preset = preset
	w.Bias = map[string]int64{"busy-local": 0, "free-local": sched.TierStepMs}
	return w
}

func tierDoc(w sched.World, jobs []sched.Job) model.PlacementPlan {
	return Build(Input{Name: "default", Revision: strings.Repeat("a", 40), Trigger: "local", PolicyDigest: "p" + strings.Repeat("0", 63),
		WorldDigest: "w" + strings.Repeat("0", 63), World: w, Jobs: jobs, Result: sched.Place(w, jobs)})
}

func linuxBuild(t *testing.T, queuedMs int64) []sched.Job {
	t.Helper()
	_, jobs, _ := fixture(t)
	for _, j := range jobs {
		if j.Name == "build" {
			j.QueuedMs = queuedMs
			return []sched.Job{j}
		}
	}
	t.Fatal("fixture has no build job")
	return nil
}

func TestPlanShowsTierAndWait(t *testing.T) {
	// A fresh job waits for the busy better tier: the cell names tier.waiting and the wait left.
	held := tierDoc(tierWorld(t, sched.AutoPreset), linuxBuild(t, 0))
	cells := Cells(held)
	if len(cells) != 1 || !strings.Contains(cells[0], string(model.TierWaiting)) || !strings.Contains(cells[0], "ms left of step_sec") {
		t.Fatalf("a held job's cell must show tier.waiting with the wait left: %q", cells)
	}
	if !strings.Contains(cells[0], "busy-local") {
		t.Fatalf("the cell must name the held better runner: %q", cells[0])
	}

	// Past step_sec it runs on the lower tier, and the chosen line shows its tier and bias.
	ran := tierDoc(tierWorld(t, sched.AutoPreset), linuxBuild(t, sched.DefaultAutoStepSec*1000+1))
	lines := strings.Join(ExplainLines(ran, SourcePlan), "\n")
	if !strings.Contains(lines, "chosen  free-local") {
		t.Fatalf("the job must run on the lower tier after the wait:\n%s", lines)
	}
	if !strings.Contains(lines, "  tier local  bias ") {
		t.Fatalf("the auto chosen line must show tier and bias:\n%s", lines)
	}

	// Another preset with the same world prints no tier text.
	plain := strings.Join(ExplainLines(tierDoc(tierWorld(t, "balanced"), linuxBuild(t, 0)), SourcePlan), "\n")
	if strings.Contains(plain, "  tier ") || strings.Contains(plain, string(model.TierWaiting)) {
		t.Fatalf("a non-auto preset must not print tier text:\n%s", plain)
	}
}
