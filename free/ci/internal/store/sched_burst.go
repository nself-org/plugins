package store

// Purpose: the store half of burst (P7-SCHED-08, Epic D12): the intent that is
// committed before any provider call, its provisioning lease, and the intent's
// state machine live -> tearing-down -> terminal. Migration set sched-burst.
// Inputs:  calls from internal/burst, inside its own transactions or inside the
// ledger transaction that reserves the lifetime cost.
// Outputs: BurstTx for in-transaction work, Store reads for the sweeper, the
// decision and the world contributor.
// Constraints: times are unix milliseconds; the token is the 26-character intent
// id that labels the resource; a state only moves forward (terminal is final);
// no foreign key crosses the set (attempt is read by store code in sched_burst_ops.go).

import (
	"context"
	"database/sql"
	"embed"
	"errors"
)

//go:embed migrations/sched-burst/*.sql
var schedBurstMigrations embed.FS

func init() {
	if err := Register("sched-burst", schedBurstMigrations); err != nil {
		panic(err)
	}
}

// The intent states.
const (
	BurstLive        = "live"
	BurstTearingDown = "tearing-down"
	BurstTerminal    = "terminal"
)

// BurstIntent is one provision from its commit to its confirmed teardown.
type BurstIntent struct {
	Token, Coordinator, Provider, Pipeline, JobKey, Platform string
	ReservedMicro, LifetimeMs                                int64
	State, ResourceID, NodeID, Reason                        string
	CreatedAt, UpdatedAt                                     int64
}

// BurstLease is the provisioning lease: the idle and the maximum deadline.
type BurstLease struct{ IdleUntil, MaxUntil int64 }

// BurstTx is a write transaction over the burst tables.
type BurstTx struct {
	s   *Store
	ctx context.Context
	tx  *sql.Tx
}

// Burst is the burst handle of the ledger transaction that reserves the lifetime cost.
func (t LedgerTx) Burst() BurstTx { return BurstTx(t) }

// Burst is the burst handle of an admission step; it lives in the admission transaction.
func (v AdmitView) Burst(ctx context.Context) BurstTx { return BurstTx{v.s, ctx, v.tx} }

// BurstWrite runs fn in one writer transaction.
func (s *Store) BurstWrite(ctx context.Context, fn func(BurstTx) error) error {
	return s.write(ctx, func(tx *sql.Tx) error { return fn(BurstTx{s, ctx, tx}) })
}

// BurstRevokeHook marks the live intent bound to the revoked node tearing-down in
// the revoke transaction (Epic D12: one transaction revokes the node and marks the
// intent). It matches nothing for a node that is not a burst node.
func BurstRevokeHook() RevokeHook {
	return func(_ context.Context, tx *RevokeTx, nodeID string) error {
		_, err := tx.Exec(`UPDATE burst_intent SET state='tearing-down', reason='node revoked', updated_at=? WHERE node_id=? AND state='live'`, tx.s.now().UnixMilli(), nodeID)
		return err
	}
}

// NowMs is the store clock in unix milliseconds, so a row and its transaction agree.
func (t BurstTx) NowMs() int64 { return t.s.now().UnixMilli() }

// SetIdle moves the idle deadline of a non-terminal intent (the node joined, or its job ended).
func (t BurstTx) SetIdle(token string, idleUntil int64) error {
	_, err := t.tx.ExecContext(t.ctx, `UPDATE burst_provisioning_lease SET idle_until=? WHERE intent=?`, idleUntil, token)
	return err
}

// PutIntent records a new live intent with its lease.
func (t BurstTx) PutIntent(i BurstIntent, l BurstLease) error {
	if _, err := t.tx.ExecContext(t.ctx, `INSERT INTO burst_intent (token,coordinator_id,provider,pipeline_id,job_key,platform,reserved_micro,lifetime_ms,state,created_at,updated_at)
 VALUES (?,?,?,?,?,?,?,?,'live',?,?)`, i.Token, i.Coordinator, i.Provider, i.Pipeline, i.JobKey, i.Platform, i.ReservedMicro, i.LifetimeMs, i.CreatedAt, i.CreatedAt); err != nil {
		return err
	}
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO burst_provisioning_lease (intent,idle_until,max_until) VALUES (?,?,?)`, i.Token, l.IdleUntil, l.MaxUntil)
	return err
}

const intentCols = `token,coordinator_id,provider,pipeline_id,job_key,platform,reserved_micro,lifetime_ms,state,resource_id,node_id,reason,created_at,updated_at`

func scanIntent(r interface{ Scan(...any) error }) (BurstIntent, error) {
	var i BurstIntent
	err := r.Scan(&i.Token, &i.Coordinator, &i.Provider, &i.Pipeline, &i.JobKey, &i.Platform, &i.ReservedMicro, &i.LifetimeMs, &i.State, &i.ResourceID, &i.NodeID, &i.Reason, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

// Intent reads an intent inside the transaction.
func (t BurstTx) Intent(token string) (BurstIntent, bool, error) {
	i, err := scanIntent(t.tx.QueryRowContext(t.ctx, `SELECT `+intentCols+` FROM burst_intent WHERE token=?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return BurstIntent{}, false, nil
	}
	return i, err == nil, err
}

// BurstIntent reads an intent.
func (s *Store) BurstIntent(ctx context.Context, token string) (BurstIntent, bool, error) {
	i, err := scanIntent(s.readers.QueryRowContext(ctx, `SELECT `+intentCols+` FROM burst_intent WHERE token=?`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return BurstIntent{}, false, nil
	}
	return i, err == nil, err
}

// BurstIntents lists a coordinator's intents that are not terminal, oldest first.
func (s *Store) BurstIntents(ctx context.Context, coordinator string) ([]BurstIntent, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT `+intentCols+` FROM burst_intent WHERE coordinator_id=? AND state<>'terminal' ORDER BY created_at, token`, coordinator)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []BurstIntent
	for rows.Next() {
		i, err := scanIntent(rows)
		if err != nil {
			return nil, coded("E607", err.Error())
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// SetResource records the provider's resource id on a non-terminal intent; false means the intent already ended.
func (t BurstTx) SetResource(token, resourceID string, at int64) (bool, error) {
	res, err := t.tx.ExecContext(t.ctx, `UPDATE burst_intent SET resource_id=?, updated_at=? WHERE token=? AND state<>'terminal'`, resourceID, at, token)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// SetNode binds the enrolled node to a live intent; the node must be unbound.
func (t BurstTx) SetNode(token, nodeID string, at int64) (bool, error) {
	res, err := t.tx.ExecContext(t.ctx, `UPDATE burst_intent SET node_id=?, updated_at=? WHERE token=? AND state='live' AND node_id=''`, nodeID, at, token)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// MarkTearingDown moves a live intent to tearing-down; any other state is left as it is.
func (t BurstTx) MarkTearingDown(token, reason string, at int64) error {
	_, err := t.tx.ExecContext(t.ctx, `UPDATE burst_intent SET state='tearing-down', reason=?, updated_at=? WHERE token=? AND state='live'`, reason, at, token)
	return err
}

// MarkTearingDownIfUnused is MarkTearingDown for the idle and join-timeout teardowns: it refuses (false) when the
// bound node has taken its one job, so a lease committed between the decision and the mark is never torn down under.
// A refusal because the intent is no longer live is also false; the caller reads the intent to tell them apart.
func (t BurstTx) MarkTearingDownIfUnused(token, reason string, at int64) (bool, error) {
	res, err := t.tx.ExecContext(t.ctx, `UPDATE burst_intent SET state='tearing-down', reason=?, updated_at=? WHERE token=? AND state='live'
 AND NOT EXISTS (SELECT 1 FROM burst_node_use u WHERE u.node_id=burst_intent.node_id AND burst_intent.node_id<>'')`, reason, at, token)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// MarkTerminal ends an intent: its resource is confirmed gone.
func (t BurstTx) MarkTerminal(token string, at int64) error {
	if _, err := t.tx.ExecContext(t.ctx, `UPDATE burst_intent SET state='terminal', updated_at=? WHERE token=?`, at, token); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(t.ctx, `DELETE FROM burst_provisioning_lease WHERE intent=?`, token); err != nil {
		return err
	}
	_, err := t.tx.ExecContext(t.ctx, `DELETE FROM burst_teardown_failure WHERE intent=?`, token)
	return err
}

// AppendAudit adds an audit row to the transaction.
func (t BurstTx) AppendAudit(actor, action, target, detail string) error {
	return t.s.appendAudit(t.ctx, t.tx, actor, action, target, detail)
}

// BurstLease reads the provisioning lease of an intent.
func (s *Store) BurstLease(ctx context.Context, token string) (BurstLease, bool, error) {
	var l BurstLease
	err := s.readers.QueryRowContext(ctx, `SELECT idle_until,max_until FROM burst_provisioning_lease WHERE intent=?`, token).Scan(&l.IdleUntil, &l.MaxUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return BurstLease{}, false, nil
	}
	return l, err == nil, err
}
