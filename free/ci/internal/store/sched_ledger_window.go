package store

// Purpose: the day-window high-water mark of the usage ledger (P7-SCHED-07, Epic
// D11, review F5): the UTC day key a reservation is charged to is the later of
// the clock's date and the highest key recorded, so a clock stepped back keeps
// charging the later window.
// Inputs:  the dispatcher's wall-clock date.
// Outputs: the day key, recorded.
// Constraints: a recorded key that is not a date, or more than windowSkewDays
// past today, is a clock that jumped forward once or a damaged row; it is
// replaced by today and the replacement is audited, so one bad reading cannot
// pin every later admission to a day that never ends.

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Window returns the UTC day key to charge: today, or the highest day key ever
// recorded when the clock stepped back. today must be YYYY-MM-DD. The key is recorded.
func (t LedgerTx) Window(today string) (string, error) {
	if _, err := time.Parse("2006-01-02", today); err != nil {
		return "", coded("E607", "window key must be a UTC date")
	}
	var high string
	err := t.tx.QueryRowContext(t.ctx, "SELECT key FROM window_high WHERE kind='day'").Scan(&high)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	// A mark that is not a date, or lies far past today, is a clock that jumped forward once
	// (or a damaged row): it would pin every later admission to that day for good. It is
	// replaced by today and the replacement is audited. A clock stepped back by hours or a
	// day or two (the case the mark exists for) is far below the tolerance.
	if high != "" && (!ValidDay(high) || high > addDays(today, windowSkewDays)) {
		if err = t.s.appendAudit(t.ctx, t.tx, "coordinator", "ledger.window.reset", "window_high", "recorded="+quote(high)+" today="+today); err != nil {
			return "", err
		}
		high = ""
	}
	day := max(today, high)
	if _, err = t.tx.ExecContext(t.ctx, "INSERT INTO window_high(kind,key) VALUES ('day',?) ON CONFLICT(kind) DO UPDATE SET key=excluded.key WHERE key<>excluded.key", day); err != nil {
		return "", err
	}
	return day, nil
}

// windowSkewDays is how far ahead of today the recorded day key may be before it is
// treated as a clock that jumped forward.
const windowSkewDays = 3

// ValidDay reports whether s is a UTC date key (YYYY-MM-DD).
func ValidDay(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil && len(s) == 10
}

func addDays(day string, n int) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

func quote(s string) string {
	if len(s) > 40 {
		s = s[:40]
	}
	return fmt.Sprintf("%q", s)
}
