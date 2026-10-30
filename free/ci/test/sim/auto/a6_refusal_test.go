package auto

// Purpose: A6 a node that is busy (interactive use) or short of memory refuses the job, and the
// explanation a `status --json` consumer reads names the runner, the reason code and eligibility.
// Inputs: sched.Place over a world of one busy, one low-memory and one idle runner. Outputs: pass or fail.
// Constraints: pure; no clock.

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/background"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func runnerOf(id string, memMB int64, interactive string) sched.Runner {
	loc, os, arch := "lan", "linux", "amd64"
	cpu, mem := float64(8), memMB
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects, c.Lifecycle.Eligible = []string{"p"}, true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = &os, &arch
	c.Resources.CPU.Value, c.Resources.MemMB.Value = &cpu, &mem
	c.Location.Kind.Value = &loc
	if interactive != "" {
		c.Availability.Interactive.Value = &interactive
	}
	return sched.Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: 2, CPU: 16, MemMB: memMB}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", NoLANRoute: true}}
}

func TestConformanceA6Refusal(t *testing.T) {
	w := sched.World{Runners: []sched.Runner{runnerOf("idle", 16384, "idle"), runnerOf("low-mem", 64, "idle")},
		Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}
	j := sched.Job{Key: "job", ID: "job", Project: "p", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 2048}, Trust: "owner", Network: "lan", TimeoutMs: 60000}
	res := sched.Place(w, []sched.Job{j})
	if len(res.Placements) != 1 || res.Placements[0].RunnerID != "idle" {
		t.Fatalf("placements = %+v", res.Placements)
	}
	raw, err := json.Marshal(res.Explanations)
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Alternatives []struct {
			RunnerID string `json:"runner_id"`
			Eligible bool   `json:"eligible"`
			Reasons  []struct {
				Code string `json:"code"`
			} `json:"reasons"`
		} `json:"alternatives"`
	}
	if err = json.Unmarshal(raw, &rows); err != nil || len(rows) != 1 {
		t.Fatalf("explanation json: %v %s", err, raw)
	}
	seen := map[string]bool{}
	for _, a := range rows[0].Alternatives {
		if a.RunnerID == "low-mem" {
			seen["low"] = !a.Eligible && len(a.Reasons) > 0
		}
		if a.RunnerID == "idle" {
			seen["idle"] = a.Eligible && len(a.Reasons) == 0
		}
	}
	if !seen["low"] || !seen["idle"] {
		t.Fatalf("status fields = %s", raw)
	}
	// a node in interactive use refuses background work, naming the cause
	v := background.Contended(runnerOf("busy", 16384, "active"))
	if !v.Busy() || !slices.Contains(v.Causes, background.InteractiveActive) {
		t.Fatalf("verdict = %+v", v)
	}
}
