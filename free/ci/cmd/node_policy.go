package main

// Purpose: the coordinator's answer to a pull agent that asks for its contention policy in hello (D-1226):
//          agentapi Config.Policy reads the node's stored settings (`nodes set`) for the welcome.
// Inputs:  the store; the authenticated node id.
// Outputs: the protocol.NodePolicy of the node, or nil when the operator never set one.
// Constraints: read only; a store error is returned (agentapi logs it and sends no policy, so the node
//          keeps the policy it last stored).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func nodePolicy(st *store.Store) func(ctx context.Context, nodeID string) (*protocol.NodePolicy, error) {
	return func(ctx context.Context, nodeID string) (*protocol.NodePolicy, error) {
		s, ok, err := st.NodeSettingsGet(ctx, nodeID)
		if err != nil || !ok {
			return nil, err
		}
		return &protocol.NodePolicy{Node: s.NodeID, ReservedCPU: s.ReservedCPU, ReservedMemMB: s.ReservedMemMB,
			Interactive: s.Interactive, OnBattery: s.OnBattery, Persistence: s.Persistence, Version: s.Version}, nil
	}
}
