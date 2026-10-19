package store

// Purpose: reservations, ephemeral resources, provider ownership and provider
// usage rows of the usage ledger (P7-SCHED-07, Epic D11/D12). Same migration
// set as sched_ledger.go.
// Inputs:  calls from internal/economics inside an admission or burst transaction.
// Outputs: sums per scope and window, the allowance inputs of a provider.
// Constraints: a reservation is written once per (owner, scope); a sum counts
// the actual of a settled reservation and the reserved amount otherwise, in Go
// with saturating addition, because SQLite's sum() errors on overflow; the
// provider usage rows are the only `known` figures and are idempotent by key.

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Reservation scopes.
const (
	ScopeJob      = "job"
	ScopePipeline = "pipeline"
	ScopeDay      = "day"
	ScopeMonth    = "month"
	ScopeProvider = "provider"
)

// Reservation is money held against one budget scope. ScopeKey names the
// attempt (job), the pipeline or the provider ("" for day and month); WindowKey
// is the UTC day, the UTC month, or "" for a scope with no window.
type Reservation struct {
	Owner, Scope, ScopeKey, WindowKey string
	Micro                             int64
}

// Reserve writes a reservation. An owner holds each scope once: a repeat of the
// same admission, or the re-admission of an attempt whose lease expired, replaces
// the owner's unsettled reservation (new amount, new window) and never adds a
// second one; a settled reservation is final.
func (t LedgerTx) Reserve(r Reservation) error {
	_, err := t.tx.ExecContext(t.ctx, `INSERT INTO reservation(owner,scope,scope_key,window_key,micro,settled) VALUES (?,?,?,?,?,0)
 ON CONFLICT(owner,scope) DO UPDATE SET scope_key=excluded.scope_key, window_key=excluded.window_key, micro=excluded.micro WHERE settled=0`, r.Owner, r.Scope, r.ScopeKey, r.WindowKey, r.Micro)
	return err
}

// Reserved sums the money held in one scope and window (saturating).
func (t LedgerTx) Reserved(scope, scopeKey, windowKey string) (int64, error) {
	return reserved(t.ctx, t.tx, scope, scopeKey, windowKey, "")
}

// ReservedExcept is Reserved without the money held by owner, for a check that
// is about to replace that owner's own reservation.
func (t LedgerTx) ReservedExcept(scope, scopeKey, windowKey, owner string) (int64, error) {
	return reserved(t.ctx, t.tx, scope, scopeKey, windowKey, owner)
}

// Reserved is the same sum outside a transaction.
func (s *Store) Reserved(ctx context.Context, scope, scopeKey, windowKey string) (int64, error) {
	return reserved(ctx, s.readers, scope, scopeKey, windowKey, "")
}

func reserved(ctx context.Context, q ledgerQuerier, scope, scopeKey, windowKey, except string) (int64, error) {
	rows, err := q.QueryContext(ctx, "SELECT CASE WHEN settled=1 THEN actual_micro ELSE micro END FROM reservation WHERE scope=? AND scope_key=? AND window_key=? AND owner<>?", scope, scopeKey, windowKey, except)
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

// WindowHigh is the highest day key recorded ("" when none).
func (s *Store) WindowHigh(ctx context.Context) (string, error) {
	var k string
	err := s.readers.QueryRowContext(ctx, "SELECT key FROM window_high WHERE kind='day'").Scan(&k)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !ValidDay(k)) {
		return "", nil // a damaged mark is no mark; the next admission replaces it
	}
	return k, err
}

// ProviderOwner returns the coordinator that owns a paid provider account. The
// first coordinator to ask with a non-empty claim becomes the owner.
func (t LedgerTx) ProviderOwner(provider, claim string) (string, error) {
	if claim != "" {
		if _, err := t.tx.ExecContext(t.ctx, "INSERT OR IGNORE INTO provider_account(id,owner_coordinator) VALUES (?,?)", provider, claim); err != nil {
			return "", err
		}
	}
	var owner string
	err := t.tx.QueryRowContext(t.ctx, "SELECT owner_coordinator FROM provider_account WHERE id=?", provider).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return owner, err
}

// ProviderOwners lists the recorded owners (provider id to coordinator id).
func (s *Store) ProviderOwners(ctx context.Context) (map[string]string, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT id,owner_coordinator FROM provider_account")
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, owner string
		if err = rows.Scan(&id, &owner); err != nil {
			return nil, coded("E607", err.Error())
		}
		out[id] = owner
	}
	return out, rows.Err()
}

// UsageRow is a figure a provider reported (P7-HOST-10's importer); it is the
// only `known` figure. Key makes the write idempotent (account, date, sku, repo).
type UsageRow struct {
	Key, Provider, Period, Day string
	Minutes, CostMicro         int64
	BalanceMicro               *int64
	ReportedAt                 int64
}

// PutUsage stores or replaces a provider usage row.
func (s *Store) PutUsage(ctx context.Context, u UsageRow) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO ledger_usage(key,provider,period,day,minutes,cost_micro,balance_micro,reported_at) VALUES (?,?,?,?,?,?,?,?)",
			u.Key, u.Provider, u.Period, u.Day, u.Minutes, u.CostMicro, u.BalanceMicro, u.ReportedAt)
		return err
	})
}

// AllowanceInputs are the figures an allowance is made of for one provider and UTC month.
type AllowanceInputs struct {
	// Imported is the provider-reported consumption of the month (saturating); HasImport says there is any.
	Imported  int64
	HasImport bool
	// Balance is the newest provider-reported balance, if the provider reports one.
	Balance *int64
	// Local is what the local ledger spent after the newest import (before it, the import already counts it).
	Local int64
}

// Allowance reads the allowance inputs outside a transaction.
func (s *Store) Allowance(ctx context.Context, provider, month string) (AllowanceInputs, error) {
	return allowance(ctx, s.readers, provider, month)
}

// Allowance reads the allowance inputs inside the transaction.
func (t LedgerTx) Allowance(provider, month string) (AllowanceInputs, error) {
	return allowance(t.ctx, t.tx, provider, month)
}

func allowance(ctx context.Context, q ledgerQuerier, provider, month string) (AllowanceInputs, error) {
	var a AllowanceInputs
	rows, err := q.QueryContext(ctx, "SELECT cost_micro,balance_micro,day,reported_at FROM ledger_usage WHERE provider=? AND period=? ORDER BY reported_at,key", provider, month)
	if err != nil {
		return a, err
	}
	var newestDay string
	var newestReport int64
	for rows.Next() {
		var cost, reportedAt int64
		var bal sql.NullInt64
		var day string
		if err = rows.Scan(&cost, &bal, &day, &reportedAt); err != nil {
			_ = rows.Close()
			return a, err
		}
		newestReport = max(newestReport, reportedAt)
		a.HasImport = true
		a.Imported, _ = satAdd(a.Imported, cost)
		newestDay = max(newestDay, day)
		if bal.Valid {
			v := bal.Int64
			a.Balance = &v
		}
	}
	if err = rows.Close(); err != nil {
		return a, err
	}
	// Provider usage lags by hours, so the local ledger counts from the start of the newest
	// day the import covers, and never from a day later than the day before the newest
	// report was made (a row dated in the future must not make local spend disappear): a
	// little overlap over-counts, a gap would not.
	if newestReport > 0 {
		newestDay = min(newestDay, time.Unix(0, newestReport).UTC().AddDate(0, 0, -1).Format("2006-01-02"))
	}
	a.Local, err = ledgerSinceDay(ctx, q, provider, month, newestDay)
	return a, err
}

// UsageProviders lists the providers that reported usage for a UTC month.
func (s *Store) UsageProviders(ctx context.Context, month string) ([]string, error) {
	rows, err := s.readers.QueryContext(ctx, "SELECT DISTINCT provider FROM ledger_usage WHERE period=? ORDER BY provider", month)
	if err != nil {
		return nil, coded("E607", err.Error())
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, coded("E607", err.Error())
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
