package internal

import (
	"time"

	"github.com/go-chi/chi/v5"
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

