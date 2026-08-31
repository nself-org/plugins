package main

// handlers_ci.go — /v1/ci/events: CI-failure event ingestion.
//
// Purpose: the SaaS equivalent of what self-hosted Sentry Bundle users get
//   from the GitHub-Actions bridge (CI-failure report aggregation). We do
//   NOT host multi-tenant git — tenants POST one event per pipeline run
//   from their own CI (curl on failure()), the gateway stores it and, on
//   status=failure, dispatches an alert through the existing alert-router
//   path (same as monitor-down) so tenants see/get alerted on CI breakage
//   without us running any git infrastructure.
// Routes:  POST /v1/ci/events — ingest one event (API-key or JWT auth)
//          GET  /v1/ci/events — list, tenant-scoped, newest first
// Outputs: {"ci_event":{...}} on ingest, {"ci_events":[...]} on list —
//   enveloped snake_case per the gateway contract.
// Constraints — P0 tenancy: tenant comes ONLY from saas.TenantFrom (verified
//   nsk_ key or session JWT) — never a client-supplied field. Ingest is
//   quota-metered like error events (IncrMonthlyUsage -> 429 past the
//   tenant's monthly cap). Missing gateway table (fresh box, pre-boot) can't
//   happen here — schema_gateway.go creates it unconditionally at boot, so
//   unlike the proxied plugin tables this handler does not probe existence.

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// ciEventDTO is the contract CIEvent shape.
type ciEventDTO struct {
	ID        string `json:"id"`
	Repo      string `json:"repo"`
	Workflow  string `json:"workflow,omitempty"`
	Status    string `json:"status"`
	RunURL    string `json:"run_url,omitempty"`
	SHA       string `json:"sha,omitempty"`
	Title     string `json:"title,omitempty"`
	CreatedAt string `json:"created_at"`
}

// maxCIEvents caps the list response (matches maxErrorGroups convention).
const maxCIEvents = 100

// validCIStatus is the ingest status allowlist (np_saas_ci_events.status).
func validCIStatus(s string) bool {
	return s == "success" || s == "failure" || s == "cancelled"
}

// ciIngestBody is the POST /v1/ci/events request shape. One event per call
// — tenants call this from a `failure()` step in their own CI, so batching
// would just add client-side complexity for no benefit.
type ciIngestBody struct {
	Repo     string `json:"repo"`
	Workflow string `json:"workflow"`
	Status   string `json:"status"`
	RunURL   string `json:"run_url"`
	SHA      string `json:"sha"`
	Title    string `json:"title"`
}

// handleIngestCIEvent — POST /v1/ci/events.
func (g *gateway) handleIngestCIEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := saas.TenantFrom(ctx)

	var body ciIngestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}
	if body.Repo == "" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "repo is required")
		return
	}
	if !validCIStatus(body.Status) {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
			"status must be success, failure, or cancelled")
		return
	}

	// Quota BEFORE the write: same 429 contract as error-event ingest.
	limits, tier, ok, err := saas.EffectiveLimits(ctx, g.db, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "quota lookup failed")
		return
	}
	if ok {
		if qerr, err := saas.IncrMonthlyUsage(ctx, g.db, tenantID, saas.MetricCIEvents, 1, limits.CIEventsMonth, tier); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "quota update failed")
			return
		} else if qerr != nil {
			saas.WriteQuotaThrottled(w, qerr)
			return
		}
	}

	var ev ciEventDTO
	var createdAt time.Time
	err = g.db.QueryRowContext(ctx, `
		INSERT INTO np_saas_ci_events (tenant_id, repo, workflow, status, run_url, sha, title)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, repo, workflow, status, run_url, sha, title, created_at`,
		tenantID, body.Repo, body.Workflow, body.Status, body.RunURL, body.SHA, body.Title,
	).Scan(&ev.ID, &ev.Repo, &ev.Workflow, &ev.Status, &ev.RunURL, &ev.SHA, &ev.Title, &createdAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "ci event insert failed")
		return
	}
	ev.CreatedAt = createdAt.UTC().Format(time.RFC3339)

	// On failure, dispatch an alert through the same path monitor-down uses
	// (best-effort: delivery-plugin outage must not fail the ingest write —
	// the CI event is already durably stored).
	if body.Status == "failure" {
		title := "CI failed: " + body.Repo
		if body.Workflow != "" {
			title = "CI failed: " + body.Repo + " / " + body.Workflow
		}
		reason := body.Title
		if reason == "" {
			reason = "workflow run failed"
		}
		if st, _, err := g.dispatchAlert(ctx, tenantID, alertEvent{
			Title:   title,
			Kind:    "ci.failure",
			State:   "firing",
			Target:  body.Repo,
			Reason:  reason,
			URL:     body.RunURL,
			DedupID: ev.ID,
		}); err != nil || !upstreamOK(st) {
			log.Printf("saas-gateway: ci.failure dispatch for %s: status=%d err=%v", ev.ID, st, err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{"ci_event": ev})
}

// handleListCIEvents — GET /v1/ci/events.
func (g *gateway) handleListCIEvents(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	rows, err := g.db.QueryContext(r.Context(), `
		SELECT id::text, repo, workflow, status, run_url, sha, title, created_at
		FROM np_saas_ci_events
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT `+itoa(maxCIEvents), tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "ci event lookup failed")
		return
	}
	defer rows.Close() //nolint:errcheck

	events := make([]ciEventDTO, 0)
	for rows.Next() {
		var ev ciEventDTO
		var createdAt time.Time
		if err := rows.Scan(&ev.ID, &ev.Repo, &ev.Workflow, &ev.Status, &ev.RunURL,
			&ev.SHA, &ev.Title, &createdAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "ci event scan failed")
			return
		}
		ev.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		events = append(events, ev)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "ci event iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ci_events": events})
}
