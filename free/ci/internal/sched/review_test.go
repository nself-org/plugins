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

func TestReviewIncludedAllowanceAcrossJobs(t *testing.T) {
	a, b := fixtureRunner("a", "lan"), fixtureRunner("b", "lan")
	a.Class, b.Class = "included", "included"
	a.Provider, b.Provider = "shared", "shared"
	w := fixtureWorld(a, b)
	w.Costs = map[string]Cost{
		"a": {Class: "included", RateMicro: 1, BillingIncrementMs: 60000},
		"b": {Class: "included", RateMicro: 1, BillingIncrementMs: 60000},
	}
	w.Allowances = map[string]Allowance{"shared": {Known: true, CeilingKnown: true, RemainingEstimated: 100000}}
	got := Place(w, []Job{fixtureJob("first"), fixtureJob("second")})
	if len(got.Placements) != 1 || !containsCode(got.Explanations[1].Alternatives[1].Reasons, string(model.AllowanceBelowMargin)) {
		t.Fatalf("allowance overcommitted: %+v", got)
	}
	if w.Allowances["shared"].RemainingEstimated != 100000 {
		t.Fatal("Place mutated caller allowance")
	}
}

func TestReviewPaidBudgetAcrossJobs(t *testing.T) {
	a, b := fixtureRunner("a", "lan"), fixtureRunner("b", "lan")
	a.Class, b.Class = "metered", "metered"
	w := fixtureWorld(a, b)
	w.Policy.PaidEnabled = true
	w.Costs = map[string]Cost{
		"a": {Class: "metered", RateMicro: 1, BillingIncrementMs: 60000},
		"b": {Class: "metered", RateMicro: 1, BillingIncrementMs: 60000},
	}
	w.Budgets.Day = Budget{Set: true, Remaining: 100000}
	got := Place(w, []Job{fixtureJob("first"), fixtureJob("second")})
	if len(got.Placements) != 1 || !containsCode(got.Explanations[1].Alternatives[1].Reasons, string(model.BudgetExceeded)) {
		t.Fatalf("budget overcommitted: %+v", got)
	}
	if w.Budgets.Day.Remaining != 100000 {
		t.Fatal("Place mutated caller budget")
	}
}

func TestReviewApplyPersistsLimits(t *testing.T) {
	cases := []struct {
		name  string
		code  model.PlacementReason
		limit func(*Limits)
	}{
		{"pipeline", model.LimitPipeline, func(l *Limits) { l.Pipeline = 1 }},
		{"job", model.LimitJob, func(l *Limits) { l.Job = 1 }},
		{"runner", model.LimitRunner, func(l *Limits) { l.Runner = 1 }},
		{"provider", model.LimitProvider, func(l *Limits) { l.Provider = 1 }},
		{"project", model.LimitProject, func(l *Limits) { l.Project = 1 }},
		{"user", model.LimitUser, func(l *Limits) { l.User = 1 }},
		{"team", model.LimitTeam, func(l *Limits) { l.Team = 1 }},
		{"global", model.LimitGlobal, func(l *Limits) { l.Global = 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := fixtureRunner("r", "lan")
			r.Avail.Slots = 2
			w := fixtureWorld(r)
			tc.limit(&w.Policy.Limits)
			w.Apply(Placement{RunnerID: "r", CPU: 1, MemMB: 128})
			got := Place(w, []Job{fixtureJob("second")})
			if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(tc.code)) {
				t.Fatalf("Apply lost %s count: %+v", tc.name, got)
			}
		})
	}
}

func TestReviewBackgroundUnknownActivity(t *testing.T) {
	for _, tc := range []struct {
		name        string
		plug        *bool
		interactive *string
		code        model.PlacementReason
	}{
		{"unplugged", ptr(false), ptr("idle"), model.NodeOnBattery},
		{"unknown-plug", nil, ptr("idle"), model.NodeOnBattery},
		{"active", ptr(true), ptr("active"), model.NodeInteractive},
		{"unknown-interactive", ptr(true), nil, model.NodeInteractive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := fixtureRunner("r", "local")
			r.Capability.Availability.PluggedIn.Value = tc.plug
			r.Capability.Availability.Interactive.Value = tc.interactive
			j := fixtureJob("j")
			j.Pipeline.Band = "background"
			got := Place(fixtureWorld(r), []Job{j})
			if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(tc.code)) {
				t.Fatalf("unknown operator activity admitted: %+v", got)
			}
		})
	}
}

func TestReviewBackgroundOwnershipSources(t *testing.T) {
	job := fixtureJob("j")
	job.Pipeline.Band = "background"
	operator := fixtureRunner("r", "lan")
	operator.Decl.Ownership = ""
	operator.Capability.Identity.Ownership.Value = ptr("operator")
	got := Place(fixtureWorld(operator), []Job{job})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.NodeOnBattery)) || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.NodeInteractive)) {
		t.Fatalf("capability operator ownership ignored: %+v", got)
	}
	operator.Capability.Availability.PluggedIn.Value = ptr(true)
	operator.Capability.Availability.Interactive.Value = ptr("idle")
	got = Place(fixtureWorld(operator), []Job{job})
	if len(got.Placements) != 1 {
		t.Fatalf("idle plugged operator refused: %+v", got)
	}

	shared := fixtureRunner("r", "lan")
	shared.Decl.Ownership = "shared"
	shared.Capability.Identity.Ownership.Value = ptr("shared")
	shared.Capability.Availability.PluggedIn.Value = ptr(true)
	shared.Capability.Availability.Interactive.Value = ptr("active")
	got = Place(fixtureWorld(shared), []Job{job})
	if len(got.Placements) != 0 || !containsCode(got.Explanations[0].Alternatives[0].Reasons, string(model.NodeInteractive)) {
		t.Fatalf("known active use admitted: %+v", got)
	}
	shared.Capability.Availability.Interactive.Value = ptr("idle")
	got = Place(fixtureWorld(shared), []Job{job})
	if len(got.Placements) != 1 {
		t.Fatalf("idle shared runner refused: %+v", got)
	}
}
