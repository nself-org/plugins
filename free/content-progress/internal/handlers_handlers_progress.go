package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	sdk "github.com/nself-org/plugin-sdk"
)

// =========================================================================
// Progress Handlers
// =========================================================================

func handleUpdateProgress(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req UpdateProgressRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		if errMsg := validateUpdateProgress(req); errMsg != "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%s", errMsg))
			return
		}

		scopedDB := scopedDBFromRequest(r, db)
		pos, err := scopedDB.UpdateProgress(req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("update progress failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, pos)
	}
}

func handleGetUserProgress(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		limit := queryInt(r, "limit", 100)
		offset := queryInt(r, "offset", 0)

		scopedDB := scopedDBFromRequest(r, db)
		positions, err := scopedDB.GetUserProgress(userID, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get user progress failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"data":   positions,
			"limit":  limit,
			"offset": offset,
		})
	}
}

func handleGetProgress(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		contentType := chi.URLParam(r, "contentType")
		contentID := chi.URLParam(r, "contentId")

		scopedDB := scopedDBFromRequest(r, db)
		pos, err := scopedDB.GetProgress(userID, contentType, contentID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get progress failed: %w", err))
			return
		}
		if pos == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("progress not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, pos)
	}
}

func handleDeleteProgress(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		contentType := chi.URLParam(r, "contentType")
		contentID := chi.URLParam(r, "contentId")

		scopedDB := scopedDBFromRequest(r, db)
		deleted, err := scopedDB.DeleteProgress(userID, contentType, contentID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("delete progress failed: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("progress not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

func handleMarkCompleted(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		contentType := chi.URLParam(r, "contentType")
		contentID := chi.URLParam(r, "contentId")

		scopedDB := scopedDBFromRequest(r, db)
		pos, err := scopedDB.MarkCompleted(userID, contentType, contentID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("mark completed failed: %w", err))
			return
		}
		if pos == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("progress not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, pos)
	}
}

func handleContinueWatching(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		limit := queryInt(r, "limit", 20)

		scopedDB := scopedDBFromRequest(r, db)
		items, err := scopedDB.GetContinueWatching(userID, limit)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get continue watching failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"data": items})
	}
}

func handleRecentlyWatched(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		limit := queryInt(r, "limit", 50)

		scopedDB := scopedDBFromRequest(r, db)
		items, err := scopedDB.GetRecentlyWatched(userID, limit)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get recently watched failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"data": items})
	}
}

