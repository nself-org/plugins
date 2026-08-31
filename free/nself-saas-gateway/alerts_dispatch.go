package main

// alerts_dispatch.go — gateway→alert-router event dispatch.
//
// Purpose: one helper for pushing AlertEvents into nself-alert-router's
//   ingest (POST /api/alerts). The router matches routes AND runs G1 channel
//   delivery (email/webhook/telegram/slack via DispatchAlert) with tier
//   gating + dedup — the gateway only describes the event.
// Inputs:  tenant id + event fields (kind/state/dedup drive templates+dedup).
// Outputs: upstream status + body (callers decide how hard to fail).
// Constraints: X-Hasura-Tenant-Id here is the gateway→plugin INTERNAL hop
//   (resolved tenant, never client input); HMAC-signed when the box ingest
//   secret is configured. Best-effort callers must never block on delivery.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// alertEvent is the ingest payload for /api/alerts (G1 delivery fields).
type alertEvent struct {
	Title    string            `json:"title"`
	Severity string            `json:"severity"`
	Labels   map[string]string `json:"labels,omitempty"`
	Source   string            `json:"source"`
	Kind     string            `json:"kind,omitempty"`     // monitor.down | monitor.up | incident.opened | incident.resolved | ssl.expiring
	Target   string            `json:"target,omitempty"`   // URL/host being watched
	Reason   string            `json:"reason,omitempty"`   // failure detail
	URL      string            `json:"url,omitempty"`      // dashboard deep link
	State    string            `json:"state,omitempty"`    // firing | resolved
	DedupID  string            `json:"dedup_id,omitempty"` // stable id for cooldown
}

// dispatchAlert posts one event to alert-router's ingest for the tenant.
// The router delivers to the tenant's np_alert_channels (DispatchAlert).
func (g *gateway) dispatchAlert(ctx context.Context, tenantID string, ev alertEvent) (int, []byte, error) {
	if ev.Source == "" {
		ev.Source = "nself-saas-gateway"
	}
	if ev.Severity == "" {
		if ev.State == "resolved" {
			ev.Severity = "info"
		} else {
			ev.Severity = "critical"
		}
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.cfg.AlertURL+"/api/alerts", bytesReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if tenantID != "" {
		req.Header.Set("X-Hasura-Tenant-Id", tenantID)
	}
	if g.cfg.AlertIngestHMACSecret != "" {
		mac := hmac.New(sha256.New, []byte(g.cfg.AlertIngestHMACSecret))
		mac.Write(payload) //nolint:errcheck // hash.Write never errors
		req.Header.Set("X-Alertmanager-Hmac", hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := upstreamClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := readAllLimited(resp.Body)
	return resp.StatusCode, body, err
}
