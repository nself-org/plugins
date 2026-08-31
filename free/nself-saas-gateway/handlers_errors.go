package main

// handlers_errors.go — /v1/errors: error-tracking reads over the nself-errors
// plugin's tables.
//
// Purpose: the SPA's Errors screen. The errors plugin owns INGEST (Sentry
//   envelopes, quota, grouping); the gateway serves the tenant-scoped READ
//   surface straight from np_errors_groups / np_errors_events on the shared
//   ops-Postgres (same box), so no per-plugin read API is needed.
// Routes:  GET  /v1/errors            — error groups, ?status= filter
//          GET  /v1/errors/{id}       — one group + recent events
//          POST /v1/errors/{id}/resolve — mark a group resolved
// Outputs: {"errors":[...]}, {"error_group":{...}} — enveloped snake_case.
// Constraints — P0 tenancy: every query is WHERE tenant_id = $1 with the
//   tenant resolved ONLY from the verified credential (saas middleware);
//   a group belonging to another tenant is indistinguishable from a missing
//   one (404). Missing plugin tables (fresh box) → empty list, never 500.

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// errorGroupDTO is the contract ErrorGroup shape.
type errorGroupDTO struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Title      string `json:"title"`
	Level      string `json:"level"`
	Status     string `json:"status"`
	EventCount int64  `json:"event_count"`
	FirstSeen  string `json:"first_seen"`
	LastSeen   string `json:"last_seen"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

// errorEventDTO is one recent event inside a group detail.
type errorEventDTO struct {
	ID             string `json:"id"`
	Message        string `json:"message"`
	Level          string `json:"level"`
	ExceptionType  string `json:"exception_type,omitempty"`
	ExceptionValue string `json:"exception_value,omitempty"`
	Environment    string `json:"environment,omitempty"`
	Release        string `json:"release,omitempty"`
	ReceivedAt     string `json:"received_at"`
}

// maxErrorGroups caps the group list response.
const maxErrorGroups = 100

// maxGroupEvents caps the recent-events list in a group detail.
const maxGroupEvents = 20

// validErrorStatus is the ?status= filter allowlist (np_errors_groups.status).
func validErrorStatus(s string) bool {
	return s == "unresolved" || s == "resolved" || s == "ignored"
}

// scanErrorGroup reads one np_errors_groups row into the DTO.
func scanErrorGroup(scan func(dest ...any) error) (errorGroupDTO, error) {
	var g errorGroupDTO
	var firstSeen, lastSeen time.Time
	var resolvedAt sql.NullTime
	if err := scan(&g.ID, &g.ProjectID, &g.Title, &g.Level, &g.Status,
		&g.EventCount, &firstSeen, &lastSeen, &resolvedAt); err != nil {
		return g, err
	}
	g.FirstSeen = firstSeen.UTC().Format(time.RFC3339)
	g.LastSeen = lastSeen.UTC().Format(time.RFC3339)
	if resolvedAt.Valid {
		g.ResolvedAt = resolvedAt.Time.UTC().Format(time.RFC3339)
	}
	return g, nil
}

// errorGroupCols is the shared SELECT column list for np_errors_groups.
const errorGroupCols = `id::text, project_id, title, level, status,
	event_count, first_seen, last_seen, resolved_at`

// handleListErrors — GET /v1/errors?status=.
func (g *gateway) handleListErrors(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())

	// Fresh box: the errors plugin may not be migrated yet — empty list.
	var exists bool
	if err := g.db.QueryRowContext(r.Context(),
		`SELECT to_regclass($1) IS NOT NULL`, "np_errors_groups").Scan(&exists); err != nil || !exists {
		writeJSON(w, http.StatusOK, map[string]any{"errors": []errorGroupDTO{}})
		return
	}

	q := `SELECT ` + errorGroupCols + `
		FROM np_errors_groups
		WHERE tenant_id = $1`
	args := []any{tenantID}
	if s := r.URL.Query().Get("status"); s != "" {
		if !validErrorStatus(s) {
			writeErr(w, http.StatusUnprocessableEntity, "invalid_request",
				"status must be unresolved, resolved, or ignored")
			return
		}
		q += ` AND status = $2`
		args = append(args, s)
	}
	q += ` ORDER BY last_seen DESC LIMIT ` + itoa(maxErrorGroups)

	rows, err := g.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "error group lookup failed")
		return
	}
	defer rows.Close() //nolint:errcheck

	groups := make([]errorGroupDTO, 0)
	for rows.Next() {
		grp, err := scanErrorGroup(rows.Scan)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db_error", "error group scan failed")
			return
		}
		groups = append(groups, grp)
	}
	if rows.Err() != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "error group iteration failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"errors": groups})
}

// handleGetError — GET /v1/errors/{id}: one group + recent events.
func (g *gateway) handleGetError(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		// A non-UUID id cannot exist — same 404 as an unknown one.
		writeErr(w, http.StatusNotFound, "not_found", "error group not found")
		return
	}

	grp, err := scanErrorGroup(g.db.QueryRowContext(r.Context(),
		`SELECT `+errorGroupCols+`
		FROM np_errors_groups
		WHERE id = $1 AND tenant_id = $2`, id, tenantID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not_found", "error group not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "error group lookup failed")
		return
	}

	events := make([]errorEventDTO, 0, maxGroupEvents)
	rows, err := g.db.QueryContext(r.Context(), `
		SELECT id::text, COALESCE(message, ''), level,
		       COALESCE(exception_type, ''), COALESCE(exception_value, ''),
		       COALESCE(environment, ''), COALESCE(release_version, ''), received_at
		FROM np_errors_events
		WHERE group_id = $1 AND tenant_id = $2
		ORDER BY received_at DESC
		LIMIT `+itoa(maxGroupEvents), id, tenantID)
	if err == nil {
		defer rows.Close() //nolint:errcheck
		for rows.Next() {
			var e errorEventDTO
			var receivedAt time.Time
			if err := rows.Scan(&e.ID, &e.Message, &e.Level, &e.ExceptionType,
				&e.ExceptionValue, &e.Environment, &e.Release, &receivedAt); err != nil {
				break // best-effort: group detail still renders without events
			}
			e.ReceivedAt = receivedAt.UTC().Format(time.RFC3339)
			events = append(events, e)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"error_group": grp,
		"events":      events,
	})
}

// handleResolveError — POST /v1/errors/{id}/resolve.
func (g *gateway) handleResolveError(w http.ResponseWriter, r *http.Request) {
	tenantID := saas.TenantFrom(r.Context())
	id := chi.URLParam(r, "id")
	if uuid.Validate(id) != nil {
		writeErr(w, http.StatusNotFound, "not_found", "error group not found")
		return
	}

	grp, err := scanErrorGroup(g.db.QueryRowContext(r.Context(), `
		UPDATE np_errors_groups
		SET status = 'resolved', resolved_at = NOW()
		WHERE id = $1 AND tenant_id = $2
		RETURNING `+errorGroupCols, id, tenantID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "not_found", "error group not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "error group resolve failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"error_group": grp})
}

// itoa builds LIMIT clauses from compile-time constants (never user input).
func itoa(n int) string { return strconv.Itoa(n) }
