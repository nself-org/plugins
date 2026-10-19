package dispatch

// Purpose: the audit row of a placement on an overridden deploy host (D21,
// D-1249). trust.RequiresAudit says which placements need one; Admit appends
// this step for them.
// Inputs:  the Request being admitted.
// Outputs: one audit row, placement.deploy_host_override, target = attempt id.
// Constraints: the row is written inside the admission transaction, so a lease
// on such a host exists only with its row; a failed write refuses the placement
// (node.deploy_host) and the job stays queued. The step is added by Admit, not
// registered, so no registry reset can drop it.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// DeployHostAuditAction is the action of the audit row.
const DeployHostAuditAction = "placement.deploy_host_override"

func deployHostAudit(r Request) store.AdmitStep {
	return func(ctx context.Context, v store.AdmitView) error {
		detail := "runner=" + r.Placement.RunnerID + ";job=" + r.Job.Name + ";trust=" + string(r.Job.Trust)
		if err := v.AppendAudit(ctx, "coordinator", DeployHostAuditAction, r.AttemptID, detail); err != nil {
			return &Refusal{Reason: model.NodeDeployHost, Detail: "audit row for the overridden deploy host could not be written"}
		}
		return nil
	}
}
