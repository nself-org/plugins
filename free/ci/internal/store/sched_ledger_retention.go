package store

// Purpose: retention of the usage ledger (D-1281, P7-SCHED-07): a row that no
// budget check can read any more is deleted in the transaction that inserts the
// next one, as sched_telemetry does, so the tables stop growing without bound.
// Inputs:  the store clock; the insert paths PutLedger, PutResource and PutUsage.
// Outputs: deletes only.
// Constraints: only settled rows of a window older than LedgerKeepDays go: an
// open attempt, an unsettled reservation, a resource that holds money and a
// reservation with no window (the provider scope) are read by a budget check
// and are kept for ever. The cutoff is a UTC day key from the store clock.

import "time"

// LedgerKeepDays is how long a settled row of a past window is kept.
const LedgerKeepDays = 400

// pruneLedger deletes the settled rows of windows older than LedgerKeepDays.
func (t LedgerTx) pruneLedger() error {
	cutoff := time.Unix(0, t.Now()).UTC().AddDate(0, 0, -LedgerKeepDays).Format("2006-01-02")
	month := cutoff[:7]
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"DELETE FROM ledger WHERE finalized_at IS NOT NULL AND window_day<?", []any{cutoff}},
		{"DELETE FROM resource_ledger WHERE confirmed_teardown_at IS NOT NULL AND window_day<?", []any{cutoff}},
		{"DELETE FROM reservation WHERE settled=1 AND window_key<>'' AND window_key<CASE WHEN length(window_key)=7 THEN ? ELSE ? END", []any{month, cutoff}},
		{"DELETE FROM ledger_usage WHERE period<?", []any{month}},
	} {
		if _, err := t.tx.ExecContext(t.ctx, q.sql, q.args...); err != nil {
			return err
		}
	}
	return nil
}
