package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

var startTime = time.Now()

// RegisterRoutes mounts all content-progress API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB, cfg Config) {
	// Health / readiness / liveness
	r.Get("/ready", handleReady(db))
	r.Get("/live", handleLive(db))

	r.Route("/v1", func(r chi.Router) {
		// Status
		r.Get("/status", handleStatus(db, cfg))

		// Progress
		r.Post("/progress", handleUpdateProgress(db))
		r.Get("/progress/{userId}", handleGetUserProgress(db))
		r.Get("/progress/{userId}/{contentType}/{contentId}", handleGetProgress(db))
		r.Delete("/progress/{userId}/{contentType}/{contentId}", handleDeleteProgress(db))
		r.Post("/progress/{userId}/{contentType}/{contentId}/complete", handleMarkCompleted(db))

		// Continue watching / recently watched
		r.Get("/continue-watching/{userId}", handleContinueWatching(db))
		r.Get("/recently-watched/{userId}", handleRecentlyWatched(db))

		// History
		r.Get("/history/{userId}", handleGetUserHistory(db))

		// Watchlist
		r.Post("/watchlist", handleAddToWatchlist(db))
		r.Get("/watchlist/{userId}", handleGetWatchlist(db))
		r.Put("/watchlist/{userId}/{contentType}/{contentId}", handleUpdateWatchlistItem(db))
		r.Delete("/watchlist/{userId}/{contentType}/{contentId}", handleRemoveFromWatchlist(db))

		// Favorites
		r.Post("/favorites", handleAddToFavorites(db))
		r.Get("/favorites/{userId}", handleGetFavorites(db))
		r.Delete("/favorites/{userId}/{contentType}/{contentId}", handleRemoveFromFavorites(db))

		// Stats
		r.Get("/stats/{userId}", handleGetUserStats(db))

		// Webhook events
		r.Get("/events", handleListWebhookEvents(db))
	})
}

// =========================================================================
// Health / Readiness / Liveness
// =========================================================================

func handleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			sdk.Error(w, http.StatusServiceUnavailable, fmt.Errorf("database unavailable"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"ready":     true,
			"plugin":    "content-progress",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleLive(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetPluginStats()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get stats: %w", err))
			return
		}

		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"alive":   true,
			"plugin":  "content-progress",
			"version": "1.0.0",
			"uptime":  time.Since(startTime).Seconds(),
			"memory": map[string]interface{}{
				"alloc_mb":       float64(memStats.Alloc) / 1024 / 1024,
				"sys_mb":         float64(memStats.Sys) / 1024 / 1024,
				"num_gc":         memStats.NumGC,
				"goroutines":     runtime.NumGoroutine(),
			},
			"stats": map[string]interface{}{
				"totalUsers":      stats.TotalUsers,
				"totalPositions":  stats.TotalPositions,
				"totalCompleted":  stats.TotalCompleted,
				"lastActivity":    stats.LastActivity,
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleStatus(db *DB, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := db.GetPluginStats()
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get stats: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"plugin":  "content-progress",
			"version": "1.0.0",
			"status":  "running",
			"config": map[string]interface{}{
				"completeThreshold":    cfg.CompleteThreshold,
				"historySampleSeconds": cfg.HistorySampleSeconds,
			},
			"stats":     stats,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

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

// scopedDBFromRequest resolves multi-app source_account_id from the
// X-Source-Account-Id header and returns a scoped DB instance.
func scopedDBFromRequest(r *http.Request, db *DB) *DB {
	sourceAccountID := r.Header.Get("X-Source-Account-Id")
	if sourceAccountID == "" {
		sourceAccountID = os.Getenv("SOURCE_ACCOUNT_ID")
	}
	if sourceAccountID == "" {
		sourceAccountID = "primary"
	}
	return db.ForSourceAccount(sourceAccountID)
}
