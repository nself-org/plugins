package run

// Purpose: the evidence context a remote lease carries (D-1299): what the coordinator recorded for the
// attempt, read with the very query the signed-evidence verifier compares with (store.AttestLedger), so a
// node that names it in its evidence and signs that evidence produces a document attest.RunnerVerifier
// accepts.
// Inputs:  the run's store and the attempt id of the lease being delivered.
// Outputs: a *protocol.LeaseEvidence, or nil when there is no store or no recorded lease holder.
// Constraints: a store read error fails the delivery (the lease would otherwise go out with evidence that
// cannot verify); the node never decides these fields, it only repeats them.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// LeaseEvidence reads the attempt's recorded facts for its lease (exported for the agent's end-to-end signing test).
func LeaseEvidence(ctx context.Context, st *store.Store, attemptID string) (*protocol.LeaseEvidence, error) {
	if st == nil {
		return nil, nil
	}
	f, ok, err := store.AttestLedger{S: st}.AttemptFacts(ctx, attemptID)
	if err != nil || !ok {
		return nil, err
	}
	return &protocol.LeaseEvidence{RunID: f.RunID, RepoID: f.RepoID, SourceTrust: f.SourceTrust, PolicyDigest: f.PolicyDigest,
		SelectionMode: f.SelectionMode, Job: f.Job}, nil
}
