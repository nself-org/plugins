package store

// Purpose: the two writes `nself ci serve restore` needs and no other store call offers (P7-NODE-20, Unit B):
//          dropping the job-token keys of the restored image, and marking nodes needs-reconfirm.
// Inputs:  an actor name (audited); for the node mark, the reason text shown by `nodes show`.
// Outputs: PKITokenKeysPurge returns how many token keys it removed; NodesNeedReconfirm how many nodes it marked.
// Constraints: each is ONE transaction with ONE audit row carrying counts only; a purge removes the rows, so a
//          token signed by a removed key fails verification with an unknown key id (pki E660
//          token_unknown_key, E661 at the carrier) and `Tokens` then mints a fresh key; the node mark changes
//          only the record state and reason (never the capability document, which stays valid under the
//          availability enum) and skips revoked nodes, so a revocation is never softened; `nodes enable`
//          (registry SetState online) is the only way back.

import (
	"context"
	"database/sql"
	"strconv"
)

// NodeStateNeedsReconfirm is the record state of a node restored onto an empty data dir (Epic D21, review F15).
const NodeStateNeedsReconfirm = "needs-reconfirm"

// PKITokenKeysPurge deletes every job-token key and audits it. The caller creates the next key.
func (s *Store) PKITokenKeysPurge(ctx context.Context, actor string) (removed int64, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `DELETE FROM pki_token_key`)
		if e != nil {
			return e
		}
		removed, _ = res.RowsAffected()
		return s.appendAudit(ctx, tx, actor, "pki.token_keys.purge", "coordinator", "removed="+strconv.FormatInt(removed, 10))
	})
	if err != nil {
		removed = 0
	}
	return removed, err
}

// NodesNeedReconfirm sets every node that is not revoked to needs-reconfirm with the reason text.
func (s *Store) NodesNeedReconfirm(ctx context.Context, actor, reason string) (marked int64, err error) {
	err = s.write(ctx, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `UPDATE node SET state=?,reason=?,version=version+1,updated_at=? WHERE state<>'revoked'`, NodeStateNeedsReconfirm, reason, s.now().UnixNano())
		if e != nil {
			return e
		}
		marked, _ = res.RowsAffected()
		return s.appendAudit(ctx, tx, actor, "node.needs_reconfirm", "coordinator", "nodes="+strconv.FormatInt(marked, 10))
	})
	if err != nil {
		marked = 0
	}
	return marked, err
}

// GrantPrincipalsRevokedOnly lists the principals that have at least one revoked grant and no live grant:
// the positive evidence a restore needs to revoke them again after it brought an older image back.
func (s *Store) GrantPrincipalsRevokedOnly(ctx context.Context) ([]string, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT DISTINCT principal FROM trust_grant WHERE revoked_at IS NOT NULL AND principal NOT IN (SELECT principal FROM trust_grant WHERE revoked_at IS NULL) ORDER BY principal`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var who string
		if err = rows.Scan(&who); err != nil {
			return nil, err
		}
		out = append(out, who)
	}
	return out, rows.Err()
}
