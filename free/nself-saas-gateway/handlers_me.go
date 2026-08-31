package main

// handlers_me.go — GET /v1/me: identity, tier, and quota usage.
//
// Purpose: answer "who am I and how much of my plan have I used" straight
//   from the np_saas_* control tables + per-plugin resource tables (the
//   gateway shares the box's ops-Postgres).
// Outputs: {"tenant_id","email","tier","quotas":{dim:{"used","limit"}}}.
// Constraints: resource tables belong to plugins that may not be migrated
//   yet on a fresh box — missing tables count as used=0 (best-effort reads,
//   never a 500 for a cosmetic counter).

import (
	"context"
	"net/http"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// quotaUsageDTO is the contract QuotaUsage shape.
type quotaUsageDTO struct {
	Used  int64 `json:"used"`
	Limit int64 `json:"limit"`
}

// countIfExists counts tenant rows in a table, treating a missing table
// (fresh box, plugin not yet migrated) as zero. Existence is probed first
// with to_regclass because Postgres rejects a query referencing an unknown
// table at parse time — a CASE cannot guard it.
func (g *gateway) countIfExists(ctx context.Context, table, tenantID string) int64 {
	var exists bool
	if err := g.db.QueryRowContext(ctx,
		`SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
		return 0
	}
	var used int64
	// table is a compile-time constant in every caller (never user input).
	err := g.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM `+table+` WHERE tenant_id = $1`, //nolint:gosec
		tenantID).Scan(&used)
	if err != nil {
		return 0
	}
	return used
}

// monthlyUsage reads the current-period ingest counter for a metric.
func (g *gateway) monthlyUsage(ctx context.Context, tenantID, metric string) int64 {
	var used int64
	err := g.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(used), 0) FROM np_saas_usage
		WHERE tenant_id = $1 AND metric = $2 AND period = $3`,
		tenantID, metric, saas.MonthKey(time.Now())).Scan(&used)
	if err != nil {
		return 0
	}
	return used
}

// quotaUsage builds the per-dimension used/limit map shared by /v1/me and
// /v1/billing. Best-effort reads: missing plugin tables count as used=0.
func (g *gateway) quotaUsage(ctx context.Context, tenantID string, limits saas.Limits) map[string]quotaUsageDTO {
	return map[string]quotaUsageDTO{
		"monitors": {
			Used:  g.countIfExists(ctx, "np_uptime_targets", tenantID),
			Limit: int64(limits.Monitors),
		},
		"heartbeats": {
			Used:  g.countIfExists(ctx, "np_cron_monitors", tenantID),
			Limit: int64(limits.Heartbeats),
		},
		"status_pages": {
			Used:  g.countIfExists(ctx, "np_saas_status_pages", tenantID),
			Limit: int64(limits.StatusPages),
		},
		"seats": {
			// The owner always occupies seat #1 (not a team_members row).
			Used:  1 + g.countIfExists(ctx, "np_saas_team_members", tenantID),
			Limit: int64(limits.Seats),
		},
		"error_events_month": {
			Used:  g.monthlyUsage(ctx, tenantID, saas.MetricErrorEvents),
			Limit: limits.ErrorEventsMonth,
		},
		"rum_pageviews_month": {
			Used:  g.monthlyUsage(ctx, tenantID, saas.MetricRUMPageviews),
			Limit: limits.RUMPageviewsMonth,
		},
		"ci_events_month": {
			Used:  g.monthlyUsage(ctx, tenantID, saas.MetricCIEvents),
			Limit: limits.CIEventsMonth,
		},
	}
}

// handleUsage — GET /v1/usage: tier + per-dimension used/limit. Same map as
// /v1/me and /v1/billing (monitors, heartbeats, status_pages, seats,
// error_events_month, rum_pageviews_month, ci_events_month) under its own
// contract envelope.
func (g *gateway) handleUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := saas.TenantFrom(ctx)

	tenant, err := saas.GetTenant(ctx, g.db, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "tenant lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"usage": map[string]any{
			"tier":   tenant.Tier,
			"quotas": g.quotaUsage(ctx, tenantID, tenant.LimitsFor()),
		},
	})
}

// handleMe — GET /v1/me.
func (g *gateway) handleMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := saas.TenantFrom(ctx)

	tenant, err := saas.GetTenant(ctx, g.db, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "tenant lookup failed")
		return
	}
	limits := tenant.LimitsFor()

	var email string
	_ = g.db.QueryRowContext(ctx,
		`SELECT COALESCE(owner_email, '') FROM np_saas_tenants WHERE tenant_id = $1`,
		tenantID).Scan(&email)

	quotas := g.quotaUsage(ctx, tenantID, limits)

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"email":     email,
		"tier":      tenant.Tier,
		"quotas":    quotas,
	})
}
