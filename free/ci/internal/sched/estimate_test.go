package sched

// D-1280 regression: a job the scheduler holds with approval.pending never reaches the budget
// step, so the explanation itself must carry the worst-case cost the approval has to cover.

import "testing"

func TestAwaitingApprovalExplanationCarriesTheEstimate(t *testing.T) {
	r := fixtureRunner("cloud", "lan")
	r.Class = "metered"
	j := fixtureJob("j")
	w := fixtureWorld(r)
	w.Costs[r.ID] = Cost{RateMicro: 5}
	w.Policy.PaidEnabled, w.Policy.ApprovalThresholdMicro = true, 1
	w.Budgets.Day = Budget{Remaining: 1 << 40, Set: true}
	want, _ := worstCost(j.TimeoutMs, w.Costs[r.ID])
	got := Place(w, []Job{j})
	if len(got.Explanations) != 1 || got.Explanations[0].Status != "awaiting-approval" {
		t.Fatalf("the job was not held for approval: %+v", got)
	}
	if e := got.Explanations[0].EstimateMicro; e != want || want <= 0 {
		t.Fatalf("estimate = %d, want the worst case %d", e, want)
	}
}
