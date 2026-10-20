package store

// Purpose: the month totals behind `nself ci policy usage` (P7-SCHED-07, Epic D11,
// X18): what the local ledger and the burst VMs hold per provider in a UTC month.
// Inputs:  the ledger and resource_ledger tables.
// Outputs: MonthTotals per provider.
// Constraints: read only; money saturates; an attempt counts at its actual once
// settled and at its estimate before, a VM at its actual after a confirmed
// teardown and at its reservation before.

import (
	"context"
	"database/sql"
)

// MonthTotals is what the local ledger holds for one provider in a UTC month.
type MonthTotals struct {
	Attempts, Settled int
	ActualMicro       int64 // settled attempts
	PendingMicro      int64 // estimates of attempts not settled yet
	AllComputed       bool  // every settled attempt is computed or known
}

// LedgerMonth totals the ledger by provider for a UTC month (YYYY-MM).
func (s *Store) LedgerMonth(ctx context.Context, month string) (map[string]MonthTotals, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT provider,est_micro,act_micro,confidence FROM ledger WHERE substr(window_day,1,7)=? ORDER BY provider,attempt_id", month)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	out := map[string]MonthTotals{}
	for rows.Next() {
		var provider, conf string
		var est int64
		var act sql.NullInt64
		if err = rows.Scan(&provider, &est, &act, &conf); err != nil {
			return nil, coded("E607", err.Error())
		}
		m, seen := out[provider]
		if !seen {
			m.AllComputed = true
		}
		m.Attempts++
		if act.Valid {
			m.Settled++
			m.ActualMicro, _ = satAdd(m.ActualMicro, act.Int64)
			m.AllComputed = m.AllComputed && (conf == "computed" || conf == "known")
		} else {
			m.PendingMicro, _ = satAdd(m.PendingMicro, est)
		}
		out[provider] = m
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	// A burst VM is money too, held in resource_ledger rather than as an attempt's estimate.
	vms, err := s.readers.QueryContext(ctx, "SELECT provider,reserved_micro,actual_micro,confirmed_teardown_at FROM resource_ledger WHERE substr(window_day,1,7)=? ORDER BY provider,intent", month)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = vms.Close() }()
	for vms.Next() {
		var provider string
		var reservedMicro int64
		var act, done sql.NullInt64
		if err = vms.Scan(&provider, &reservedMicro, &act, &done); err != nil {
			return nil, coded("E607", err.Error())
		}
		m, seen := out[provider]
		if !seen {
			m.AllComputed = true
		}
		m.Attempts++
		if done.Valid {
			m.Settled++
			m.ActualMicro, _ = satAdd(m.ActualMicro, act.Int64)
		} else {
			m.PendingMicro, _ = satAdd(m.PendingMicro, reservedMicro)
		}
		out[provider] = m
	}
	return out, vms.Err()
}
