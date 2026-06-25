package internal

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"strconv"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleGetUserStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")

		scopedDB := scopedDBFromRequest(r, db)
		stats, err := scopedDB.GetUserStats(userID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get user stats failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, stats)
	}
}

// =========================================================================
// Webhook Events Handler
// =========================================================================

func handleListWebhookEvents(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		eventType := r.URL.Query().Get("type")
		limit := queryInt(r, "limit", 100)
		offset := queryInt(r, "offset", 0)

		scopedDB := scopedDBFromRequest(r, db)
		events, err := scopedDB.ListWebhookEvents(eventType, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("list events failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"data":   events,
			"limit":  limit,
			"offset": offset,
		})
	}
}

// =========================================================================
// Validation Helpers
// =========================================================================

func validateUpdateProgress(req UpdateProgressRequest) string {
	if req.UserID == "" {
		return "user_id is required"
	}
	if req.ContentType == "" {
		return "content_type is required"
	}
	if !ValidContentTypes[ContentType(req.ContentType)] {
		return "content_type must be one of: movie, episode, video, audio, article, course"
	}
	if req.ContentID == "" {
		return "content_id is required"
	}
	if req.PositionSeconds < 0 {
		return "position_seconds must be >= 0"
	}
	if req.DurationSeconds != nil && *req.DurationSeconds < 0 {
		return "duration_seconds must be >= 0"
	}
	return ""
}

func validateAddToWatchlist(req AddToWatchlistRequest) string {
	if req.UserID == "" {
		return "user_id is required"
	}
	if req.ContentType == "" {
		return "content_type is required"
	}
	if !ValidContentTypes[ContentType(req.ContentType)] {
		return "content_type must be one of: movie, episode, video, audio, article, course"
	}
	if req.ContentID == "" {
		return "content_id is required"
	}
	if req.Priority != nil && (*req.Priority < 0 || *req.Priority > 10) {
		return "priority must be between 0 and 10"
	}
	return ""
}

func validateAddToFavorites(req AddToFavoritesRequest) string {
	if req.UserID == "" {
		return "user_id is required"
	}
	if req.ContentType == "" {
		return "content_type is required"
	}
	if !ValidContentTypes[ContentType(req.ContentType)] {
		return "content_type must be one of: movie, episode, video, audio, article, course"
	}
	if req.ContentID == "" {
		return "content_id is required"
	}
	return ""
}

// =========================================================================
// Utility Helpers
// =========================================================================

// queryInt parses a query parameter as an integer with a default value.
func queryInt(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// scopedDBFromRequest resolves multi-app source_account_id from the request
// using sdk.SourceAccountID (all 4 canonical header spellings) with env-var
// fallback for non-HTTP contexts. Fix: previously only checked
// X-Source-Account-Id (P4-E0 audit).
func scopedDBFromRequest(r *http.Request, db *DB) *DB {
	sourceAccountID := sdk.SourceAccountID(r)
	if sourceAccountID == "primary" {
		if env := os.Getenv("SOURCE_ACCOUNT_ID"); env != "" {
			sourceAccountID = env
		}
	}
	return db.ForSourceAccount(sourceAccountID)
}
