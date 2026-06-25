package internal

import (
	"context"
	"net/http"
	"time"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

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
