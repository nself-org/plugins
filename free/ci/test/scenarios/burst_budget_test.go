//go:build scenarios

package scenarios

// Purpose: Epic acceptance 5 and 6 at the cloud edge. Without `paid_enabled` plus a day or month budget
// the burst engine never calls the cloud: no machine is created however many jobs queue. The same check
// function runs in TestPlantedProvisionWithoutSpend against an engine fed a policy the coordinator
// does not have.
// Inputs:  a TEAM rig whose coordinator policy forbids spend, the engine's Provision.
// Outputs: assertions only.
// Constraints: injected clock, fakecloud; no sleeps.

import (
	"context"
	"fmt"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

// burstCheckNoProvisionWithoutSpend asks for five provisions and returns a violation for every cloud
// call or machine it finds: the coordinator's own policy allows no spend, so none may exist.
func burstCheckNoProvisionWithoutSpend(r *burstRig) []string {
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		req := r.request(fmt.Sprintf("job-%d", i))
		req.QueuedDemand = 3
		_, _ = r.eng.Provision(ctx, req)
	}
	var out []string
	if n := r.cloud.Calls["Provision"]; n != 0 {
		out = append(out, fmt.Sprintf("%d cloud Provision calls without a spend policy", n))
	}
	if n := len(r.cloud.All()); n != 0 {
		out = append(out, fmt.Sprintf("%d machines exist without a spend policy", n))
	}
	return out
}

func TestBurstNoProvisionWithoutPaidEnabled(t *testing.T) {
	r := burstNew(t, func(r *burstRig) { r.pol.PaidEnabled = false })
	if bad := burstCheckNoProvisionWithoutSpend(r); len(bad) > 0 {
		t.Fatalf("paid_enabled is off: %v", bad)
	}
	res, err := r.eng.Provision(context.Background(), r.request("probe"))
	if err == nil || res.Decision.Allow || burst.ReasonOf(err) != burst.ReasonPaidDisabled {
		t.Fatalf("want the %s refusal, got allow=%v reason=%q err=%v", burst.ReasonPaidDisabled, res.Decision.Allow, res.Decision.Reason, err)
	}
}

func TestBurstNoProvisionWithoutADayOrMonthBudget(t *testing.T) {
	r := burstNew(t, func(r *burstRig) {
		r.pol.PaidEnabled = true
		r.pol.Budgets = schedpolicy.Budgets{Provider: map[string]int64{}} // no day, no month, no job budget
	})
	if bad := burstCheckNoProvisionWithoutSpend(r); len(bad) > 0 {
		t.Fatalf("no budget is set: %v", bad)
	}
}
