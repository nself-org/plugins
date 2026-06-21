package internal

import (
	"context"
	"net/http"
	"strconv"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleSync(pool *pgxpool.Pool, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		result := SyncAll(ctx, pool, cfg)
		status := http.StatusOK
		if !result.Success {
			status = http.StatusPartialContent
		}
		sdk.Respond(w, status, result)
	}
}

func handleReconcile(pool *pgxpool.Pool, cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		lookback := 7
		if v := r.URL.Query().Get("days"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				lookback = n
			}
		}

		result := Reconcile(ctx, pool, cfg, lookback)
		status := http.StatusOK
		if !result.Success {
			status = http.StatusPartialContent
		}
		sdk.Respond(w, status, result)
	}
}

// --- Transaction handlers ----------------------------------------------------
