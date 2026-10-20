package run

// Purpose: deliver the coordinator's recorded evidence context with a remote lease.
// Inputs: the run's store-backed sink and admitted attempt ID.
// Outputs: wire context matching the remote verifier's ledger, or an error.
// Constraints: never guess context for a production attempt; non-store adapters retain legacy leases.

import (
	"context"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func remoteEvidenceContext(ctx context.Context, sink transport.Sink, id string) (*protocol.EvidenceContext, error) {
	s, ok := sink.(*remoteSink)
	if !ok {
		return nil, nil
	}
	f, found, err := (store.AttestLedger{S: s.st}).AttemptFacts(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("remote evidence context: attempt %s has no recorded lease", id)
	}
	attempts, err := s.st.PipelineAttempts(ctx, f.RunID)
	if err != nil {
		return nil, err
	}
	for _, a := range attempts {
		if a.ID == id {
			return &protocol.EvidenceContext{RunID: f.RunID, RepoID: f.RepoID, SourceTrust: model.TrustClass(f.SourceTrust),
				PolicyDigest: f.PolicyDigest, SelectionMode: model.SelectionMode(f.SelectionMode), Job: f.Job, AttemptN: a.N}, nil
		}
	}
	return nil, fmt.Errorf("remote evidence context: attempt %s is missing from its pipeline", id)
}
