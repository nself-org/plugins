package burst_test

// Purpose: a burst intent refused with approval.pending leaves the pipeline estimate for ApproveSpend
// (D-1280): ReserveResource rolls its transaction back, so the engine notes the figure itself.
// Inputs:  the fake cloud rig with a spend policy whose approval threshold the lifetime bound crosses.
// Outputs: assertions only. Constraints: fake provider and a store in a temp dir; no real cloud API.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

func TestBurstApprovalPendingNotesThePipelineEstimate(t *testing.T) {
	r := newRig(t, func(r *rig) {
		r.pol.ApprovalThresholdMicro = schedpolicy.Budget{Micro: 1_000, Set: true}
	})
	if _, err := r.eng.Provision(context.Background(), r.request("job-1")); err == nil {
		t.Fatal("a reservation over the approval threshold must be refused")
	}
	noted, _, err := r.st.PendingEstimate(context.Background(), "pipe-1")
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(105 * 60_000); noted != want {
		t.Fatalf("noted estimate %d, want the lifetime reservation %d", noted, want)
	}
	if ds, derr := r.st.BurstDecisions(context.Background(), "job-1"); derr != nil || len(ds) != 1 || ds[0].Allowed || ds[0].Reason != "reservation.no_fit" || !strings.Contains(ds[0].Input, `"FitReason":"`+string(model.ApprovalPending)+`"`) {
		t.Fatalf("decision log %+v %v", ds, derr)
	}
}
