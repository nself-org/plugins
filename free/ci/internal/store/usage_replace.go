package store

import (
	"context"
	"database/sql"
)

// ReplaceUsage atomically replaces the complete inclusive UTC date range for a provider.
func (s *Store) ReplaceUsage(ctx context.Context, provider, from, to string, rows []UsageRow) error {
	if provider == "" || !ValidDay(from) || !ValidDay(to) || from > to {
		return coded("E607", "invalid usage replacement range")
	}
	for _, u := range rows {
		if u.Key == "" || u.Provider != provider || u.Day < from || u.Day > to || !ValidDay(u.Day) || u.Period != u.Day[:7] || u.Minutes < 0 || u.CostMicro < 0 {
			return coded("E607", "usage row is outside replacement range or invalid")
		}
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM ledger_usage WHERE provider=? AND day>=? AND day<=?", provider, from, to); err != nil {
			return err
		}
		for _, u := range rows {
			// A foreign provider's key must never be overwritten by a replacement.
			_, err := tx.ExecContext(ctx, `INSERT INTO ledger_usage(key,provider,period,day,series,minutes,cost_micro,balance_micro,reported_at) VALUES (?,?,?,?,?,?,?,?,?)`, u.Key, u.Provider, u.Period, u.Day, u.Series, u.Minutes, u.CostMicro, u.BalanceMicro, u.ReportedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
