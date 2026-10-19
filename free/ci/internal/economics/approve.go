package economics

// Purpose: spend approval (Epic D11, X4). A pipeline whose estimate crosses
// economics.approval_threshold_micro stays awaiting-approval (approval.pending)
// until an admin approves the spend; `nself ci runs approve <run> --spend`
// (P7-TRUST-24) calls ApproveSpend.
// Inputs:  the run id and the acting roles.Actor.
// Outputs: the recorded Approval; an audit row naming actor, amount, run, head
// SHA and config digest.
// Constraints: authorization is roles.AuthorizeAction(spend.approve) for the
// run's project (admin; the PERSONAL operator holds it; a member or release
// user is refused with E687); the approval is bound to the run, its head SHA,
// its config digest and the estimate noted for it, so a new push or a changed
// config voids it with no write, and a job costing more than the approved
// estimate is held again; the estimate is the largest figure noted for the run
// (NotePending, or the approval.pending refusals the budget step leaves); with
// none, there is nothing to approve (E695).

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type projectKey struct{}

// WithProject names the project id the approver's roles are checked against. The
// pipeline row holds the repository root, which is not the id roles grants use; the
// command that knows the id passes it here. Without it an actor must hold the role on every project.
func WithProject(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, projectKey{}, id)
}

// Approval is an approved spend.
type Approval struct {
	Run, SHA, ConfigDigest string
	EstimateMicro          int64
	Actor                  string
}

// NotePending records the estimate of a run held for spend approval. The
// dispatcher calls it for a job the scheduler holds with approval.pending, which
// never reaches the budget step; a job that does reach it leaves the figure in its refusal.
func NotePending(ctx context.Context, run string, estimateMicro int64) error {
	env, ok := envOf(ctx)
	if !ok {
		return nil
	}
	return env.Store.NotePending(ctx, run, estimateMicro)
}

// ApproveSpend approves the spend of run as actor; see the file comment.
func ApproveSpend(ctx context.Context, run string, actor roles.Actor) (Approval, error) {
	env, ok := envOf(ctx)
	if !ok {
		return Approval{}, refuse(model.ApprovalPending, "economics is not bound, so there is no spend to approve")
	}
	facts, found, err := env.Store.PipelineFacts(ctx, run)
	if err != nil {
		return Approval{}, err
	}
	if !found {
		if err = roles.AuthorizeAction(ctx, actor, roles.ActSpendApprove, ""); err != nil {
			return Approval{}, err // a member learns nothing about which runs exist
		}
		return Approval{}, refuse(model.ApprovalPending, "run %s is unknown", run)
	}
	project, _ := ctx.Value(projectKey{}).(string)
	if err = roles.AuthorizeAction(ctx, actor, roles.ActSpendApprove, project); err != nil {
		return Approval{}, err
	}
	noted, details, err := env.Store.PendingEstimate(ctx, run)
	if err != nil {
		return Approval{}, err
	}
	estimate := noted
	for _, d := range details {
		if v, ok := strings.CutPrefix(d, PendingDetailPrefix); ok {
			if n, perr := strconv.ParseInt(strings.Fields(v + " ")[0], 10, 64); perr == nil {
				estimate = max(estimate, n)
			}
		}
	}
	if estimate <= 0 {
		return Approval{}, refuse(model.ApprovalPending, "run %s has no spend awaiting approval", run)
	}
	a := Approval{Run: run, SHA: facts.Revision, ConfigDigest: facts.ConfigDigest, EstimateMicro: estimate, Actor: actor.Label()}
	detail := fmt.Sprintf("amount_micro=%d sha=%s config=%s", estimate, a.SHA, a.ConfigDigest)
	if err = env.Store.PutApproval(ctx, storeApproval(a), detail); err != nil {
		return Approval{}, err
	}
	return a, nil
}

func storeApproval(a Approval) store.Approval {
	return store.Approval{Run: a.Run, SHA: a.SHA, ConfigDigest: a.ConfigDigest, EstimateMicro: a.EstimateMicro, Actor: a.Actor}
}
