package demand_test

import (
	"slices"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func apiFact[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "api", ObservedAt: epoch, Confidence: "known"}
}

// demandFacts is a runner demand as the poll builds it: API-read run facts, per-repo auth.
func demandFacts(event, headRepo string, fork, protectedOnly bool) model.TriggerFacts {
	f := model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: "github", Kind: "runner_demand", DeliveryID: "poll:job:1", ReceivedAt: epoch.Add(time.Minute),
		Auth: "per-repo", Repo: testRepo, Revision: sha, RevisionSource: "payload"}
	f.RunnerLabels = apiFact([]string{"self-hosted", "nself"})
	f.RunEvent, f.HeadRepo, f.Fork = apiFact(event), apiFact(headRepo), apiFact(fork)
	f.GroupProtectedOnly = apiFact(protectedOnly)
	f.ActorPermission, f.SHAReachable, f.ProtectedRef, f.HeadCurrent = apiFact("admin"), apiFact(true), apiFact(true), apiFact(true)
	return f
}

// TestDemandTrust proves acceptance line 6 (review F7) through TRUST-13's Classify: a runner demand of
// event pull_request_target or workflow_run, or from a fork head repository, is untrusted even when the
// runner group takes protected refs only; a push on the base repository classifies from the run facts only
// when the group is protected-only; a repo-scope runner (never a protected group) is always untrusted.
func TestDemandTrust(t *testing.T) {
	const reason = source.ReasonRunnerDemand
	cases := []struct {
		name          string
		f             model.TriggerFacts
		untrusted     bool
		wantRunReason bool
	}{
		{"pull_request_target, protected-only group", demandFacts("pull_request_target", testRepo, false, true), true, true},
		{"workflow_run, protected-only group", demandFacts("workflow_run", testRepo, false, true), true, true},
		{"issue_comment, protected-only group", demandFacts("issue_comment", testRepo, false, true), true, true},
		{"fork head repo, protected-only group", demandFacts("push", "stranger/widgets", true, true), true, true},
		{"fork flag alone", demandFacts("push", testRepo, true, true), true, false},
		{"push on the base repo, repo scope (no protected group)", demandFacts("push", testRepo, false, false), true, true},
		{"pull_request event, protected-only group", demandFacts("pull_request", testRepo, false, true), true, true},
		{"push on the base repo, protected-only group", demandFacts("push", testRepo, false, true), false, false},
		{"schedule on the base repo, protected-only group", demandFacts("schedule", testRepo, false, true), false, false},
	}
	for _, tc := range cases {
		r := source.Classify(tc.f)
		isUntrusted := r.Class == "untrusted"
		hasReason := slices.Contains(r.Reasons, reason)
		if isUntrusted != tc.untrusted || (tc.wantRunReason && !hasReason) || (!tc.untrusted && hasReason) {
			t.Errorf("%s: class %s reasons %v", tc.name, r.Class, r.Reasons)
		}
	}
	missing := demandFacts("push", testRepo, false, true)
	missing.GroupProtectedOnly = model.Fact[bool]{}
	if r := source.Classify(missing); r.Class != "untrusted" {
		t.Errorf("an unknown group restriction must be untrusted, got %s", r.Class)
	}
}
