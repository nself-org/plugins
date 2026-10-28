package store

// Purpose: ephemeral resources of the usage ledger (P7-SCHED-07, Epic D12, review
// F7): the money held for a burst VM from its intent to its confirmed teardown.
// Inputs:  calls from internal/economics inside P7-SCHED-08's transactions.
// Outputs: the resource row and the settle of the intent's reservations.
// Constraints: the intent is unique; a release is a no-op the second time; the
// actual is not capped at the reservation, because a failed teardown keeps the
// VM alive and billing.

import (
	"context"
	"database/sql"
	"errors"
)

// ResourceRow is the money held for one ephemeral resource from intent to confirmed teardown.
type ResourceRow struct {
	Intent, Provider, Pipeline, WindowDay string
	ReservedMicro                         int64
	Rate                                  RateSnap
	ProvisionedAt                         int64
	ConfirmedTeardownAt, ActualMicro      *int64
}

// PutResource records a new resource; the intent is unique.
func (t LedgerTx) PutResource(r ResourceRow) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO resource_ledger (intent,provider,pipeline_id,reserved_micro,window_day,rate_micro,rate_unit_ms,inc_ms,min_micro,extras_micro,provisioned_at)
 VALUES (?,?,?,?,?,?,?,?,?,?,?)`, r.Intent, r.Provider, r.Pipeline, r.ReservedMicro, r.WindowDay, r.Rate.RateMicro, r.Rate.RateUnitMs, r.Rate.IncMs, r.Rate.MinMicro, r.Rate.ExtrasMicro, r.ProvisionedAt)
	if err != nil {
		return err
	}
	return t.pruneLedger()
}

// Resource reads a resource row inside the transaction.
func (t LedgerTx) Resource(intent string) (ResourceRow, bool, error) {
	return resource(t.ctx, t.tx, intent)
}

// Resource reads a resource row.
func (s *Store) Resource(ctx context.Context, intent string) (ResourceRow, bool, error) {
	return resource(ctx, s.readers, intent)
}

func resource(ctx context.Context, q ledgerQuerier, intent string) (ResourceRow, bool, error) {
	var r ResourceRow
	var done, act sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT intent,provider,pipeline_id,reserved_micro,window_day,rate_micro,rate_unit_ms,inc_ms,min_micro,extras_micro,provisioned_at,confirmed_teardown_at,actual_micro
 FROM resource_ledger WHERE intent=?`, intent).Scan(&r.Intent, &r.Provider, &r.Pipeline, &r.ReservedMicro, &r.WindowDay, &r.Rate.RateMicro, &r.Rate.RateUnitMs, &r.Rate.IncMs, &r.Rate.MinMicro, &r.Rate.ExtrasMicro, &r.ProvisionedAt, &done, &act)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceRow{}, false, nil
	}
	if done.Valid {
		r.ConfirmedTeardownAt = &done.Int64
	}
	if act.Valid {
		r.ActualMicro = &act.Int64
	}
	return r, err == nil, err
}

// ReleaseResource marks a confirmed teardown and settles the intent's
// reservations at actual. A VM whose failed teardown was retried for hours cost
// what it cost, even past its reservation, so the actual is not capped. A second
// release is a no-op.
func (t LedgerTx) ReleaseResource(intent string, at, actual int64) error {
	res, err := t.tx.ExecContext(t.ctx, "UPDATE resource_ledger SET confirmed_teardown_at=?, actual_micro=? WHERE intent=? AND confirmed_teardown_at IS NULL", at, actual, intent)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err
	}
	return settleReservations(t.ctx, t.tx, intent, actual, false)
}

// ResourcesOpen counts the resources of a provider that hold money (no confirmed teardown yet).
func (s *Store) ResourcesOpen(ctx context.Context) ([]ResourceRow, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT intent FROM resource_ledger WHERE confirmed_teardown_at IS NULL ORDER BY intent")
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, coded("E607", err.Error())
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	var out []ResourceRow
	for _, id := range ids {
		r, ok, err := s.Resource(ctx, id)
		if err != nil {
			return nil, coded("E607", err.Error())
		}
		if ok {
			out = append(out, r)
		}
	}
	return out, nil
}
