package isolation

// Purpose: turn a network scope into runtime network settings.
// Inputs:  a model.NetworkScope (and, for a scope that needs per-attempt resources, the attempt).
// Outputs: a NetworkPlan or E679.
// Constraints: the default planner supports none and lan only; restricted, internet and privileged need the
// scope planner P7-TRUST-08 installs (InstallNetworkPlanner), so they are refused here rather than weakened.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// NetworkPlan is what the runtime CLI needs for a scope.
type NetworkPlan struct {
	// Network is the --network value ("none" or a bridge name).
	Network string
	// DNS lists --dns servers; empty keeps the runtime default.
	DNS []string
	// Env is added to the job environment (the proxy variables of a restricted or internet scope).
	Env []string
	// Release removes what the plan created for the attempt; nil when it created nothing. It must be safe to
	// call more than once and must not depend on the attempt's own context (the attempt may be cancelled).
	Release func(ctx context.Context)
}

// NetworkPlanner maps a scope to a plan.
type NetworkPlanner interface {
	Plan(scope model.NetworkScope) (NetworkPlan, error)
}

// AttemptPlanner is a NetworkPlanner that creates per-attempt resources (an internal network and a proxy
// sidecar). The container profile calls PlanAttempt when its planner implements it, with the runtime binary it
// probed; a plan with a Release func must be released by the caller on every path after PlanAttempt returned.
type AttemptPlanner interface {
	NetworkPlanner
	PlanAttempt(ctx context.Context, binary string, a Attempt, scope model.NetworkScope) (NetworkPlan, error)
}

type defaultPlanner struct{}

// basePlan is the stateless part: none (also the empty scope) and lan.
func basePlan(scope model.NetworkScope) (NetworkPlan, error) {
	switch scope {
	case "", "none":
		return NetworkPlan{Network: "none"}, nil
	case "lan":
		return NetworkPlan{Network: "bridge"}, nil
	}
	return NetworkPlan{}, ScopeUnenforceable("network scope %q cannot be enforced by the default planner", scope)
}

func (defaultPlanner) Plan(scope model.NetworkScope) (NetworkPlan, error) { return basePlan(scope) }

var installed = struct {
	sync.RWMutex
	p NetworkPlanner
}{p: defaultPlanner{}}

// InstallNetworkPlanner replaces the process-wide planner every profile built with the default one uses; a
// nil planner restores the default. The scope planner installs itself from its hook file (P7-TRUST-08).
func InstallNetworkPlanner(p NetworkPlanner) {
	installed.Lock()
	defer installed.Unlock()
	if p == nil {
		p = defaultPlanner{}
	}
	installed.p = p
}

func currentPlanner() NetworkPlanner {
	installed.RLock()
	defer installed.RUnlock()
	return installed.p
}

type dynamicPlanner struct{}

// DefaultNetworkPlanner is the process-wide planner: none and lan until InstallNetworkPlanner replaces it.
// It resolves the installed planner on every call, so a profile built before the install still uses it.
func DefaultNetworkPlanner() NetworkPlanner { return dynamicPlanner{} }

func (dynamicPlanner) Plan(scope model.NetworkScope) (NetworkPlan, error) {
	return currentPlanner().Plan(scope)
}

// PlanAttempt implements AttemptPlanner over the installed planner.
func (dynamicPlanner) PlanAttempt(ctx context.Context, binary string, a Attempt, scope model.NetworkScope) (NetworkPlan, error) {
	if ap, ok := currentPlanner().(AttemptPlanner); ok {
		return ap.PlanAttempt(ctx, binary, a, scope)
	}
	return currentPlanner().Plan(scope)
}
