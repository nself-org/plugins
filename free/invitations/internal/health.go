package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// handleHealth answers GET /health: database reachability plus the boot-migrations counts.
func handleHealth(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// bootMigrationsHealth may answer 503 itself, so no explicit WriteHeader(200) before it (the default applies).
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "migrations": bootMigrationsHealth(w)})
	}
}
