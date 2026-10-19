package attest

// Purpose: Revoke(node) (Epic D13, X15): the node's evidence keys become untrusted, the evidence they
// signed is quarantined, and the caller gets the re-evaluation set (gates, posted statuses, promoted
// releases). The statuses are re-posted as errors with reason node.revoked (P7-HOST-02's Repost), and
// each promoted release gets one audit event, so a revoked node's success does not stand.
// Inputs:  the node id, the actor, a RevokeLedger, an AuditSink and a Reposter (all injected: this package
//          cannot import store, audit or report).
// Outputs: Affected; errors from the audit and repost steps are joined and returned with it.
// Constraints: the ledger step is one transaction and runs first, so a failure of the audit or the repost
//          never leaves a key trusted; every key the node pinned or ever used is revoked; Revoke is
//          idempotent (a second call returns the same set and posts again).

import (
	"context"
	"errors"
	"fmt"
)

// RevokeReason is the repost reason and the revocation reason recorded in the ledger.
const RevokeReason = "node.revoked"

// GateRef is a (repository, revision, job) a revoked key's evidence satisfied.
type GateRef struct{ RepoID, Revision, Job string }

// StatusKey names a status stream: the commit and the repository's status context.
type StatusKey struct{ Revision, Context string }

// ReleaseRef is a published release whose evidence a revoked key signed.
type ReleaseRef struct{ RunID, Project, Revision, ManifestSHA256 string }

// Affected is what a revocation re-evaluates.
type Affected struct {
	Gates    []GateRef
	Statuses []StatusKey
	Releases []ReleaseRef
}

// RevokeLedger is the persistent side of Revoke.
type RevokeLedger interface {
	// NodeKeyIDs lists the evidence key ids (ci-node-<16 hex>) the node pins or ever used.
	NodeKeyIDs(ctx context.Context, nodeID string) ([]string, error)
	// RevokeNode revokes the keys and quarantines their runs in one transaction.
	RevokeNode(ctx context.Context, nodeID, reason string, keyIDs []string) (Affected, error)
}

// AuditSink writes one audit event (audit.Log.Append in production).
type AuditSink func(ctx context.Context, actor, action, target, detail string) error

// Reposter re-posts statuses (report.Repost in production); state is the status state to post.
type Reposter interface {
	Repost(ctx context.Context, keys []StatusKey, state, reason string) error
}

// Revoker wires Revoke.
type Revoker struct {
	Ledger   RevokeLedger
	Audit    AuditSink
	Reposter Reposter
}

// Revoke revokes the evidence keys of nodeID on behalf of actor.
func (r Revoker) Revoke(ctx context.Context, nodeID, actor string) (Affected, error) {
	if r.Ledger == nil || nodeID == "" {
		return Affected{}, errors.New("attest: revoke needs a ledger and a node")
	}
	ids, err := r.Ledger.NodeKeyIDs(ctx, nodeID)
	if err != nil {
		return Affected{}, fmt.Errorf("attest: revoke %s: %w", nodeID, err)
	}
	var out Affected
	if len(ids) > 0 {
		if out, err = r.Ledger.RevokeNode(ctx, nodeID, RevokeReason, ids); err != nil {
			return Affected{}, fmt.Errorf("attest: revoke %s: %w", nodeID, err)
		}
	}
	var errs []error
	if r.Audit != nil {
		errs = append(errs, r.Audit(ctx, actor, "evidence.revoke", nodeID,
			fmt.Sprintf("keys=%d gates=%d statuses=%d releases=%d", len(ids), len(out.Gates), len(out.Statuses), len(out.Releases))))
		for _, rel := range out.Releases {
			errs = append(errs, r.Audit(ctx, actor, "release.quarantine", rel.RunID,
				fmt.Sprintf("node=%s project=%s revision=%s manifest=%s", nodeID, rel.Project, rel.Revision, rel.ManifestSHA256)))
		}
	}
	if r.Reposter != nil && len(out.Statuses) > 0 {
		errs = append(errs, r.Reposter.Repost(ctx, out.Statuses, "error", RevokeReason))
	}
	return out, errors.Join(errs...)
}
