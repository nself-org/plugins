package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/where"
)

func withKnown(t *testing.T, k where.Known, val string) {
	t.Helper()
	oldK, oldV := whereKnown, whereValue
	whereKnown = func(context.Context, bool) where.Known { return k }
	whereValue = val
	t.Cleanup(func() { whereKnown, whereValue = oldK, oldV })
}

func TestBareRunDefaultsToAuto(t *testing.T) {
	// nothing registered: today's local behaviour, nothing chosen, no values
	withKnown(t, where.Known{}, "")
	var o RunOptions
	if err := whereOptions(&o); err != nil || whereChosen.Kind != "" || o.PolicyRun != nil {
		t.Fatalf("no nodes: %+v %v %v", whereChosen, o.PolicyRun, err)
	}
	// a registered node: auto, and the operator's sched.preset stands (no value written)
	withKnown(t, where.Known{Nodes: []string{"ms5"}}, "")
	o = RunOptions{}
	if err := whereOptions(&o); err != nil || whereChosen.Kind != where.KindAuto || len(o.PolicyRun) != 0 {
		t.Fatalf("one node: %+v %v %v", whereChosen, o.PolicyRun, err)
	}
	// a provider alone also counts
	withKnown(t, where.Known{Providers: []string{"github"}}, "")
	o = RunOptions{}
	if err := whereOptions(&o); err != nil || whereChosen.Kind != where.KindAuto {
		t.Fatalf("one provider: %+v %v", whereChosen, err)
	}
}

func TestWhereFlagMergesUnderExplicitFlags(t *testing.T) {
	withKnown(t, where.Known{Nodes: []string{"ms5", "lan1"}}, "ms5")
	runner, _ := json.Marshal("lan1")
	o := RunOptions{PolicyRun: map[string]json.RawMessage{"sched.overrides.runner": runner}}
	if err := whereOptions(&o); err != nil {
		t.Fatal(err)
	}
	if got := string(o.PolicyRun["sched.overrides.runner"]); got != `"lan1"` {
		t.Fatalf("an explicit --runner must win, got %s", got)
	}
	withKnown(t, where.Known{Nodes: []string{"ms5"}}, "ms5")
	o = RunOptions{}
	if err := whereOptions(&o); err != nil || string(o.PolicyRun["sched.overrides.runner"]) != `"ms5"` {
		t.Fatalf("--where ms5: %v %v", o.PolicyRun, err)
	}
}

func TestWhereUnknownValueIsE797(t *testing.T) {
	withKnown(t, where.Known{}, "mars")
	var o RunOptions
	err := whereOptions(&o)
	if err == nil || !strings.HasPrefix(err.Error(), "E797") {
		t.Fatalf("got %v", err)
	}
	if exitForError(err) != 2 {
		t.Error("E797 must exit 2")
	}
	withKnown(t, where.Known{}, "all")
	if err = whereOptions(&o); err != nil || whereChosen.Kind != where.KindAll { // internal/race is linked (P7-WIRE-07)
		t.Fatalf("all: %v", err)
	}
}

func TestWithPlacementAddsDataPlacement(t *testing.T) {
	type doc struct {
		Schema string `json:"schema"`
	}
	if got := withPlacement(doc{"ev"}, nil); got != (doc{"ev"}) {
		t.Fatalf("no placement must leave the document alone: %v", got)
	}
	got := withPlacement(doc{"ev"}, map[string]any{"tier": "owned", "bias": "balanced"})
	raw, _ := json.Marshal(got)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || string(m["schema"]) != `"ev"` {
		t.Fatalf("evidence fields lost: %s %v", raw, err)
	}
	if !strings.Contains(string(m["placement"]), `"tier":"owned"`) || !strings.Contains(string(m["placement"]), `"bias"`) {
		t.Fatalf("placement missing: %s", raw)
	}
}

// wire08Placement drives the real entry (runV15, argv) on the planview fixture
// repo and returns data.placement of the --json envelope.
func wire08Placement(t *testing.T, args ...string) map[string]any {
	t.Helper()
	repo, _ := placementRepo(t)
	t.Setenv("NSELF_CI_REPO", repo)
	resetPlanFlags()
	oldV := whereValue
	whereValue = ""
	t.Cleanup(func() { whereValue = oldV })
	code, out, errs := capture(t, func() int { return runV15(append(args, "--json", "--no-status")) })
	var env struct {
		Data struct {
			Placement map[string]any `json:"placement"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &env); err != nil || env.Data.Placement == nil {
		t.Fatalf("exit %d, no data.placement: %v\n%s\n%s", code, err, out, errs)
	}
	return env.Data.Placement
}

func TestWire08ExplicitAutoSetsPreset(t *testing.T) {
	pl := wire08Placement(t, "--where", "auto")
	pipe, _ := pl["pipeline"].(map[string]any)
	if pipe["preset"] != "auto" {
		t.Fatalf("--where auto must set sched.preset=auto, got %v", pipe["preset"])
	}
	jobs, _ := pl["jobs"].([]any)
	found := false
	for _, j := range jobs {
		m, _ := j.(map[string]any)
		if p, _ := m["placement"].(map[string]any); p["tier"] != nil && p["tier"] != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no tier in data.placement jobs: %v", pl["jobs"])
	}
}

func TestWire08BareDefaultUnchanged(t *testing.T) {
	withKnown(t, where.Known{Nodes: []string{"ms5"}}, "")
	var o RunOptions
	if err := whereOptions(&o); err != nil || whereChosen.Kind != where.KindAuto || len(o.PolicyRun) != 0 {
		t.Fatalf("bare default with a node must choose auto and write no value: %+v %v %v", whereChosen, o.PolicyRun, err)
	}
	if _, set := o.PolicyRun["sched.preset"]; set {
		t.Fatal("the bare default must not set sched.preset")
	}
}
