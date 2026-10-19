package economics

// Purpose: usage rows from a provider (RecordUsage) and the ledger report
// behind `nself ci policy usage` (Epic D11, X18).
// Inputs:  a Usage row from P7-HOST-10's importer; the store, the spend policy and a clock.
// Outputs: provider usage rows (the only `known` figures); a Report in which
// every money figure carries its confidence and source.
// Constraints: RecordUsage is idempotent by Key; the report never prints an
// allowance, an estimate or an actual without a confidence label; rates,
// ceilings and budget limits are the owner's declaration and are printed as
// such; nothing here calls a billing or pricing API.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Usage is one consumption line a provider reported. Date is the UTC day
// (YYYY-MM-DD); Key makes the write idempotent (account, date, sku, repo).
type Usage struct {
	Key, Provider, Date string
	Minutes, CostMicro  int64
	BalanceMicro        *int64
	ReportedAt          time.Time
}

// RecordUsage stores a provider-reported line with confidence `known`.
func RecordUsage(ctx context.Context, u Usage) error {
	env, ok := envOf(ctx)
	if !ok {
		return refuse(model.AllowanceUnknown, "economics is not bound, so usage cannot be recorded")
	}
	day, err := time.Parse("2006-01-02", u.Date)
	if u.Key == "" || u.Provider == "" || err != nil || u.Minutes < 0 || u.CostMicro < 0 || (u.BalanceMicro != nil && *u.BalanceMicro < 0) {
		return refuse(model.AllowanceUnknown, "a usage line needs a key, a provider, a UTC date and non-negative amounts")
	}
	// A provider in UTC+N may already report tomorrow; anything later is not a usage date.
	if u.Date > dayOf(env.Now().AddDate(0, 0, 1)) {
		return refuse(model.AllowanceUnknown, "usage dated %s is in the future", u.Date)
	}
	at := u.ReportedAt
	if at.IsZero() || at.After(env.Now()) { // a report is never made in the future
		at = env.Now()
	}
	return env.Store.PutUsage(ctx, store.UsageRow{Key: u.Key, Provider: u.Provider, Period: day.Format("2006-01"), Day: u.Date, Minutes: u.Minutes,
		CostMicro: u.CostMicro, BalanceMicro: u.BalanceMicro, ReportedAt: at.UnixNano()})
}

// BudgetLine is one budget scope. Reserved is the worst case held (a bound, so estimated).
type BudgetLine struct {
	Scope     string  `json:"scope"`
	Window    string  `json:"window,omitempty"`
	Limit     Figure  `json:"limit"`
	Reserved  *Figure `json:"reserved,omitempty"`
	Remaining *Figure `json:"remaining,omitempty"`
}

// ProviderLine is one provider's rate, ceiling, consumption and allowance.
type ProviderLine struct {
	Provider       string  `json:"provider"`
	Class          string  `json:"class"`
	RatePerHour    *Figure `json:"rate_per_hour,omitempty"`
	RatePerMinute  *Figure `json:"rate_per_minute,omitempty"`
	Ceiling        *Figure `json:"ceiling,omitempty"`
	Imported       *Figure `json:"imported,omitempty"`
	Local          Figure  `json:"local"`
	Allowance      *Figure `json:"allowance,omitempty"`
	AllowanceKnown bool    `json:"allowance_known"`
}

// AttemptLine is one recent ledger row.
type AttemptLine struct {
	Attempt  string  `json:"attempt"`
	Provider string  `json:"provider"`
	Class    string  `json:"class"`
	Estimate Figure  `json:"estimate"`
	Actual   *Figure `json:"actual,omitempty"`
}

// ApprovalLine is a recorded spend approval.
type ApprovalLine struct {
	Run          string `json:"run"`
	SHA          string `json:"sha"`
	ConfigDigest string `json:"config_digest"`
	Estimate     Figure `json:"estimate"`
	Actor        string `json:"actor"`
}

// Report is the ledger as `ci policy usage` prints it.
type Report struct {
	Currency    string         `json:"currency"`
	Day         string         `json:"day"`
	Month       string         `json:"month"`
	PaidEnabled bool           `json:"paid_enabled"`
	Budgets     []BudgetLine   `json:"budgets"`
	Providers   []ProviderLine `json:"providers"`
	Recent      []AttemptLine  `json:"recent"`
	Approvals   []ApprovalLine `json:"approvals"`
}
