package main

// handlers_alerts.go — /v1/alerts/channels: alert delivery channels CRUD.
//
// Purpose: manage np_alert_channels — the table G1 delivery (alert-router's
//   DispatchAlert) actually sends through. The gateway writes rows directly
//   on the shared ops-Postgres (migration 004, alert-router); the router
//   reads them at dispatch time, so a created channel is live immediately.
// Routes:  GET    /v1/alerts/channels           — list
//          POST   /v1/alerts/channels           — create (tier-gated)
//          DELETE /v1/alerts/channels/{id}      — delete
//          POST   /v1/alerts/channels/{id}/test — dispatch a test event
// Outputs: {"channels":[...]}, {"channel":{...}}, {"delivered","detail"}.
// Constraints — P0 tenancy: every row is scoped WHERE tenant_id = verified
//   credential; cross-tenant ids 404. TIER GATE (PRD §3 / G1 SaasGate):
//   email channels all tiers; webhook/telegram/slack require bundle+
//   (Limits.WebhookChannels) → 402 otherwise. The test endpoint pushes a
//   real event through alert-router ingest → DispatchAlert, which delivers
//   to the tenant's enabled channels (router-side tier gate + dedup apply).

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// channelDTO is the contract AlertChannel shape.
type channelDTO struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // email | webhook | telegram | slack
	Target    string `json:"target"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at,omitempty"`
}

// validChannelKind mirrors the np_alert_channels kind CHECK constraint.
func validChannelKind(k string) bool {
	switch k {
	case "email", "webhook", "telegram", "slack":
		return true
	}
	return false
}

// validateChannelTarget sanity-checks the target per kind. Deep validation
// (SSRF guards, reachability) happens in alert-router at send time.
func validateChannelTarget(kind, target string) string {
	switch kind {
	case "email":
		if _, err := mail.ParseAddress(target); err != nil {
			return "target must be a valid email address"
		}
	case "webhook", "slack":
		u, err := url.Parse(target)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return "target must be an https:// URL"
		}
	case "telegram":
		if strings.TrimSpace(target) == "" {
			return "target must be a telegram chat id"
		}
	}
	return ""
}

// handleListAlertChannels — GET /v1/alerts/channels.
func (g *gateway) handleListAlertChannels(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	// Fresh box: alert-router may not be migrated yet — empty list.
	var exists bool
	if err := g.db.QueryRowContext(r.Context(),
		`SELECT to_regclass($1) IS NOT NULL`, "np_alert_channels").Scan(&exists); err != nil || !exists {
		writeJSON(w, http.StatusOK, map[string]any{"channels": []channelDTO{}})
		return
	}

	rows, err := g.db.QueryContext(r.Context(), `
		SELECT id::text, kind, target, enabled, created_at
		FROM np_alert_channels
		WHERE tenant_id = $1
		ORDER BY created_at ASC`, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "channel lookup failed")
		return
	}
	defer rows.Close() //nolint:errcheck

	channels := make([]channelDTO, 0)
	for rows.Next() {
		var c channelDTO
		var createdAt time.Time
		if err := rows.Scan(&c.ID, &c.Kind, &c.Target, &c.Enabled, &createdAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "channel scan failed")
			return
		}
		c.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		channels = append(channels, c)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "channel iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

// handleCreateAlertChannel — POST /v1/alerts/channels
// {"kind","target","secret"?,"config"?} (tier-gated for non-email kinds).
func (g *gateway) handleCreateAlertChannel(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	var req struct {
		Kind   string          `json:"kind"`
		Target string          `json:"target"`
		Secret string          `json:"secret"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	req.Target = strings.TrimSpace(req.Target)
	if !validChannelKind(req.Kind) {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
			"kind must be email, webhook, telegram, or slack")
		return
	}
	if msg := validateChannelTarget(req.Kind, req.Target); msg != "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", msg)
		return
	}
	config := req.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	} else if !json.Valid(config) {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "config must be a JSON object")
		return
	}

	// TIER GATE (G1 SaasGate parity): webhook/telegram/slack are bundle+.
	if req.Kind != "email" {
		limits, tier, found, err := saas.EffectiveLimits(r.Context(), g.db, tenantID)
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "quota_unavailable", "tier check unavailable")
			return
		}
		if found && !limits.WebhookChannels {
			writeErr(w, http.StatusPaymentRequired, "quota_exceeded",
				"the "+string(tier)+" tier includes email alerts only — "+
					req.Kind+" channels need the bundle tier. Upgrade at "+saas.UpgradeURL+".")
			return
		}
	}

	var c channelDTO
	var createdAt time.Time
	err := g.db.QueryRowContext(r.Context(), `
		INSERT INTO np_alert_channels (tenant_id, kind, target, secret, config)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5::jsonb)
		RETURNING id::text, kind, target, enabled, created_at`,
		tenantID, req.Kind, req.Target, req.Secret, string(config)).
		Scan(&c.ID, &c.Kind, &c.Target, &c.Enabled, &createdAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "channel create failed")
		return
	}
	c.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusCreated, map[string]any{"channel": c})
}

// handleDeleteAlertChannel — DELETE /v1/alerts/channels/{id}.
func (g *gateway) handleDeleteAlertChannel(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "channel not found")
		return
	}
	res, err := g.db.ExecContext(r.Context(),
		`DELETE FROM np_alert_channels WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "channel delete failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "not_found", "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// handleTestAlertChannel — POST /v1/alerts/channels/{id}/test.
// Dispatches a real test event through alert-router ingest → DispatchAlert.
// NOTE: DispatchAlert fans out to the tenant's enabled channels (the router
// owns per-channel targeting); the unique DedupID defeats cooldown so
// repeated tests always send.
func (g *gateway) handleTestAlertChannel(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "channel not found")
		return
	}

	var kind string
	var enabled bool
	err := g.db.QueryRowContext(r.Context(),
		`SELECT kind, enabled FROM np_alert_channels WHERE id = $1 AND tenant_id = $2`,
		id, tenantID).Scan(&kind, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not_found", "channel not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "channel lookup failed")
		return
	}
	if !enabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"delivered": false, "detail": "channel is disabled — enable it and retry",
		})
		return
	}

	status, body, err := g.dispatchAlert(r.Context(), tenantID, alertEvent{
		Title:    "ɳSentry test alert",
		Severity: "info",
		Kind:     "channel.test",
		State:    "resolved", // renders the calm RECOVERED template
		Reason:   "This is a test notification you requested from the dashboard.",
		URL:      g.cfg.DashboardBaseURL + "/alerts",
		// Unique per call: never suppressed by the delivery cooldown.
		DedupID: "channel-test|" + id + "|" + time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "alert ingest unreachable: "+err.Error())
		return
	}
	if !upstreamOK(status) {
		relayUpstreamError(w, status, body)
		return
	}
	var result struct {
		Delivery struct {
			Sent   int `json:"sent"`
			Failed int `json:"failed"`
			Gated  int `json:"gated"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		writeErr(w, http.StatusBadGateway, "upstream_error", "invalid ingest response")
		return
	}
	delivered := result.Delivery.Sent > 0
	detail := "test event dispatched — sent to " + itoa(result.Delivery.Sent) + " channel(s)"
	if !delivered {
		detail = "test event was not delivered (failed=" + itoa(result.Delivery.Failed) +
			", tier-gated=" + itoa(result.Delivery.Gated) + ") — check the channel target and your tier"
	}
	writeJSON(w, http.StatusOK, map[string]any{"delivered": delivered, "detail": detail})
}
