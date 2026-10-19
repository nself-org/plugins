package run

// Purpose: wire runner-key evidence verification into the run (P7-TRUST-20, Epic D13). At init the
// attest.RunnerVerifier becomes the remote verifier, so signed-remote evidence satisfies a gate only when
// its key, purpose, revocation, lease holder, node eligibility and replay checks pass. RevokeNode is the
// call a node revocation makes: it revokes the node's evidence keys, quarantines the runs they signed,
// audits each promoted release and re-posts the affected statuses as errors.
// Inputs:  the run's store (taken from the first attempt that starts, or set with UseAttestStore); for
//          RevokeNode a store, an audit log and a node id.
// Outputs: the registered verifier; attest.Affected from RevokeNode.
// Constraints: with no store known the verifier refuses everything (fail closed); the store is only read
//          and written through store.AttestLedger.

import (
	"context"
	"sync/atomic"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var attestStore atomic.Pointer[store.Store]

// UseAttestStore sets the store the remote verifier reads (a coordinator calls it at start).
func UseAttestStore(st *store.Store) { attestStore.Store(st) }

func attestLedger() attest.Ledger {
	if st := attestStore.Load(); st != nil {
		return store.AttestLedger{S: st}
	}
	return nil
}

func init() {
	attest.RegisterRemoteVerifier(&attest.RunnerVerifier{Ledger: attestLedger})
	RegisterAttemptStart(attestStoreHook{})
}

// attestStoreHook points the verifier at the store of the attempt that is starting.
type attestStoreHook struct{}

func (attestStoreHook) AttemptStart(_ context.Context, a AttemptInfo) error {
	if a.Store != nil {
		UseAttestStore(a.Store)
	}
	return nil
}

// statusReposter re-posts through report.Repost (P7-HOST-02).
type statusReposter struct{}

func (statusReposter) Repost(ctx context.Context, keys []attest.StatusKey, state, reason string) error {
	ks := make([]report.Key, len(keys))
	for i, k := range keys {
		ks[i] = report.Key{Revision: k.Revision, Context: k.Context}
	}
	return report.Repost(ctx, ks, report.State(state), reason)
}

// RevokeNode revokes the evidence keys of nodeID on behalf of actor and returns the re-evaluation set.
func RevokeNode(ctx context.Context, st *store.Store, log *audit.Log, nodeID, actor string) (attest.Affected, error) {
	r := attest.Revoker{Ledger: store.AttestLedger{S: st}, Reposter: statusReposter{}}
	if log != nil {
		r.Audit = func(ctx context.Context, actor, action, target, detail string) error {
			return log.Append(ctx, audit.Event{Actor: actor, Action: action, Target: target, Detail: detail})
		}
	}
	return r.Revoke(ctx, nodeID, actor)
}
