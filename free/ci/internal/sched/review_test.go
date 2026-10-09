package sched

import (
	"math"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestReviewUnknownResourceFailsClosed(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Capability.Resources.CPU.Value = nil
	r.Capability.Resources.MemMB.Value = nil
	got := Place(fixtureWorld(r), []Job{fixtureJob("j")})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.ResourcesInsufficient)) {
		t.Fatalf("unknown resources admitted: %+v", got)
	}
}

func TestReviewRequirementsIsolationMinimum(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Decl.Isolation = "container"
	j := fixtureJob("j")
	j.Requirements.IsolationMin = "ephemeral-vm"
	got := Place(fixtureWorld(r), []Job{j})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, "isolation.below_minimum") {
		t.Fatalf("isolation floor ignored: %+v", got)
	}
}

func TestReviewRetryIsolationMinimum(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Decl.Isolation = "container"
	w := fixtureWorld(r)
	w.PreviousIsolation = map[string]model.Isolation{"j": "ephemeral-vm"}
	got := Place(w, []Job{fixtureJob("j")})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, "isolation.below_minimum") {
		t.Fatalf("retry isolation floor ignored: %+v", got)
	}
}

func TestReviewUnknownCostClassFailsClosed(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Class = ""
	got := Place(fixtureWorld(r), []Job{fixtureJob("j")})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.PaidDisabled)) || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.CostUnknownRate)) {
		t.Fatalf("unknown class admitted: %+v", got)
	}
}

func TestReviewIncludedUnknownRateFailsClosed(t *testing.T) {
	r := fixtureRunner("r", "lan")
	r.Class, r.Provider = "included", "hosted"
	w := fixtureWorld(r)
	w.Allowances = map[string]Allowance{"hosted": {Known: true, CeilingKnown: true, RemainingEstimated: 1}}
	got := Place(w, []Job{fixtureJob("j")})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.AllowanceBelowMargin)) {
		t.Fatalf("unknown rate beat allowance: %+v", got)
	}
}

func TestReviewFastPathPreservesRemoteForLocalRefusal(t *testing.T) {
	local, remote := fixtureRunner("a", "local"), fixtureRunner("b", "lan")
	local.Decl.Accepts = []model.TrustClass{"owner"}
	j := fixtureJob("j")
	j.Trust = "collaborator"
	got := Place(fixtureWorld(local, remote), []Job{fixtureJob("local"), j})
	if !got.FastPath.Taken || len(got.Placements) != 2 || got.Placements[1].RunnerID != "b" {
		t.Fatalf("remote candidate discarded: %+v", got)
	}
}

func TestReviewTransferOverflowCannotImproveScore(t *testing.T) {
	big, small := fixtureRunner("a", "lan"), fixtureRunner("b", "lan")
	w := fixtureWorld(big, small)
	w.Estimates = map[string]map[string]Sample{
		"a": {"j": {CheckoutBytes: ptr(int64(math.MaxInt64))}},
		"b": {"j": {CheckoutBytes: ptr(int64(1000000))}},
	}
	got := Place(w, []Job{fixtureJob("j")})
	if len(got.Placements) != 1 || got.Placements[0].RunnerID != "b" {
		t.Fatalf("overflow improved score: %+v", got)
	}
	w.Cache = map[string]map[string]CacheFact{"a": {"j": {MissBytes: ptr(int64(math.MaxInt64)), Confidence: "known"}}}
	if value, _ := cacheValue(w, fixtureJob("j"), big); value < 0 {
		t.Fatalf("cache overflow: %d", value)
	}
}

func TestReviewRunnerLimitPerRunner(t *testing.T) {
	a, b := fixtureRunner("a", "lan"), fixtureRunner("b", "lan")
	w := fixtureWorld(a, b)
	w.Policy.Limits.Runner = 1
	w.Policy.Limits.Provider = 1
	b.Provider = "other"
	w.Runners[1] = b
	got := Place(w, []Job{fixtureJob("first"), fixtureJob("second")})
	if len(got.Placements) != 2 || got.Placements[0].RunnerID == got.Placements[1].RunnerID {
		t.Fatalf("limits shared across runners/providers: %+v", got)
	}
}

func TestReviewFastPathUsesSlots(t *testing.T) {
	local, remote := fixtureRunner("a", "local"), fixtureRunner("b", "lan")
	local.Avail.Slots, local.Avail.CPU = 2, 8
	j1, j2 := fixtureJob("one"), fixtureJob("two")
	w := fixtureWorld(local, remote)
	w.Estimates = map[string]map[string]Sample{
		"a": {"one": {RuntimeMs: ptr(int64(60000)), Confidence: "known"}, "two": {RuntimeMs: ptr(int64(60000)), Confidence: "known"}},
		"b": {"one": {RuntimeMs: ptr(int64(40000)), Confidence: "known"}, "two": {RuntimeMs: ptr(int64(40000)), Confidence: "known"}},
	}
	got := Place(w, []Job{j1, j2})
	if !got.FastPath.Taken || len(got.Placements) != 2 || got.Placements[0].RunnerID != "a" || got.Placements[1].RunnerID != "a" {
		t.Fatalf("parallel local slots ignored: %+v", got)
	}
}

func TestReviewFastPathHonorsDependencies(t *testing.T) {
	local, remote := fixtureRunner("a", "local"), fixtureRunner("b", "lan")
	local.Avail.Slots = 2
	first, second := fixtureJob("one"), fixtureJob("two")
	second.Deps = []string{"one"}
	w := fixtureWorld(local, remote)
	w.Estimates = map[string]map[string]Sample{
		"a": {"one": {RuntimeMs: ptr(int64(60000)), Confidence: "known"}, "two": {RuntimeMs: ptr(int64(60000)), Confidence: "known"}},
		"b": {"one": {RuntimeMs: ptr(int64(40000)), Confidence: "known"}, "two": {RuntimeMs: ptr(int64(40000)), Confidence: "known"}},
	}
	got := Place(w, []Job{first, second})
	if got.FastPath.Taken {
		t.Fatalf("dependency omitted from makespan: %+v", got.FastPath)
	}
}

func TestReviewFastPathSchedulesRemoteOnlyDependency(t *testing.T) {
	local, remote := fixtureRunner("a", "local"), fixtureRunner("b", "lan")
	first, second := fixtureJob("one"), fixtureJob("two")
	first.RemoteOnly = true
	second.Deps = []string{"one"}
	w := fixtureWorld(local, remote)
	w.Estimates = map[string]map[string]Sample{
		"a": {"two": {RuntimeMs: ptr(int64(60000)), Confidence: "known"}},
		"b": {"one": {RuntimeMs: ptr(int64(40000)), Confidence: "known"}, "two": {RuntimeMs: ptr(int64(40000)), Confidence: "known"}},
	}
	end, known := planMakespan(w, []Job{first, second}, []Runner{local, remote}, true)
	if !known || end <= 60000 {
		t.Fatalf("remote-only predecessor omitted from local plan: end=%d known=%v", end, known)
	}
}
