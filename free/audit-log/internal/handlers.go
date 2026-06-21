package internal

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
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
