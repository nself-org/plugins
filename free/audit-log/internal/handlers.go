package internal

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all audit-log endpoints on the given router.
//
//   - secret is the value of PLUGIN_INTERNAL_SECRET; all /events and
//     /admin/events endpoints require a matching X-Plugin-Secret header, except
//     /admin/events which accepts either X-Plugin-Secret or X-Hasura-Admin-Secret.
//   - adminSecret is the value of HASURA_GRAPHQL_ADMIN_SECRET; when non-empty
//     GET /admin/events also accepts requests carrying this header.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool, secret, adminSecret string) {
	r.Get("/health", handleHealth(pool))

	r.Post("/events", handleIngest(pool, secret))
	r.Get("/events", handleList(pool, secret))
	r.Get("/events/{id}", handleGet(pool, secret))
	r.Get("/events/export", handleExport(pool, secret))

	r.Get("/admin/events", handleAdminList(pool, secret, adminSecret))
}

// handleHealth handles GET /health.
// Returns plugin name, version, and a lightweight database connectivity check.
func handleHealth(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := pool.Ping(ctx); err != nil {
			dbStatus = "unreachable"
		}

		status := "ok"
		httpStatus := http.StatusOK
		if dbStatus != "ok" {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		sdk.Respond(w, httpStatus, map[string]string{
			"status":  status,
			"plugin":  "audit-log",
			"version": "1.0.0",
			"db":      dbStatus,
		})
	}
}

// handleIngest handles POST /events.
// This endpoint is for internal use only (e.g. other plugins, the CLI, Admin).
// All requests must include a matching X-Plugin-Secret header.
func handleIngest(pool *pgxpool.Pool, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Always authenticate internal callers.
		provided := r.Header.Get("X-Plugin-Secret")
		if provided == "" || provided != secret {
			sdk.Respond(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid X-Plugin-Secret"})
			return
		}

		var req IngestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		// Validate required fields.
		if req.EventType == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "event_type is required"})
			return
		}
		if !validEventTypes[req.EventType] {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{
				"error": "invalid event_type; accepted values: auth.login, auth.logout, auth.login_failed, auth.mfa_enabled, privilege.change, secret.accessed, plugin.installed, plugin.uninstalled",
			})
			return
		}
		if req.ActorType != "" && !validActorTypes[req.ActorType] {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{
				"error": "invalid actor_type; accepted values: user, system, plugin",
			})
			return
		}
		if req.Severity != "" && !validSeverities[req.Severity] {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{
				"error": "invalid severity; accepted values: info, warning, critical",
			})
			return
		}

		// Apply defaults.
		if req.ActorType == "" {
			req.ActorType = "system"
		}
		if req.Severity == "" {
			req.Severity = "info"
		}
		if req.SourceAccountID == "" {
			req.SourceAccountID = "primary"
		}
		if req.Metadata == nil {
			req.Metadata = map[string]any{}
		}

		event := &AuditEvent{
			ID:              uuid.New().String(),
			SourceAccountID: req.SourceAccountID,
			ActorUserID:     req.ActorUserID,
			ActorType:       req.ActorType,
			EventType:       req.EventType,
			ResourceType:    req.ResourceType,
			ResourceID:      req.ResourceID,
			IPAddress:       req.IPAddress,
			UserAgent:       req.UserAgent,
			Metadata:        req.Metadata,
			Severity:        req.Severity,
			CreatedAt:       time.Now().UTC(),
		}

		// Prefer real client IP when X-Plugin-Secret is set (trusted call from
		// within the stack) or fall back to the remote address.
		if event.IPAddress == "" {
			event.IPAddress = realIP(r)
		}
		if event.UserAgent == "" {
			event.UserAgent = r.Header.Get("User-Agent")
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := InsertEvent(ctx, pool, event); err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to store event: " + err.Error()})
			return
		}

		sdk.Respond(w, http.StatusCreated, event)
	}
}

// handleList handles GET /events.
// All requests must include a matching X-Plugin-Secret header.
//
// Supported query parameters:
//
//	event_type        — exact match
//	actor_user_id     — exact match
//	severity          — exact match (info | warning | critical)
//	source_account_id — exact match; omit to return all accounts
//	from              — RFC 3339 lower bound on created_at
//	to                — RFC 3339 upper bound on created_at
//	limit             — default 50, max 1000
//	offset            — default 0
func handleList(pool *pgxpool.Pool, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-Plugin-Secret")
		if provided == "" || provided != secret {
			sdk.Respond(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid X-Plugin-Secret"})
			return
		}

		f, err := parseQueryFilter(r)
		if err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		events, total, err := ListEvents(ctx, pool, f)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if events == nil {
			events = []*AuditEvent{}
		}

		sdk.Respond(w, http.StatusOK, ListResponse{
			Events: events,
			Total:  total,
			Limit:  f.Limit,
			Offset: f.Offset,
		})
	}
}

// handleGet handles GET /events/{id}.
// All requests must include a matching X-Plugin-Secret header.
func handleGet(pool *pgxpool.Pool, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-Plugin-Secret")
		if provided == "" || provided != secret {
			sdk.Respond(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid X-Plugin-Secret"})
			return
		}

		id := chi.URLParam(r, "id")
		if id == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		event, err := GetEvent(ctx, pool, id)
		if err != nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "event not found"})
			return
		}

		sdk.Respond(w, http.StatusOK, event)
	}
}

// handleAdminList handles GET /admin/events.
// This is the Admin-facing variant of GET /events. It accepts either
// X-Plugin-Secret (plugin internal callers) or X-Hasura-Admin-Secret (Admin
// UI via Hasura). Supports the same filter query parameters as GET /events.
func handleAdminList(pool *pgxpool.Pool, secret, adminSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizeAdminRequest(r, secret, adminSecret) {
			sdk.Respond(w, http.StatusUnauthorized, map[string]string{
				"error": "missing or invalid authentication header; provide X-Plugin-Secret or X-Hasura-Admin-Secret",
			})
			return
		}

		f, err := parseQueryFilter(r)
		if err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		events, total, err := ListEvents(ctx, pool, f)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if events == nil {
			events = []*AuditEvent{}
		}

		sdk.Respond(w, http.StatusOK, ListResponse{
			Events: events,
			Total:  total,
			Limit:  f.Limit,
			Offset: f.Offset,
		})
	}
}

// handleExport handles GET /events/export.
// Returns all matching events as a CSV download suitable for compliance
// archiving. Pagination parameters (limit/offset) are ignored; use start/end
// to bound the export window.
//
// Query parameters:
//
//	format            — must be "csv" (only supported format)
//	start             — RFC 3339 lower bound on created_at (required)
//	end               — RFC 3339 upper bound on created_at (required)
//	event_type        — optional exact match
//	actor_user_id     — optional exact match
//	severity          — optional exact match
//	source_account_id — optional exact match
func handleExport(pool *pgxpool.Pool, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-Plugin-Secret")
		if provided == "" || provided != secret {
			sdk.Respond(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid X-Plugin-Secret"})
			return
		}

		q := r.URL.Query()

		format := q.Get("format")
		if format == "" {
			format = "csv"
		}
		if format != "csv" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "unsupported format; only 'csv' is accepted"})
			return
		}

		startStr := q.Get("start")
		endStr := q.Get("end")
		if startStr == "" || endStr == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "start and end (RFC 3339) are required for export"})
			return
		}

		startTime, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid 'start' timestamp; expected RFC 3339"})
			return
		}
		endTime, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid 'end' timestamp; expected RFC 3339"})
			return
		}
		if !endTime.After(startTime) {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "'end' must be after 'start'"})
			return
		}

		f := QueryFilter{
			EventType:       q.Get("event_type"),
			ActorUserID:     q.Get("actor_user_id"),
			Severity:        q.Get("severity"),
			SourceAccountID: q.Get("source_account_id"),
			From:            &startTime,
			To:              &endTime,
		}

		if f.EventType != "" && !validEventTypes[f.EventType] {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid event_type filter"})
			return
		}
		if f.Severity != "" && !validSeverities[f.Severity] {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid severity filter; accepted values: info, warning, critical"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		events, err := ExportEvents(ctx, pool, f)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		filename := fmt.Sprintf("audit-log-%s-%s.csv",
			startTime.UTC().Format("20060102"),
			endTime.UTC().Format("20060102"),
		)

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.WriteHeader(http.StatusOK)

		cw := csv.NewWriter(w)

		// Write header row.
		cw.Write([]string{
			"id", "source_account_id", "actor_user_id", "actor_type",
			"event_type", "resource_type", "resource_id",
			"ip_address", "user_agent", "severity", "created_at",
		})

		for _, e := range events {
			cw.Write([]string{
				e.ID,
				e.SourceAccountID,
				e.ActorUserID,
				e.ActorType,
				e.EventType,
				e.ResourceType,
				e.ResourceID,
				e.IPAddress,
				e.UserAgent,
				e.Severity,
				e.CreatedAt.UTC().Format(time.RFC3339),
			})
		}

		cw.Flush()
	}
}

// authorizeAdminRequest returns true when the request carries a valid
// X-Plugin-Secret or (when adminSecret is non-empty) a valid
// X-Hasura-Admin-Secret header.
func authorizeAdminRequest(r *http.Request, secret, adminSecret string) bool {
	if ps := r.Header.Get("X-Plugin-Secret"); ps != "" && ps == secret {
		return true
	}
	if adminSecret != "" {
		if as := r.Header.Get("X-Hasura-Admin-Secret"); as != "" && as == adminSecret {
			return true
		}
	}
	return false
}

// parseQueryFilter builds a QueryFilter from the request's URL query parameters.
// Returns an error for any malformed parameter so the caller can return HTTP 400.
func parseQueryFilter(r *http.Request) (QueryFilter, error) {
	q := r.URL.Query()

	f := QueryFilter{
		EventType:       q.Get("event_type"),
		ActorUserID:     q.Get("actor_user_id"),
		Severity:        q.Get("severity"),
		SourceAccountID: q.Get("source_account_id"),
	}

	// Parse limit (cap at 1000).
	f.Limit = 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return QueryFilter{}, fmt.Errorf("invalid 'limit' parameter")
		}
		if n > 1000 {
			n = 1000
		}
		f.Limit = n
	}

	// Parse offset.
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return QueryFilter{}, fmt.Errorf("invalid 'offset' parameter")
		}
		f.Offset = n
	}

	// Parse optional time bounds (from/to).
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return QueryFilter{}, fmt.Errorf("invalid 'from' timestamp; expected RFC 3339")
		}
		f.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return QueryFilter{}, fmt.Errorf("invalid 'to' timestamp; expected RFC 3339")
		}
		f.To = &t
	}

	// Validate enum filters if provided.
	if f.EventType != "" && !validEventTypes[f.EventType] {
		return QueryFilter{}, fmt.Errorf("invalid event_type filter")
	}
	if f.Severity != "" && !validSeverities[f.Severity] {
		return QueryFilter{}, fmt.Errorf("invalid severity filter; accepted values: info, warning, critical")
	}

	return f, nil
}

// realIP extracts the client IP from standard proxy headers or falls back to
// the remote address.
func realIP(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// X-Forwarded-For can be a comma-separated list; the leftmost is the
		// original client.
		parts := strings.SplitN(v, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	// Strip port from RemoteAddr.
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		addr = addr[:idx]
	}
	return addr
}
