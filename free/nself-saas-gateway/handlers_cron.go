package main

// handlers_cron.go — /v1/cron: heartbeat (cron) monitors.
//
// Purpose: the SPA's Heartbeats screen + a simple authenticated ping path.
//   The cron-monitor plugin owns the scheduler (missed-beat detection); the
//   gateway serves the tenant-scoped list and records check-ins straight in
//   np_cron_monitors / np_cron_check_ins on the shared ops-Postgres.
// Routes:  GET  /v1/cron                — list heartbeat monitors + status
//          POST /v1/cron/{id}/heartbeat — record a ping (body optional:
//            {"status":"ok"|"error","duration_ms":N})
// Outputs: {"heartbeats":[...]}, {"heartbeat":{...}} — enveloped snake_case.
// Constraints — P0 tenancy: ownership is verified with
//   WHERE id = $1 AND tenant_id = $2 before any write; another tenant's
//   monitor id 404s identically to a missing one. Missing plugin tables
//   (fresh box) → empty list, never 500.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// heartbeatDTO is the contract HeartbeatMonitor shape.
type heartbeatDTO struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Schedule       string `json:"schedule"`
	GraceMinutes   int    `json:"grace_period_minutes"`
	MonitorStatus  string `json:"monitor_status"`         // active | paused
	LastStatus     string `json:"last_status,omitempty"`  // ok | error | missed
	LastCheckInAt  string `json:"last_check_in_at,omitempty"`
	NextExpectedAt string `json:"next_expected_at,omitempty"`
}

// handleListCron — GET /v1/cron.
func (g *gateway) handleListCron(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	var exists bool
	if err := g.db.QueryRowContext(r.Context(),
		`SELECT to_regclass($1) IS NOT NULL`, "np_cron_monitors").Scan(&exists); err != nil || !exists {
		writeJSON(w, http.StatusOK, map[string]any{"heartbeats": []heartbeatDTO{}})
		return
	}

	rows, err := g.db.QueryContext(r.Context(), `
		SELECT id::text, name, slug, schedule, grace_period_minutes,
		       COALESCE(monitor_status, 'active'), COALESCE(last_status, ''),
		       last_check_in_at, next_expected_at
		FROM np_cron_monitors
		WHERE tenant_id = $1
		ORDER BY created_at ASC`, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "heartbeat lookup failed")
		return
	}
	defer rows.Close() //nolint:errcheck

	monitors := make([]heartbeatDTO, 0)
	for rows.Next() {
		var m heartbeatDTO
		var lastCheckIn, nextExpected sql.NullTime
		if err := rows.Scan(&m.ID, &m.Name, &m.Slug, &m.Schedule, &m.GraceMinutes,
			&m.MonitorStatus, &m.LastStatus, &lastCheckIn, &nextExpected); err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "heartbeat scan failed")
			return
		}
		if lastCheckIn.Valid {
			m.LastCheckInAt = lastCheckIn.Time.UTC().Format(time.RFC3339)
		}
		if nextExpected.Valid {
			m.NextExpectedAt = nextExpected.Time.UTC().Format(time.RFC3339)
		}
		monitors = append(monitors, m)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "heartbeat iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"heartbeats": monitors})
}

// handleCronHeartbeat — POST /v1/cron/{id}/heartbeat.
func (g *gateway) handleCronHeartbeat(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "heartbeat monitor not found")
		return
	}

	// Optional body: default is a plain OK ping.
	ping := struct {
		Status     string `json:"status"`
		DurationMs *int64 `json:"duration_ms"`
	}{Status: "ok"}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&ping); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
			return
		}
	}
	if ping.Status == "" {
		ping.Status = "ok"
	}
	if ping.Status != "ok" && ping.Status != "error" {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", `status must be "ok" or "error"`)
		return
	}

	// P0: ownership check BEFORE any write — another tenant's monitor 404s.
	var monitorID string
	err := g.db.QueryRowContext(r.Context(),
		`SELECT id::text FROM np_cron_monitors WHERE id = $1 AND tenant_id = $2`,
		id, tenantID).Scan(&monitorID)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not_found", "heartbeat monitor not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "heartbeat monitor lookup failed")
		return
	}

	if _, err := g.db.ExecContext(r.Context(), `
		INSERT INTO np_cron_check_ins (monitor_id, status, duration_ms, tenant_id)
		VALUES ($1, $2, $3, $4)`, monitorID, ping.Status, ping.DurationMs, tenantID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "check-in insert failed")
		return
	}
	if _, err := g.db.ExecContext(r.Context(), `
		UPDATE np_cron_monitors
		SET last_check_in_at = NOW(), last_status = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3`, ping.Status, monitorID, tenantID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "monitor update failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"heartbeat": map[string]any{
			"monitor_id":  monitorID,
			"status":      ping.Status,
			"recorded_at": time.Now().UTC().Format(time.RFC3339),
		},
	})
}
