package internal

import (
	"context"
	"net/http"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

// --- Health probes -----------------------------------------------------------

func handleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if err := db.Pool().Ping(ctx); err != nil {
			sdk.Respond(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready", "error": err.Error()})
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func handleLive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"status": "alive",
			"uptime": time.Since(startedAt).String(),
		})
	}
}

func handleStatus(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		stats, err := db.GetStats(ctx)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"plugin":  "donorbox",
			"version": "1.0.0",
			"uptime":  time.Since(startedAt).String(),
			"stats":   stats,
		})
	}
}

