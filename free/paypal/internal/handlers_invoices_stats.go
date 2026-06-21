package internal

import (
	"context"
	"net/http"
	"strconv"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleListInvoices(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		invoices, err := ListInvoices(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if invoices == nil {
			invoices = []Invoice{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"invoices": invoices,
			"limit":    limit,
			"offset":   offset,
		})
	}
}

// --- Webhook events handler --------------------------------------------------

func handleListWebhookEvents(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		limit, offset := parsePagination(r)

		events, err := ListWebhookEvents(ctx, pool, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}
		if events == nil {
			events = []WebhookEvent{}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"webhook_events": events,
			"limit":          limit,
			"offset":         offset,
		})
	}
}

// --- Stats handler -----------------------------------------------------------

func handleStats(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		stats, err := GetSyncStats(ctx, pool)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, err)
			return
		}

		sdk.Respond(w, http.StatusOK, stats)
	}
}

// --- Helpers -----------------------------------------------------------------

// parsePagination extracts limit and offset query parameters with defaults.
func parsePagination(r *http.Request) (int, int) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	return limit, offset
}
