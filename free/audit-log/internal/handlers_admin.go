package internal

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

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
			"ip_address", "user_agent", "severity",
			"source_plugin", "target_plugin", "created_at",
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
				e.SourcePlugin,
				e.TargetPlugin,
				e.CreatedAt.UTC().Format(time.RFC3339),
			})
		}

		cw.Flush()
	}
}

// authorizeAdminRequest returns true when the request carries a valid
// X-Plugin-Secret or (when adminSecret is non-empty) a valid
// X-Hasura-Admin-Secret header.
