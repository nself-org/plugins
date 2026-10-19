package store

// Purpose: NodeAdd, NodeRevoke and NodeRemove, the aggregate writes of the node lifecycle (P7-NODE-05,
//          ADR 0025 §2, §10): each is ONE transaction with ONE audit row.
// Inputs:  a node id, the admin actor and the revoker hooks other Tickets registered (the pairing
//          deny list of P7-NODE-07 adds the node's credentials to its deny list here).
// Outputs: the node row and its capability snapshot say revoked (or are gone for NodeRemove), every
//          in-flight lease of the node expires now, every hook ran, one audit row.
// Constraints: everything happens in ONE transaction; a failing hook rolls the whole write back
//          (state stays, leases stay) and the error names the hook. The audit row carries counts and
//          the node id only. NodeRemove keeps every audit row of the node.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// NodeAdd inserts a node that `nodes add` finished: the probed capability with the admin's trust,
// ownership and the one authorized project already in the document. It is the only write that accepts
// trust and lifecycle from the caller, and only with an admin authority (never "agent" or "probe").
func (s *Store) NodeAdd(ctx context.Context, n NodeRecord, admin AdminAuthority) error {
	if !admin.valid() {
		return coded("E651", "admin authority required")
	}
	var c model.Capability
	if err := json.Unmarshal(n.Capability, &c); err != nil || c.Identity.ID != n.ID || n.ID == "" {
		return coded("E651", "node document invalid")
	}
	if err := model.ValidateCapability(c); err != nil {
		return coded("E651", "node document invalid: "+err.Error())
	}
	if err := nodeDocument(&n, c); err != nil {
		return err
	}
	dh, err := json.Marshal(c.DeployHost)
	if err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		now := s.now().UnixNano()
		if _, err := tx.ExecContext(ctx, `INSERT INTO node(id,name,version,lifecycle,trust_json,deploy_host_json,state,reason,created_at,updated_at) VALUES(?,?,1,?,?,?,?,?,?,?)`, n.ID, c.Identity.Name, string(n.Lifecycle), string(n.Trust), string(dh), n.State, n.Reason, now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO capability_snapshot(node_id,digest,doc_json,observed_at) VALUES(?,?,?,?)`, n.ID, digestNode(n.Capability), string(n.Capability), now); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, admin.actor, "node.add", n.ID, "added")
	})
}

// RevokeTx is the transaction a revoker hook writes in. Exec is the only write path a hook gets, so a
// hook is part of the revoke commit and cannot outlive its rollback.
type RevokeTx struct {
	s   *Store
	ctx context.Context
	tx  *sql.Tx
}

// Exec runs one statement in the revoke transaction and returns the rows it changed.
func (t *RevokeTx) Exec(query string, args ...any) (int64, error) {
	res, err := t.tx.ExecContext(t.ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AppendAudit adds an audit row to the revoke transaction.
func (t *RevokeTx) AppendAudit(actor, action, target, detail string) error {
	return t.s.appendAudit(t.ctx, t.tx, actor, action, target, detail)
}

// RevokeHook runs inside the revoke transaction; returning an error aborts the revocation.
type RevokeHook func(ctx context.Context, tx *RevokeTx, nodeID string) error

// RevokeResult says what one revoke or remove changed.
type RevokeResult struct {
	LeasesExpired int64
	Removed       bool
}

// NodeRevoke marks the node revoked, expires its in-flight leases and runs the hooks, atomically.
func (s *Store) NodeRevoke(ctx context.Context, nodeID, actor string, hooks []RevokeHook) (RevokeResult, error) {
	return s.revokeWrite(ctx, nodeID, actor, "node.revoke", false, hooks)
}

// NodeRemove revokes the node (same transaction, same hooks) and deletes its record; audit rows stay.
func (s *Store) NodeRemove(ctx context.Context, nodeID, actor string, hooks []RevokeHook) (RevokeResult, error) {
	return s.revokeWrite(ctx, nodeID, actor, "node.remove", true, hooks)
}

func (s *Store) revokeWrite(ctx context.Context, nodeID, actor, action string, remove bool, hooks []RevokeHook) (RevokeResult, error) {
	var out RevokeResult
	if nodeID == "" || actor == "" {
		return out, coded("E651", "revoke needs a node and an actor")
	}
	err := s.write(ctx, func(tx *sql.Tx) error {
		var err error
		out = RevokeResult{}
		if out.LeasesExpired, err = s.revokeRows(ctx, tx, nodeID); err != nil {
			return err
		}
		rt := &RevokeTx{s: s, ctx: ctx, tx: tx}
		for i, h := range hooks {
			if err = h(ctx, rt, nodeID); err != nil {
				return fmt.Errorf("revoker %d failed: %w", i+1, err)
			}
		}
		if remove {
			for _, q := range []string{"DELETE FROM capability_snapshot WHERE node_id=?", "DELETE FROM node_failure_streak WHERE node_id=?", "DELETE FROM node WHERE id=?"} {
				if _, err = tx.ExecContext(ctx, q, nodeID); err != nil {
					return err
				}
			}
			out.Removed = true
		}
		return s.appendAudit(ctx, tx, actor, action, nodeID, fmt.Sprintf("leases_expired=%d hooks=%d", out.LeasesExpired, len(hooks)))
	})
	return out, err
}

// revokeRows writes the revoked state into the node row and snapshot and expires the leases.
func (s *Store) revokeRows(ctx context.Context, tx *sql.Tx, nodeID string) (int64, error) {
	var doc string
	var version int64
	err := tx.QueryRowContext(ctx, "SELECT n.version,c.doc_json FROM node n JOIN capability_snapshot c ON c.node_id=n.id WHERE n.id=?", nodeID).Scan(&version, &doc)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, coded("E650", "node not found")
	}
	if err != nil {
		return 0, err
	}
	var c model.Capability
	if err = json.Unmarshal([]byte(doc), &c); err != nil {
		return 0, err
	}
	now := s.now()
	revoked := "revoked"
	c.Availability.State = model.Fact[string]{Value: &revoked, Source: "assigned", ObservedAt: now.UTC(), Confidence: "known"}
	if c.Lifecycle.RevokedAt == nil {
		t := now.UTC()
		c.Lifecycle.RevokedAt = &t
	}
	n := NodeRecord{Version: version, UpdatedAt: now.UnixNano()}
	if err = nodeDocument(&n, c); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, "UPDATE node SET version=version+1,lifecycle=?,state='revoked',reason='revoked',updated_at=? WHERE id=? AND version=?", string(n.Lifecycle), n.UpdatedAt, nodeID, version)
	if err != nil {
		return 0, err
	}
	if changed, _ := res.RowsAffected(); changed != 1 {
		return 0, coded("E605", "node version changed")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE capability_snapshot SET digest=?,doc_json=?,observed_at=? WHERE node_id=?", digestNode(n.Capability), string(n.Capability), now.UnixNano(), nodeID); err != nil {
		return 0, err
	}
	return expireNodeLeases(ctx, tx, nodeID, now)
}

// expireNodeLeases moves the deadline of every live lease of the node to now, so the lease sweep fences
// it on its next pass and the node's late results are stale.
func expireNodeLeases(ctx context.Context, tx *sql.Tx, nodeID string, now time.Time) (int64, error) {
	res, err := tx.ExecContext(ctx, `UPDATE lease SET expires_at=? WHERE runner_id=? AND expires_at>? AND attempt_id IN (SELECT id FROM attempt WHERE state IN ('leased','running'))`, now.UnixNano(), nodeID, now.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
