package tiers

// Purpose: the `auto` plan of a hosted + vps + lan + local world (tier, bias and
// the wait in the placement-plan document and the explain lines, kept as a
// golden) and the proof that every other preset's placement is unchanged by
// this package.

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var update = flag.Bool("update", false, "rewrite the auto goldens")

func runner(id, loc, class string) sched.Runner {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
	c.Location.Kind.Value = ptr(loc)
	return sched.Runner{ID: id, Location: loc, Class: class, Provider: "operator", Avail: sched.Avail{Slots: 1, CPU: 8, MemMB: 8192}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: loc == "local", NoLANRoute: true}}
}

func job(key string, queuedMs int64) sched.Job {
	return sched.Job{Key: key, ID: key, Project: "project", Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: "owner", Network: "lan", TimeoutMs: 60000, PersonalOwnRun: true, QueuedMs: queuedMs}
}

// fourTiers is a hosted (not admitted: paid is off), a busy vps, a lan and a local runner.
func fourTiers(t *testing.T, preset string) sched.World {
	t.Helper()
	eff, _, err := registry(t).Merge("")
	if err != nil {
		t.Fatal(err)
	}
	vps := runner("v-vps", "remote", "owned")
	vps.Avail.Slots = 0
	return sched.World{Policy: sched.Policy{Preset: preset, Trust: eff}, Costs: map[string]sched.Cost{}, Previous: map[string]string{},
		Runners: []sched.Runner{runner("l-lan", "lan", "owned"), vps, runner("x-local", "local", "local")},
		Hosted:  []sched.Runner{runner("h-hosted", "hosted", "metered")}}
}

func doc(w sched.World, jobs []sched.Job) model.PlacementPlan {
	return planview.Build(planview.Input{Name: "default", Revision: strings.Repeat("a", 40), Trigger: "local", PolicyDigest: "p" + strings.Repeat("0", 63),
		WorldDigest: "w" + strings.Repeat("0", 63), World: w, Jobs: jobs, Result: sched.Place(w, jobs)})
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := "testdata/" + name
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs (go test -run %s -update):\n%s", path, t.Name(), got)
	}
}

func TestAutoPlanShowsTierAndWait(t *testing.T) {
	w := fourTiers(t, sched.AutoPreset)
	if err := contribute(context.Background(), &w); err != nil {
		t.Fatal(err)
	}
	jobs := []sched.Job{job("fresh", 0), job("older", 20000), job("stale", 30000)}
	p := doc(w, jobs)
	raw, _ := json.MarshalIndent(p, "", "  ")
	schema, err := os.ReadFile("../../schemas/placement-plan.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = model.ValidateJSON(schema, raw); err != nil {
		t.Fatalf("the auto plan must be valid placement-plan v1: %v", err)
	}
	st := map[string]string{}
	for _, j := range p.Jobs {
		st[j.ID] = j.Status
	}
	if st["fresh"] != "waiting" || st["older"] != "waiting" || st["stale"] != "placed" || p.Jobs[2].Placement.RunnerID != "l-lan" {
		t.Fatalf("statuses %v", st)
	}
	text := string(raw)
	for _, want := range []string{`"preset": "auto"`, `"tier_rank"`, "held for v-vps (tier vps, bias 60001, capacity.busy): 30000 ms left", ": 10000 ms left"} {
		if !strings.Contains(text, want) {
			t.Errorf("plan json lacks %q", want)
		}
	}
	lines := strings.Join(planview.ExplainLines(p, planview.SourcePlan), "\n")
	cells := strings.Join(planview.Cells(p), "\n")
	for _, want := range []string{"tier lan  bias 120000", "reason  tier.waiting  held for v-vps", "tier_rank=120000"} {
		if !strings.Contains(lines, want) {
			t.Errorf("explain lacks %q:\n%s", want, lines)
		}
	}
	if !strings.Contains(cells, "waiting: tier.waiting (held for v-vps") {
		t.Errorf("plan cells lack the wait:\n%s", cells)
	}
	golden(t, "auto-plan.golden.json", append(raw, '\n'))
	golden(t, "auto-explain.golden.txt", []byte(lines+"\n"+cells+"\n"))
}

func TestAutoOtherPresetsGolden(t *testing.T) {
	jobs := []sched.Job{job("fresh", 0), job("older", 20000), job("stale", 30000)}
	for preset := range sched.Presets() {
		if preset == sched.AutoPreset {
			continue
		}
		plain := fourTiers(t, preset)
		contributed := fourTiers(t, preset)
		if err := contribute(context.Background(), &contributed); err != nil || contributed.Bias != nil {
			t.Fatalf("%s: the tiers contributor must leave the world alone: %v %v", preset, contributed.Bias, err)
		}
		a, _ := json.Marshal(doc(plain, jobs))
		b, _ := json.Marshal(doc(contributed, jobs))
		if !bytes.Equal(a, b) || bytes.Contains(a, []byte("tier_rank")) || bytes.Contains(a, []byte("tier.waiting")) {
			t.Fatalf("%s: placement changed by the auto preset:\n%s\n%s", preset, a, b)
		}
		lines := strings.Join(planview.ExplainLines(doc(plain, jobs), planview.SourcePlan), "\n")
		if strings.Contains(lines, "  tier ") {
			t.Fatalf("%s: explain shows tier outside auto:\n%s", preset, lines)
		}
	}
}
