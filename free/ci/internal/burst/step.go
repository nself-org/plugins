package burst

// Purpose: the `ephemeral` admission step (Epic D12): an ephemeral node takes
// exactly one job, in every trust class. Inside the admission transaction the
// step claims the node for the attempt; a second lease request finds the claim
// and is refused with ephemeral.used. It also binds a runner that declares a
// burst intent label to the node the intent was enrolled with, because the
// budget step lets such a runner spend the intent's reservation.
// Inputs:  the dispatch.View of one admission.
// Outputs: nil (admit) or a *dispatch.Refusal.
// Constraints: the claim row is written in the admission transaction, so it
// commits with the lease or not at all (a refusal by a later step leaves no
// claim); the step reads and writes only through the AdmitView (the admission
// holds the store's writer); an ephemeral runner with no intent (a local
// one-job runner) is limited the same way; a node being torn down takes nothing.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// StepName is the registered name; steps run in name order.
const StepName = "ephemeral"

// Step is the admission step; see the file comment.
func Step(ctx context.Context, v dispatch.View) error {
	r := v.Request.Runner
	b := v.AdmitView.Burst(ctx)
	in, bound, err := b.IntentOfNode(r.ID)
	if err != nil {
		return err
	}
	// Every intent label must name the enrolled intent: the budget step reads the first one it finds,
	// so a runner that lists its own intent after another's must not pass.
	for _, l := range r.Capability.Identity.Labels {
		if tok, ok := strings.CutPrefix(l, economics.IntentLabelPrefix); ok && (!bound || in.Token != tok) {
			return &dispatch.Refusal{Reason: model.LabelsMismatch, Detail: "runner " + r.ID + " names a burst intent it is not enrolled with"}
		}
	}
	if !r.Ephemeral && !bound {
		return nil
	}
	if r.Used {
		return &dispatch.Refusal{Reason: model.EphemeralUsed, Detail: "ephemeral runner " + r.ID + " already took its job"}
	}
	if bound && in.State != store.BurstLive {
		return &dispatch.Refusal{Reason: model.EphemeralUsed, Detail: "ephemeral runner " + r.ID + " is being torn down"}
	}
	first, err := b.NodeUse(r.ID, in.Token, v.Demand.AttemptID, b.NowMs())
	if err != nil {
		return err
	}
	if !first {
		return &dispatch.Refusal{Reason: model.EphemeralUsed, Detail: "ephemeral runner " + r.ID + " already held a lease"}
	}
	return nil
}
