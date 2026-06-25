package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// Subscriptions
// =========================================================================

func handleCreateSubscription(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateSubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ContentName == "" || req.ContentType == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("contentType and contentName are required"))
			return
		}

		accountID := sourceAccountID(r)
		sub, err := db.CreateSubscription(accountID, req.ContentType, req.ContentID, req.ContentName, req.QualityProfileID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create subscription: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"subscription": sub})
	}
}

func handleListSubscriptions(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		subs, err := db.ListSubscriptions(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list subscriptions: %w", err))
			return
		}
		if subs == nil {
			subs = []Subscription{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"subscriptions": subs})
	}
}

func handleGetSubscription(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sub, err := db.GetSubscription(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get subscription: %w", err))
			return
		}
		if sub == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Subscription not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"subscription": sub})
	}
}

func handleUpdateSubscription(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req UpdateSubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		sub, err := db.UpdateSubscription(id, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update subscription: %w", err))
			return
		}
		if sub == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Subscription not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"subscription": sub})
	}
}

func handleDeleteSubscription(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted, err := db.DeleteSubscription(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete subscription: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Subscription not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

