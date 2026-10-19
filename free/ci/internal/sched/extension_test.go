package sched

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func altOf(t *testing.T, r Result, runner string) model.Alternative {
	t.Helper()
	for _, a := range r.Explanations[0].Alternatives {
		if a.RunnerID == runner {
			return a
		}
	}
	t.Fatalf("no alternative for %s in %+v", runner, r.Explanations[0])
	return model.Alternative{}
}

func TestExtensionPointsNoopGoldens(t *testing.T) {
	base := fixtureWorld(fixtureRunner("a", "lan"), fixtureRunner("b", "lan"))
	want, _ := json.Marshal(Place(base, []Job{fixtureJob("job")}))
	for name, w := range map[string]World{
		"empty maps": func() World {
			w := base
			w.Exclusions = map[string][]model.PlacementReasonDetail{}
			w.Bias = map[string]int64{}
			return w
		}(),
		"zero bias":  func() World { w := base; w.Bias = map[string]int64{"a": 0, "b": 0}; return w }(),
		"empty list": func() World { w := base; w.Exclusions = map[string][]model.PlacementReasonDetail{"a": nil}; return w }(),
	} {
		got, _ := json.Marshal(Place(w, []Job{fixtureJob("job")}))
		if string(got) != string(want) {
			t.Errorf("%s changed the placement:\n got %s\nwant %s", name, got, want)
		}
	}
	for _, a := range Place(base, []Job{fixtureJob("job")}).Explanations[0].Alternatives {
		if _, ok := a.Components["tier_rank"]; ok || len(a.Components) != 9 {
			t.Errorf("%s: components without Bias must stay the nine existing ones: %v", a.RunnerID, a.Components)
		}
	}
	// the world digest input must not grow keys when nothing is registered
	raw, _ := json.Marshal(base)
	j, _ := json.Marshal(fixtureJob("job"))
	for _, key := range []string{`"Exclusions"`, `"Bias"`, `"QueuedMs"`, `"Record"`} {
		if strings.Contains(string(raw), key) || strings.Contains(string(j), key) {
			t.Errorf("%s appears in the JSON of an unextended world or job", key)
		}
	}
}

func TestExclusionsAppended(t *testing.T) {
	w := fixtureWorld(fixtureRunner("a", "lan"), fixtureRunner("b", "lan"))
	w.Exclusions = map[string][]model.PlacementReasonDetail{"a": {{Code: string(model.PoolExcluded), Detail: "not_in_project_list"}}}
	got := Place(w, []Job{fixtureJob("job")})
	a := altOf(t, got, "a")
	if a.Eligible || !containsCode(a.Reasons, "pool.excluded") {
		t.Fatalf("runner a must be ineligible with pool.excluded: %+v", a)
	}
	if !altOf(t, got, "b").Eligible || got.Placements[0].RunnerID != "b" {
		t.Fatalf("runner b must take the job: %+v", got.Placements)
	}
	// the exclusion comes after the shipped reasons, which stay in their order
	w.Runners[0].Avail.Slots = 0
	reasons := altOf(t, Place(w, []Job{fixtureJob("job")}), "a").Reasons
	if n := len(reasons); n < 2 || reasons[n-1].Code != "pool.excluded" || reasons[0].Code != "capacity.busy" {
		t.Fatalf("exclusions must be appended last: %+v", reasons)
	}
	// pool.excluded never clears without configuration; node.not_ready and tier.waiting do
	only := fixtureWorld(fixtureRunner("a", "lan"))
	for code, status := range map[model.PlacementReason]string{model.PoolExcluded: "unplaceable", model.NodeNotReady: "waiting", model.TierWaiting: "waiting"} {
		only.Exclusions = map[string][]model.PlacementReasonDetail{"a": {{Code: string(code), Detail: "x"}}}
		if s := Place(only, []Job{fixtureJob("job")}).Explanations[0].Status; s != status {
			t.Errorf("%s: status %s, want %s", code, s, status)
		}
	}
	if !model.PoolExcluded.Static() || model.NodeNotReady.Static() || model.TierWaiting.Static() {
		t.Error("Static() must be true for pool.excluded only")
	}
	for _, c := range []model.PlacementReason{model.PoolExcluded, model.NodeNotReady, model.TierWaiting} {
		if !c.Valid() {
			t.Errorf("%s is not in the closed set", c)
		}
	}
}

func TestBiasComponent(t *testing.T) {
	w := fixtureWorld(fixtureRunner("a", "lan"), fixtureRunner("b", "lan"))
	plain := Place(w, []Job{fixtureJob("job")})
	w.Bias = map[string]int64{"a": 9000}
	got := Place(w, []Job{fixtureJob("job")})
	a, b := altOf(t, got, "a"), altOf(t, got, "b")
	c, ok := a.Components["tier_rank"]
	if !ok || c.ValueMs != 9000 {
		t.Fatalf("tier_rank component missing or wrong: %+v", a.Components)
	}
	if _, ok := b.Components["tier_rank"]; ok {
		t.Fatal("runner b has no bias and must show no tier_rank component")
	}
	if *a.Score != *altOf(t, plain, "a").Score+9000 || *b.Score != *altOf(t, plain, "b").Score {
		t.Fatalf("score must move by the bias only: a=%d b=%d", *a.Score, *b.Score)
	}
	if got.Placements[0].RunnerID != "b" {
		t.Fatalf("the unbiased runner must win: %+v", got.Placements)
	}
}
