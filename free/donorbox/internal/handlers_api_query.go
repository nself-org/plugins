package internal

import (
	"context"
	"net/http"
	"strconv"
	"time"
	sdk "github.com/nself-org/plugin-sdk"
)

// --- API query handlers ------------------------------------------------------

func handleListCampaigns(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		campaigns, err := db.QueryCampaigns(ctx, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if campaigns == nil {
			campaigns = []Campaign{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"campaigns": campaigns,
			"limit":     limit,
			"offset":    offset,
		})
	}
}

func handleListDonors(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		donors, err := db.QueryDonors(ctx, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if donors == nil {
			donors = []Donor{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"donors": donors,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleListDonations(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)
		status := r.URL.Query().Get("status")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		donations, err := db.QueryDonations(ctx, status, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if donations == nil {
			donations = []Donation{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"donations": donations,
			"limit":     limit,
			"offset":    offset,
		})
	}
}

func handleListPlans(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset := parsePagination(r)
		status := r.URL.Query().Get("status")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		plans, err := db.QueryPlans(ctx, status, limit, offset)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if plans == nil {
			plans = []Plan{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"plans":  plans,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleGetStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		stats, err := db.GetStats(ctx)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		sdk.Respond(w, http.StatusOK, stats)
	}
}

func handleListWebhookEvents(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		events, err := db.QueryWebhookEvents(ctx, limit)
		if err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if events == nil {
			events = []WebhookEvent{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"events": events,
		})
	}
}

