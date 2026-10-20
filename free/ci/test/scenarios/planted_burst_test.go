//go:build scenarios && planted

package scenarios

// Purpose: planted regressions for the BURST scenarios (P7-SCHED-15). Each test runs the SAME check
// function as the real scenario against a seam with a defect injected in test code, and requires the
// check to report a violation. A check that stays silent here would not catch the real regression.
// Inputs:  burstCheckSweepSpares and burstCheckOneProvisionPerWindow from the scenario files.
// Outputs: t.Fatal "planted regression not detected" when a check misses its defect.
// Constraints: the defects live in wrappers around the provider seam; no production code is edited.

import (
	"context"
	"fmt"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/burst/fakecloud"
	"github.com/nself-org/plugins/free/ci/internal/economics"
)

// burstSloppyProvider is a provider whose Teardown deletes every other machine in the account too (a
// teardown that matches too widely: by name prefix, by region, by a dropped label filter).
type burstSloppyProvider struct {
	burst.Provider
	cloud *fakecloud.Cloud
}

func (p *burstSloppyProvider) Teardown(ctx context.Context, id string) error {
	err := p.Provider.Teardown(ctx, id)
	for _, r := range p.cloud.All() {
		if r.ID != id {
			_ = p.cloud.Teardown(ctx, r.ID)
		}
	}
	return err
}

func TestPlantedSweeperDeletesForeign(t *testing.T) {
	r := burstNew(t, func(r *burstRig) {
		r.wrap = func(p burst.Provider) burst.Provider { return &burstSloppyProvider{Provider: p, cloud: r.cloud} }
	})
	bad := burstCheckSweepSpares(r)
	if len(bad) == 0 {
		t.Fatal("planted regression not detected: a teardown that also deleted foreign, unlabelled and excluded machines passed the check")
	}
	t.Logf("detected: %v", bad)
}

func TestPlantedProvisioningStorm(t *testing.T) {
	r := burstNew(t)
	n := 0
	// a seam that provisions on every step regardless of the decision and its window
	storm := func(ctx context.Context, req burst.Request) error {
		n++
		token := fmt.Sprintf("01J00000000000000000STORM%03d", n)
		_, err := r.cloud.Provision(ctx, burst.Intent{Token: token, Spec: req.Spec, Labels: burst.IntentLabels(burstCoord, token)})
		return err
	}
	bad := burstCheckOneProvisionPerWindow(r, storm)
	if len(bad) == 0 {
		t.Fatal("planted regression not detected: a provision on every step passed the one-per-window check")
	}
	t.Logf("detected: %v", bad)
}

// TestPlantedProvisionWithoutSpend: the engine is fed a policy with paid spend and a budget while the
// coordinator really has none (the guard reads the wrong policy); the cloud then sees a provision.
func TestPlantedProvisionWithoutSpend(t *testing.T) {
	r := burstNew(t, func(r *burstRig) {
		r.pol.PaidEnabled = false
		r.spend = economics.StaticSpend(burstPolicy())
	})
	bad := burstCheckNoProvisionWithoutSpend(r)
	if len(bad) == 0 {
		t.Fatal("planted regression not detected: a provision without paid_enabled and a budget passed the check")
	}
	t.Logf("detected: %v", bad)
}
