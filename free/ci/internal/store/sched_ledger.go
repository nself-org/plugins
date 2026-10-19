package store

// Purpose: the store half of the usage ledger (P7-SCHED-07, Epic D11): one
// ledger row per admitted attempt (estimate at admission, actual at finalize),
// the reservation rows the budget step writes inside the admission transaction,
// the day-window high-water mark, and the transaction handle other store files
// use to reserve money in their own commit. Migration set sched-ledger.
// Inputs:  rows built by internal/economics (this file has no pricing logic).
// Outputs: LedgerTx for in-transaction work, Store reads for the world
// contributor and `ci policy usage`.
// Constraints: money is int64 micro-units and every sum saturates, never wraps;
// a reservation belongs to one owner (an attempt or an ephemeral intent) and
// one scope, and the window key it was written under never changes, so a
// settle charges the same window; no foreign key crosses the set.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"math"
	"time"
)

//go:embed migrations/sched-ledger/*.sql
var schedLedgerMigrations embed.FS

func init() {
	if err := Register("sched-ledger", schedLedgerMigrations); err != nil {
		panic(err)
	}
}

// RateSnap is the price a row was made under (rate per RateUnitMs milliseconds).
type RateSnap struct{ RateMicro, RateUnitMs, IncMs, MinMicro, ExtrasMicro int64 }

// LedgerRow is one attempt's usage. ActMs and ActMicro are nil until finalize.
type LedgerRow struct {
	Attempt, Pipeline, Project, Provider, Class string
	Remote                                      bool
	EstMs, EstMicro                             int64
	ActMs, ActMicro                             *int64
	Confidence, BudgetRefs, WindowDay           string
	Rate                                        RateSnap
	AdmittedAt, FinalizedAt                     int64
}

type ledgerQuerier interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// LedgerTx is a write transaction over the ledger tables. A step gets one from
// its AdmitView; other store files wrap their own transaction with ledgerTx.
type LedgerTx struct {
	s   *Store
	ctx context.Context
	tx  *sql.Tx
}

func ledgerTx(s *Store, ctx context.Context, tx *sql.Tx) LedgerTx { return LedgerTx{s, ctx, tx} }

// Ledger is the ledger handle of an admission step; it lives in the admission transaction.
func (v AdmitView) Ledger(ctx context.Context) LedgerTx { return ledgerTx(v.s, ctx, v.tx) }

// LedgerWrite runs fn in one writer transaction.
func (s *Store) LedgerWrite(ctx context.Context, fn func(LedgerTx) error) error {
	return s.write(ctx, func(tx *sql.Tx) error { return fn(ledgerTx(s, ctx, tx)) })
}

// Now is the store clock (unix nanoseconds), so a row and its transaction agree.
func (t LedgerTx) Now() int64 { return t.s.now().UnixNano() }

func satAdd(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64, true
	}
	if b < 0 && a < math.MinInt64-b {
		return math.MinInt64, true
	}
	return a + b, false
}

// PutLedger writes the admission estimate of an attempt.
func (t LedgerTx) PutLedger(r LedgerRow) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT OR REPLACE INTO ledger (attempt_id,pipeline_id,project,provider,class,remote,est_ms,est_micro,act_ms,act_micro,confidence,budget_refs,window_day,
 rate_micro,rate_unit_ms,inc_ms,min_micro,extras_micro,admitted_at,finalized_at) VALUES (?,?,?,?,?,?,?,?,NULL,NULL,?,?,?,?,?,?,?,?,?,NULL)`,
		r.Attempt, r.Pipeline, r.Project, r.Provider, r.Class, r.Remote, r.EstMs, r.EstMicro, r.Confidence, r.BudgetRefs, r.WindowDay,
		r.Rate.RateMicro, r.Rate.RateUnitMs, r.Rate.IncMs, r.Rate.MinMicro, r.Rate.ExtrasMicro, r.AdmittedAt)
	return err
}

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
	day := max(today, high)
	if day != high {
		_, err = t.tx.ExecContext(t.ctx, "INSERT INTO window_high(kind,key) VALUES ('day',?) ON CONFLICT(kind) DO UPDATE SET key=excluded.key", day)
	}
	return day, err
}

// LedgerSettle records the actual of an admitted attempt once: its reservations take
// the actual under the window they were made in. It reports false when the
// attempt has no ledger row or was already settled.
func (s *Store) LedgerSettle(ctx context.Context, attempt string, actMs, actMicro int64, confidence string) (bool, error) {
	changed := false
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE ledger SET act_ms=?,act_micro=?,confidence=?,finalized_at=? WHERE attempt_id=? AND finalized_at IS NULL", actMs, actMicro, confidence, s.now().UnixNano(), attempt)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		changed = true
		return settleReservations(ctx, tx, attempt, actMicro)
	})
	return changed, err
}

func settleReservations(ctx context.Context, tx *sql.Tx, owner string, actual int64) error {
	_, err := tx.ExecContext(ctx, "UPDATE reservation SET settled=1, actual_micro=MIN(micro,?) WHERE owner=? AND settled=0", actual, owner)
	return err
}

const ledgerCols = `attempt_id,pipeline_id,project,provider,class,remote,est_ms,est_micro,act_ms,act_micro,confidence,budget_refs,window_day,rate_micro,rate_unit_ms,inc_ms,min_micro,extras_micro,admitted_at,coalesce(finalized_at,0)`

func scanLedger(r interface{ Scan(...any) error }) (LedgerRow, error) {
	var l LedgerRow
	var am, au sql.NullInt64
	err := r.Scan(&l.Attempt, &l.Pipeline, &l.Project, &l.Provider, &l.Class, &l.Remote, &l.EstMs, &l.EstMicro, &am, &au, &l.Confidence, &l.BudgetRefs, &l.WindowDay,
		&l.Rate.RateMicro, &l.Rate.RateUnitMs, &l.Rate.IncMs, &l.Rate.MinMicro, &l.Rate.ExtrasMicro, &l.AdmittedAt, &l.FinalizedAt)
	if am.Valid {
		l.ActMs = &am.Int64
	}
	if au.Valid {
		l.ActMicro = &au.Int64
	}
	return l, err
}

// LedgerAttempt reads one attempt's ledger row.
func (s *Store) LedgerAttempt(ctx context.Context, attempt string) (LedgerRow, bool, error) {
	l, err := scanLedger(s.readers.QueryRowContext(ctx, "SELECT "+ledgerCols+" FROM ledger WHERE attempt_id=?", attempt))
	if errors.Is(err, sql.ErrNoRows) {
		return LedgerRow{}, false, nil
	}
	return l, err == nil, err
}

// LedgerRecent lists the newest limit ledger rows, newest first.
func (s *Store) LedgerRecent(ctx context.Context, limit int) ([]LedgerRow, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT "+ledgerCols+" FROM ledger ORDER BY admitted_at DESC, attempt_id DESC LIMIT ?", limit)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []LedgerRow
	for rows.Next() {
		l, err := scanLedger(rows)
		if err != nil {
			return nil, coded("E607", err.Error())
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func ledgerSince(ctx context.Context, q ledgerQuerier, provider, month string, sinceNs int64) (int64, error) {
	rows, err := q.QueryContext(ctx, "SELECT coalesce(act_micro,est_micro) FROM ledger WHERE provider=? AND substr(window_day,1,7)=? AND admitted_at>?", provider, month, sinceNs)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	var sum int64
	for rows.Next() {
		var v int64
		if err = rows.Scan(&v); err != nil {
			return 0, err
		}
		sum, _ = satAdd(sum, v)
	}
	return sum, rows.Err()
}

// AttemptSpan is the start and end the coordinator recorded for an attempt (unix
// nanoseconds); a zero value means it was not recorded.
func (s *Store) AttemptSpan(ctx context.Context, attempt string) (start, end int64, err error) {
	var a, b sql.NullInt64
	err = s.readers.QueryRowContext(ctx, "SELECT started_at,ended_at FROM attempt WHERE id=?", attempt).Scan(&a, &b)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return a.Int64, b.Int64, err
}

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
	return out, rows.Err()
}
