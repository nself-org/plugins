package internal

import (
	"context"
	"net/http"
	"strconv"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

// --- Sync operations ---------------------------------------------------------

func handleSync(db *DB, client *DonorboxClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			sdk.Respond(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Donorbox API credentials not configured. Set DONORBOX_EMAIL and DONORBOX_API_KEY.",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()

		result := runSync(ctx, db, client)
		status := http.StatusOK
		if !result.Success {
			status = http.StatusPartialContent
		}
		sdk.Respond(w, status, result)
	}
}

func handleReconcile(db *DB, client *DonorboxClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			sdk.Respond(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Donorbox API credentials not configured. Set DONORBOX_EMAIL and DONORBOX_API_KEY.",
			})
			return
		}

		lookbackDays := 7
		if v := r.URL.Query().Get("lookback_days"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				lookbackDays = n
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		result := runReconcile(ctx, db, client, lookbackDays)
		status := http.StatusOK
		if !result.Success {
			status = http.StatusPartialContent
		}
		sdk.Respond(w, status, result)
	}
}

