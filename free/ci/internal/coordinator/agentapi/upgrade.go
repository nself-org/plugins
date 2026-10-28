package agentapi

// Purpose: the upgrade conversation with a connected pull agent: the upgrade frame down the control stream and the
//          upgrade-result frame the agent answers on the stream (runner-protocol v1, additive).
// Inputs:  Upgrade (version, manifest URL, allow-downgrade); the agent's upgrade-result frame.
// Outputs: Server.Upgrade queues the frame; Server.WaitUpgradeResult returns the agent's answer for the version asked.
// Constraints: Upgrade clears the previous answer before it queues the frame, and WaitUpgradeResult returns only an answer
//          for the same version, so an answer to an earlier upgrade is never taken for this one; the answer is held in
//          memory per node and dropped with the node (CloseNode); a frame that is not a refusal or a done is a
//          protocol error before it gets here (the codec checks it).

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Upgrade asks the node's agent to upgrade to version from manifestURL. allowDowngrade is sent only when true.
func (s *Server) Upgrade(nodeID, version, manifestURL string, allowDowngrade bool) error {
	s.mu.Lock()
	if ns := s.nodes[nodeID]; ns != nil {
		ns.upgrade = nil
	}
	s.mu.Unlock()
	return s.queueControl(nodeID, protocol.Message{Type: "upgrade", Body: protocol.Upgrade{Version: version, ManifestURL: manifestURL, AllowDowngrade: allowDowngrade}})
}

// recordUpgrade keeps the agent's answer and wakes WaitUpgradeResult.
func (s *Server) recordUpgrade(nodeID string, r protocol.UpgradeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := s.state(nodeID)
	ns.upgrade = &r
	ns.signal()
}

// WaitUpgradeResult waits up to wait for the agent's answer to the upgrade to version. It returns the context's error when ctx
// ends first and a timeout error when the agent says nothing (an agent that cannot answer, or one that restarted at once).
func (s *Server) WaitUpgradeResult(ctx context.Context, nodeID, version string, wait time.Duration) (protocol.UpgradeResult, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		s.mu.Lock()
		ns := s.state(nodeID)
		if r := ns.upgrade; r != nil && r.Version == version {
			s.mu.Unlock()
			return *r, nil
		}
		changed := ns.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-timer.C:
			return protocol.UpgradeResult{}, fmt.Errorf("agentapi: node %s sent no upgrade result for %s within %s", nodeID, version, wait)
		case <-ctx.Done():
			return protocol.UpgradeResult{}, ctx.Err()
		}
	}
}
