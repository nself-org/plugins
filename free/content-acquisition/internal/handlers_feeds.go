package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// RSS Feeds
// =========================================================================

func handleListFeeds(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		feeds, err := db.ListRSSFeeds(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list feeds: %w", err))
			return
		}
		if feeds == nil {
			feeds = []RSSFeed{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"feeds": feeds})
	}
}

func handleCreateFeed(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateFeedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" || req.URL == "" || req.FeedType == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("name, url, and feedType are required"))
			return
		}

		accountID := sourceAccountID(r)
		feed, err := db.CreateRSSFeed(accountID, req.Name, req.URL, req.FeedType)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create feed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"feed": feed})
	}
}

func handleValidateFeed() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ValidateFeedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.URL == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("url is required"))
			return
		}
		// Feed URL is accepted; full RSS parsing runs asynchronously via the
		// RSS monitor goroutine.
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"valid":   true,
			"message": "URL accepted; actual feed parsing runs asynchronously",
		})
	}
}

func handleUpdateFeed(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req UpdateFeedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		feed, err := db.UpdateRSSFeed(id, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update feed: %w", err))
			return
		}
		if feed == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Feed not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"feed": feed})
	}
}

func handleDeleteFeed(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted, err := db.DeleteRSSFeed(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete feed: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Feed not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

