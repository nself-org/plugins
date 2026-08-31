package main

// detector.go — DOWN-DETECTOR: uptime results → incidents + alerts.
//
// Purpose: close the monitoring loop. The uptime plugin probes and stores
//   results; this background sweep watches those results per cloud tenant:
//     - a monitor DOWN for >= threshold consecutive checks (default 2) opens
//       a tenant-scoped incident (nself-incident-mgmt) and fires a
//       monitor.down/firing alert through alert-router (G1 DispatchAlert →
//       the tenant's np_alert_channels: email/webhook/telegram/slack);
//     - recovery resolves that incident and fires monitor.up/resolved.
//   A companion daily sweep alerts when a monitor's TLS certificate has
//   fewer than cfg.SSLExpiryDays days remaining (ssl.expiring/firing).
// Inputs:  np_uptime_targets + np_uptime_results (+ np_uptime_tls) via the
//   shared ops-Postgres; incident/alert plugins over loopback HTTP.
// Outputs: np_saas_monitor_state rows (idempotency), incidents, alerts.
// Constraints: IDEMPOTENT — state rows gate transitions so one outage opens
//   exactly one incident; alert-router dedup (DedupID=monitor id) is the
//   second belt. Tenant identity flows ONLY from the target row's tenant_id
//   (never a request header). Single-instance loop per box.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/lib/pq"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// monitorSweepRow is one enabled cloud monitor with its recent probe statuses
// (newest first) and current detector state.
type monitorSweepRow struct {
	ID         string
	TenantID   string
	Name       string
	URL        string
	Statuses   []string // newest-first probe statuses, up to threshold+1
	State      string   // np_saas_monitor_state.status ("" when no row yet)
	IncidentID string
}

// runDetector loops the down-detector sweep until ctx is cancelled. The SSL
// sweep runs on its own (much slower) cadence inside the same goroutine.
func (g *gateway) runDetector(ctx context.Context) {
	log.Printf("saas-gateway: down-detector running (interval=%s threshold=%d ssl_warn=%dd)",
		g.cfg.DetectorInterval, g.cfg.DownThreshold, g.cfg.SSLExpiryDays)
	ticker := time.NewTicker(g.cfg.DetectorInterval)
	defer ticker.Stop()
	sslTicker := time.NewTicker(g.cfg.SSLCheckInterval)
	defer sslTicker.Stop()

	// One immediate pass so a restart never leaves an outage unnoticed.
	g.sweepMonitors(ctx)
	g.sweepSSLExpiry(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.sweepMonitors(ctx)
		case <-sslTicker.C:
			g.sweepSSLExpiry(ctx)
		}
	}
}

// sweepQuery pulls every enabled cloud monitor with its latest probe statuses
// and any existing detector state in one round trip.
const sweepQuery = `
	SELECT t.id, t.tenant_id::text, t.name, t.url,
	       COALESCE(s.statuses, '{}'), COALESCE(ms.status, ''), COALESCE(ms.incident_id, '')
	FROM np_uptime_targets t
	LEFT JOIN LATERAL (
		SELECT array_agg(x.status ORDER BY x.checked_at DESC) AS statuses
		FROM (
			SELECT status, checked_at FROM np_uptime_results
			WHERE target_id = t.id
			ORDER BY checked_at DESC
			LIMIT $1
		) x
	) s ON true
	LEFT JOIN np_saas_monitor_state ms ON ms.monitor_id = t.id
	WHERE t.enabled = true AND t.tenant_id IS NOT NULL`

// sweepMonitors runs one down-detector pass. Errors are logged, never fatal —
// the next tick retries.
func (g *gateway) sweepMonitors(ctx context.Context) {
	rows, err := g.db.QueryContext(ctx, sweepQuery, g.cfg.DownThreshold+1)
	if err != nil {
		log.Printf("saas-gateway: detector sweep query: %v", err)
		return
	}
	var monitors []monitorSweepRow
	for rows.Next() {
		var m monitorSweepRow
		if err := rows.Scan(&m.ID, &m.TenantID, &m.Name, &m.URL,
			pq.Array(&m.Statuses), &m.State, &m.IncidentID); err != nil {
			log.Printf("saas-gateway: detector sweep scan: %v", err)
			continue
		}
		monitors = append(monitors, m)
	}
	rows.Close() //nolint:errcheck,gosec
	if err := rows.Err(); err != nil {
		log.Printf("saas-gateway: detector sweep rows: %v", err)
	}

	for _, m := range monitors {
		g.applyTransition(ctx, m)
	}
}

// consecutiveDown counts leading "down" results (newest first).
func consecutiveDown(statuses []string) int {
	n := 0
	for _, s := range statuses {
		if s != "down" {
			break
		}
		n++
	}
	return n
}

// applyTransition opens or resolves an incident for one monitor based on its
// probe history and recorded state. Idempotent per state row.
func (g *gateway) applyTransition(ctx context.Context, m monitorSweepRow) {
	fails := consecutiveDown(m.Statuses)
	switch {
	case fails >= g.cfg.DownThreshold && m.State != "down":
		g.markDown(ctx, m, fails)
	case len(m.Statuses) > 0 && m.Statuses[0] == "up" && m.State == "down":
		g.markRecovered(ctx, m)
	}
}

// serviceRequest builds the synthetic request callUpstream needs for
// detector-originated plugin calls: no client headers exist, so the resolved
// tenant in the context makes callUpstream mint a service JWT for exactly
// that tenant.
func serviceRequest(ctx context.Context, tenantID string) *http.Request {
	r, _ := http.NewRequestWithContext(saas.WithTenant(ctx, tenantID),
		http.MethodGet, "http://gateway.internal/", nil)
	return r
}

// monitorLink deep-links the dashboard monitor detail page for alert emails.
func (g *gateway) monitorLink(monitorID string) string {
	return g.cfg.DashboardBaseURL + "/monitors/" + monitorID
}

// markDown opens a tenant-scoped incident and fires monitor.down. The state
// upsert is last so a partial failure retries next tick (alert-router dedup
// suppresses duplicate notifications in the window).
func (g *gateway) markDown(ctx context.Context, m monitorSweepRow, fails int) {
	reason := fmt.Sprintf("%d consecutive failed checks", fails)

	// 1. Open the incident (best-effort: an incident-plugin outage must not
	// block the alert email).
	incidentID := ""
	sr := serviceRequest(ctx, m.TenantID)
	status, body, err := g.callUpstream(sr.Context(), sr, http.MethodPost, g.cfg.IncidentURL,
		"/incidents/", map[string]any{
			"title":       "Monitor down: " + m.Name,
			"description": m.URL + " — " + reason,
			"severity":    "high",
		})
	if err != nil || !upstreamOK(status) {
		log.Printf("saas-gateway: detector open incident for %s: status=%d err=%v", m.ID, status, err)
	} else {
		var inc struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &inc) == nil {
			incidentID = inc.ID
		}
	}

	// 2. Fire the alert through the router (G1 delivery to the tenant's
	// channels; DedupID collapses repeats of this outage).
	if st, _, err := g.dispatchAlert(ctx, m.TenantID, alertEvent{
		Title:   "Monitor down: " + m.Name,
		Kind:    "monitor.down",
		State:   "firing",
		Target:  m.URL,
		Reason:  reason,
		URL:     g.monitorLink(m.ID),
		DedupID: m.ID,
	}); err != nil || !upstreamOK(st) {
		log.Printf("saas-gateway: detector monitor.down dispatch for %s: status=%d err=%v", m.ID, st, err)
	}

	// 3. Email confirmed status-page subscribers (subscriber_notify.go —
	// own per-(subscriber,incident,state) ledger dedup; non-fatal).
	g.notifySubscribers(ctx, m.TenantID, incidentID, "firing",
		"Monitor down: "+m.Name, m.URL+" — "+reason, g.monitorLink(m.ID))

	// 4. Record the transition (idempotency gate for subsequent sweeps).
	if _, err := g.db.ExecContext(ctx, `
		INSERT INTO np_saas_monitor_state (monitor_id, tenant_id, status, consecutive_fails, incident_id, updated_at)
		VALUES ($1, $2, 'down', $3, $4, NOW())
		ON CONFLICT (monitor_id) DO UPDATE SET
			status = 'down', consecutive_fails = $3, incident_id = $4, updated_at = NOW()`,
		m.ID, m.TenantID, fails, incidentID); err != nil {
		log.Printf("saas-gateway: detector state upsert for %s: %v", m.ID, err)
	}
}

// markRecovered resolves the outage incident and fires monitor.up.
//
// The incident-mgmt state machine forbids open→resolved (an incident must be
// acknowledged first — internal/state/machine.go ValidTransitions), so the
// detector acknowledges before resolving. The acknowledge is best-effort: a
// human may already have moved the incident to acknowledged/mitigating, in
// which case incident-mgmt returns 409 here and the resolve still succeeds.
func (g *gateway) markRecovered(ctx context.Context, m monitorSweepRow) {
	if m.IncidentID != "" {
		sr := serviceRequest(ctx, m.TenantID)
		if status, _, err := g.callUpstream(sr.Context(), sr, http.MethodPost, g.cfg.IncidentURL,
			"/incidents/"+m.IncidentID+"/acknowledge", nil); err != nil || !upstreamOK(status) {
			// Expected when the incident is already past "open" — non-fatal.
			log.Printf("saas-gateway: detector acknowledge incident %s (pre-resolve, non-fatal): status=%d err=%v",
				m.IncidentID, status, err)
		}
		if status, _, err := g.callUpstream(sr.Context(), sr, http.MethodPost, g.cfg.IncidentURL,
			"/incidents/"+m.IncidentID+"/resolve", nil); err != nil || !upstreamOK(status) {
			log.Printf("saas-gateway: detector resolve incident %s: status=%d err=%v", m.IncidentID, status, err)
		}
	}

	if st, _, err := g.dispatchAlert(ctx, m.TenantID, alertEvent{
		Title:   "Monitor recovered: " + m.Name,
		Kind:    "monitor.up",
		State:   "resolved",
		Target:  m.URL,
		URL:     g.monitorLink(m.ID),
		DedupID: m.ID,
	}); err != nil || !upstreamOK(st) {
		log.Printf("saas-gateway: detector monitor.up dispatch for %s: status=%d err=%v", m.ID, st, err)
	}

	// Email confirmed status-page subscribers that the incident is resolved
	// (ledger-deduped per incident+state; non-fatal).
	g.notifySubscribers(ctx, m.TenantID, m.IncidentID, "resolved",
		"Monitor recovered: "+m.Name, m.URL, g.monitorLink(m.ID))

	if _, err := g.db.ExecContext(ctx, `
		INSERT INTO np_saas_monitor_state (monitor_id, tenant_id, status, consecutive_fails, incident_id, updated_at)
		VALUES ($1, $2, 'up', 0, '', NOW())
		ON CONFLICT (monitor_id) DO UPDATE SET
			status = 'up', consecutive_fails = 0, incident_id = '', updated_at = NOW()`,
		m.ID, m.TenantID); err != nil {
		log.Printf("saas-gateway: detector state upsert for %s: %v", m.ID, err)
	}
}

// sslSweepQuery finds cloud monitors whose latest TLS cert expires inside the
// warning window and that have not been alerted for THIS cert (not_after) yet.
const sslSweepQuery = `
	SELECT tls.target_id, t.tenant_id::text, t.name, t.url, tls.not_after, tls.days_remaining
	FROM np_uptime_tls tls
	JOIN np_uptime_targets t ON t.id = tls.target_id
	LEFT JOIN np_saas_ssl_alerts sa
		ON sa.monitor_id = tls.target_id AND sa.not_after = tls.not_after
	WHERE t.enabled = true
	  AND t.tenant_id IS NOT NULL
	  AND tls.days_remaining >= 0
	  AND tls.days_remaining < $1
	  AND sa.monitor_id IS NULL`

// sweepSSLExpiry alerts once per (monitor, certificate) when the cert has
// fewer than cfg.SSLExpiryDays days remaining. Renewal (new not_after) arms
// the alert again automatically.
func (g *gateway) sweepSSLExpiry(ctx context.Context) {
	rows, err := g.db.QueryContext(ctx, sslSweepQuery, g.cfg.SSLExpiryDays)
	if err != nil {
		log.Printf("saas-gateway: ssl sweep query: %v", err)
		return
	}
	type sslRow struct {
		MonitorID, TenantID, Name, URL string
		NotAfter                       time.Time
		DaysRemaining                  int
	}
	var expiring []sslRow
	for rows.Next() {
		var e sslRow
		if err := rows.Scan(&e.MonitorID, &e.TenantID, &e.Name, &e.URL, &e.NotAfter, &e.DaysRemaining); err != nil {
			log.Printf("saas-gateway: ssl sweep scan: %v", err)
			continue
		}
		expiring = append(expiring, e)
	}
	rows.Close() //nolint:errcheck,gosec
	if err := rows.Err(); err != nil {
		log.Printf("saas-gateway: ssl sweep rows: %v", err)
	}

	for _, e := range expiring {
		if st, _, err := g.dispatchAlert(ctx, e.TenantID, alertEvent{
			Title:    fmt.Sprintf("TLS certificate for %s expires in %d days", e.Name, e.DaysRemaining),
			Severity: "warning",
			Kind:     "ssl.expiring",
			State:    "firing",
			Target:   e.URL,
			Reason: fmt.Sprintf("certificate expires %s (%d days remaining)",
				e.NotAfter.UTC().Format("2006-01-02"), e.DaysRemaining),
			URL:     g.monitorLink(e.MonitorID),
			DedupID: e.MonitorID + "|ssl",
		}); err != nil || !upstreamOK(st) {
			log.Printf("saas-gateway: ssl.expiring dispatch for %s: status=%d err=%v", e.MonitorID, st, err)
			continue // retry next sweep; do not mark alerted
		}
		if _, err := g.db.ExecContext(ctx, `
			INSERT INTO np_saas_ssl_alerts (monitor_id, tenant_id, not_after)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			e.MonitorID, e.TenantID, e.NotAfter); err != nil {
			log.Printf("saas-gateway: ssl alert ledger for %s: %v", e.MonitorID, err)
		}
	}
}
