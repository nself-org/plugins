package isolation

// Purpose: turn a network scope into runtime network settings.
// Inputs:  a model.NetworkScope. Outputs: a NetworkPlan or E679.
// Constraints: the default planner supports none and lan only; restricted,
// internet and privileged need the scope planner P7-TRUST-08 installs, so they
// are refused here rather than weakened.

import "github.com/nself-org/plugins/free/ci/internal/model"

// NetworkPlan is what the runtime CLI needs for a scope.
type NetworkPlan struct {
	// Network is the --network value ("none" or a bridge name).
	Network string
	// DNS lists --dns servers; empty keeps the runtime default.
	DNS []string
}

// NetworkPlanner maps a scope to a plan.
type NetworkPlanner interface {
	Plan(scope model.NetworkScope) (NetworkPlan, error)
}

type defaultPlanner struct{}

// DefaultNetworkPlanner supports none (also the empty scope) and lan.
func DefaultNetworkPlanner() NetworkPlanner { return defaultPlanner{} }

func (defaultPlanner) Plan(scope model.NetworkScope) (NetworkPlan, error) {
	switch scope {
	case "", "none":
		return NetworkPlan{Network: "none"}, nil
	case "lan":
		return NetworkPlan{Network: "bridge"}, nil
	}
	return NetworkPlan{}, ScopeUnenforceable("network scope %q cannot be enforced by the default planner", scope)
}
