package run

// Purpose: read the network scopes a registered node proved (capability network.enforced_scopes) into the world
//          (D-1332, P7-TRUST-08 Epic D4), and run that proof on a node for the agent's `netscope-probe`.
// Inputs:  the world's runners and clock; the probe result a node reported, validated by the registry.
// Outputs: World.Network entries for registered ssh and agent nodes; ProveNetworkScopes for the agent.
// Constraints: the fact must be Valid, stated by a probe, known, not in the future and at most maxProofAge old;
//          anything else is "nothing proven" (fail closed: noLANRoute stays false). A runner called "local" is
//          never read here: the local proof belongs to the injected local runner only.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// maxProofAge is how long a node's reported proof counts. `nodes probe` refreshes it; a node nobody probes loses
// the proof and its untrusted work falls back to the "no LAN route" refusal.
const maxProofAge = 24 * time.Hour

// ProveNetworkScopes runs the scope proof on this machine now (it builds a job network and sidecar, so it is
// slow) and returns what it proved: nothing without a container runtime.
func ProveNetworkScopes(ctx context.Context) []model.NetworkScope {
	return localProof.prober.EnforcedScopes(ctx, containerRuntime(ctx))
}

// remoteScopes is the proof a registered node reported, or nil when it has none the coordinator may rely on.
func remoteScopes(r sched.Runner, nowMs int64) []model.NetworkScope {
	id := r.Capability.Identity
	if r.ID == "local" || id.Transport != "ssh" && id.Transport != "agent" || r.Capability.Network == nil {
		return nil
	}
	f := r.Capability.Network.EnforcedScopes
	if !f.Valid() || f.Source != "probe" || f.Confidence != "known" {
		return nil
	}
	age := time.UnixMilli(nowMs).Sub(f.ObservedAt)
	if age < 0 || age > maxProofAge {
		return nil
	}
	return append([]model.NetworkScope(nil), (*f.Value)...)
}
