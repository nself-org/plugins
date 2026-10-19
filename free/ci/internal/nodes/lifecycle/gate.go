package lifecycle

// Purpose: the lease gate of ADR 0025 §10: a revoked node, or a node not authorized for the project, gets
// no job. Callers (the dispatcher, the pairing listener, the coordinator) ask the gate before they hand
// a node a lease; the answer is E657 with a reason key.
// Inputs:  the registry, a node id and the project id of the job.
// Outputs: nil, E650 (unknown node) or E657 (revoked or not authorized).
// Constraints: read-only; a discovered node is not authorized; revocation wins over authorization.

import (
	"context"
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

// Gate refuses a revoked node or one not authorized for project.
func Gate(ctx context.Context, r *registry.Registry, nodeID, project string) error {
	n, err := r.Get(ctx, nodeID)
	if err != nil {
		return err
	}
	c := n.Capability
	state := ""
	if c.Availability.State.Value != nil {
		state = *c.Availability.State.Value
	}
	if c.Lifecycle.RevokedAt != nil || n.Record.State == "revoked" || state == "revoked" {
		return newErr(CodeRevoked, ReasonRevoked, "node %s is revoked and gets no jobs", nodeID)
	}
	if c.Lifecycle.Discovered || project == "" || !slices.Contains(c.Lifecycle.AuthorizedProjects, project) {
		return newErr(CodeRevoked, ReasonNotAuthorized, "node %s is not authorized for this project", nodeID)
	}
	return nil
}
