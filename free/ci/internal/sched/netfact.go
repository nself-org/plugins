package sched

// Purpose: the derived NoLANRoute fact of a runner for one job (P7-TRUST-08, Epic D4).
// Inputs:  the job and the scopes a probe proved the runner enforces (World.Network).
// Outputs: whether the runner has no LAN route for the job's effective scope.
// Constraints: derived from the proof only, never declared; a scope at or above lan is never "no LAN route".

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// noLANRoute is true when the job's effective scope is below lan and the runner proved it enforces that scope.
func noLANRoute(j Job, enforced []model.NetworkScope) bool {
	scope := j.Network
	if scope == "" {
		scope = trust.DefaultNetwork(j.Trust)
	}
	return trust.NoLANRoute(scope, enforced)
}
