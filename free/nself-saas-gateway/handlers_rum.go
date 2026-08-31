package main

// handlers_rum.go — GET /v1/rum: pageview + web-vitals summary.
//
// Purpose: the SPA's RUM screen headline numbers. The rum plugin owns INGEST
//   (beacons, quota, PII scrub); the gateway serves the tenant-scoped READ
//   summary straight from np_saas_usage (monthly pageview counter) +
//   np_rum_sessions / np_rum_events on the shared ops-Postgres.
// Outputs: {"rum":{"pageviews_month","sessions_total","error_rate",
//   "web_vitals":{"lcp_ms_avg","cls_avg","inp_ms_avg","fcp_ms_avg",
//   "ttfb_ms_avg"}}} — null for vitals with no samples.
// Constraints — P0 tenancy: every query is WHERE tenant_id = $1 with the
//   tenant resolved ONLY from the verified credential. Missing plugin tables
//   (fresh box) → zeros, never 500. All aggregates are best-effort: one
//   failing vital never fails the response.

import (
	"context"
	"net/http"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// rumVitalKeys are the web-vital payload keys the summary aggregates,
// mapped to their response field names (values are averaged).
var rumVitalKeys = []struct{ key, field string }{
	{"lcp", "lcp_ms_avg"},
	{"cls", "cls_avg"},
	{"inp", "inp_ms_avg"},
	{"fcp", "fcp_ms_avg"},
	{"ttfb", "ttfb_ms_avg"},
}

// rumVitalWindow bounds the vitals aggregation lookback.
const rumVitalWindow = 7 * 24 * time.Hour

// avgVital averages one numeric payload key over the tenant's recent vital
// events. Best-effort: any error → nil (rendered as JSON null).
func (g *gateway) avgVital(ctx context.Context, tenantID, key string, since time.Time) *float64 {
	var avg *float64
	// key comes from the compile-time rumVitalKeys list, never user input.
	err := g.db.QueryRowContext(ctx, `
		SELECT AVG((payload->>'`+key+`')::numeric)
		FROM np_rum_events
		WHERE tenant_id = $1
		  AND event_type = 'vital'
		  AND payload ? '`+key+`'
		  AND (payload->>'`+key+`') ~ '^[0-9]+(\.[0-9]+)?$'
		  AND received_at >= $2`, //nolint:gosec // key is a package constant
		tenantID, since).Scan(&avg)
	if err != nil {
		return nil
	}
	return avg
}

// handleRUMSummary — GET /v1/rum.
func (g *gateway) handleRUMSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := saas.TenantFrom(ctx)

	summary := map[string]any{
		"pageviews_month": g.monthlyUsage(ctx, tenantID, saas.MetricRUMPageviews),
		"sessions_total":  int64(0),
		"error_rate":      float64(0),
	}
	vitals := map[string]any{}
	for _, v := range rumVitalKeys {
		vitals[v.field] = nil
	}
	summary["web_vitals"] = vitals

	// Fresh box: rum plugin not migrated yet — counters only.
	var exists bool
	if err := g.db.QueryRowContext(ctx,
		`SELECT to_regclass($1) IS NOT NULL`, "np_rum_sessions").Scan(&exists); err != nil || !exists {
		writeJSON(w, http.StatusOK, map[string]any{"rum": summary})
		return
	}

	var sessions int64
	_ = g.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM np_rum_sessions WHERE tenant_id = $1`,
		tenantID).Scan(&sessions)
	summary["sessions_total"] = sessions

	var totalEvents, errorEvents int64
	_ = g.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM np_rum_events WHERE tenant_id = $1`,
		tenantID).Scan(&totalEvents)
	_ = g.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM np_rum_events WHERE tenant_id = $1 AND event_type = 'error'`,
		tenantID).Scan(&errorEvents)
	if totalEvents > 0 {
		summary["error_rate"] = float64(errorEvents) / float64(totalEvents)
	}

	since := time.Now().UTC().Add(-rumVitalWindow)
	for _, v := range rumVitalKeys {
		if avg := g.avgVital(ctx, tenantID, v.key, since); avg != nil {
			vitals[v.field] = *avg
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"rum": summary})
}
