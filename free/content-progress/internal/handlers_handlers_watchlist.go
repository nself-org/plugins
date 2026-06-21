package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// History Handlers
// =========================================================================

func handleGetUserHistory(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		limit := queryInt(r, "limit", 100)
		offset := queryInt(r, "offset", 0)

		scopedDB := scopedDBFromRequest(r, db)
		history, err := scopedDB.GetUserHistory(userID, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get user history failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"data":   history,
			"limit":  limit,
			"offset": offset,
		})
	}
}

// =========================================================================
// Watchlist Handlers
// =========================================================================

func handleAddToWatchlist(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AddToWatchlistRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		if errMsg := validateAddToWatchlist(req); errMsg != "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%s", errMsg))
			return
		}

		scopedDB := scopedDBFromRequest(r, db)
		item, err := scopedDB.AddToWatchlist(req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("add to watchlist failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, item)
	}
}

func handleGetWatchlist(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		limit := queryInt(r, "limit", 100)
		offset := queryInt(r, "offset", 0)

		scopedDB := scopedDBFromRequest(r, db)
		items, err := scopedDB.GetWatchlist(userID, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get watchlist failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"data":   items,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleUpdateWatchlistItem(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		contentType := chi.URLParam(r, "contentType")
		contentID := chi.URLParam(r, "contentId")

		var req UpdateWatchlistRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		scopedDB := scopedDBFromRequest(r, db)
		item, err := scopedDB.UpdateWatchlistItem(userID, contentType, contentID, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("update watchlist failed: %w", err))
			return
		}
		if item == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("watchlist item not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, item)
	}
}

func handleRemoveFromWatchlist(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		contentType := chi.URLParam(r, "contentType")
		contentID := chi.URLParam(r, "contentId")

		scopedDB := scopedDBFromRequest(r, db)
		deleted, err := scopedDB.RemoveFromWatchlist(userID, contentType, contentID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("remove from watchlist failed: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("watchlist item not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

// =========================================================================
// Favorites Handlers
// =========================================================================

func handleAddToFavorites(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AddToFavoritesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		if errMsg := validateAddToFavorites(req); errMsg != "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%s", errMsg))
			return
		}

		scopedDB := scopedDBFromRequest(r, db)
		item, err := scopedDB.AddToFavorites(req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("add to favorites failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, item)
	}
}

func handleGetFavorites(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		limit := queryInt(r, "limit", 100)
		offset := queryInt(r, "offset", 0)

		scopedDB := scopedDBFromRequest(r, db)
		items, err := scopedDB.GetFavorites(userID, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get favorites failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"data":   items,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleRemoveFromFavorites(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		contentType := chi.URLParam(r, "contentType")
		contentID := chi.URLParam(r, "contentId")

		scopedDB := scopedDBFromRequest(r, db)
		deleted, err := scopedDB.RemoveFromFavorites(userID, contentType, contentID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("remove from favorites failed: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("favorite not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

// =========================================================================
// Stats Handlers
// =========================================================================
