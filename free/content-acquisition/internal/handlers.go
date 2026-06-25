package internal

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// sourceAccountID extracts the multi-app isolation account ID from the request.
// Delegates to sdk.SourceAccountID for DRY cross-plugin consistency.
func sourceAccountID(r *http.Request) string {
	return sdk.SourceAccountID(r)
}

// RegisterRoutes mounts all content-acquisition API routes on the given router.
// Size-cap exception: single-responsibility HTTP route handler — 70L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func RegisterRoutes(r chi.Router, db *DB) {
	r.Route("/v1", func(r chi.Router) {
		// Subscriptions
		r.Post("/subscriptions", handleCreateSubscription(db))
		r.Get("/subscriptions", handleListSubscriptions(db))
		r.Get("/subscriptions/{id}", handleGetSubscription(db))
		r.Put("/subscriptions/{id}", handleUpdateSubscription(db))
		r.Delete("/subscriptions/{id}", handleDeleteSubscription(db))

		// RSS Feeds
		r.Get("/feeds", handleListFeeds(db))
		r.Post("/feeds", handleCreateFeed(db))
		r.Post("/feeds/validate", handleValidateFeed())
		r.Put("/feeds/{id}", handleUpdateFeed(db))
		r.Delete("/feeds/{id}", handleDeleteFeed(db))

		// Calendar
		r.Get("/calendar", handleGetCalendar())

		// Queue
		r.Get("/queue", handleGetQueue(db))
		r.Post("/queue", handleAddToQueue(db))

		// History
		r.Get("/history", handleGetHistory(db))

		// Quality Profiles
		r.Get("/profiles", handleListProfiles(db))
		r.Post("/profiles", handleCreateProfile(db))
		r.Get("/profiles/presets", handleGetPresets())

		// Movies
		r.Post("/movies", handleCreateMovie(db))
		r.Get("/movies", handleListMovies(db))
		r.Put("/movies/{id}", handleUpdateMovie(db))
		r.Delete("/movies/{id}", handleDeleteMovie(db))

		// Downloads
		r.Post("/downloads", handleCreateDownload(db))
		r.Get("/downloads", handleListDownloads(db))
		r.Get("/downloads/{id}", handleGetDownload(db))
		r.Delete("/downloads/{id}", handleDeleteDownload(db))
		r.Patch("/downloads/{id}/pause", handlePauseDownload(db))
		r.Patch("/downloads/{id}/resume", handleResumeDownload(db))
		r.Post("/downloads/{id}/retry", handleRetryDownload(db))
		r.Get("/downloads/{id}/history", handleGetDownloadHistory(db))

		// Download Rules
		r.Post("/rules", handleCreateRule(db))
		r.Get("/rules", handleListRules(db))
		r.Put("/rules/{id}", handleUpdateRule(db))
		r.Delete("/rules/{id}", handleDeleteRule(db))
		r.Post("/rules/{id}/test", handleTestRule(db))

		// Dashboard
		r.Get("/dashboard", handleGetDashboard(db))
	})

	r.Route("/api", func(r chi.Router) {
		// Pipeline
		r.Get("/pipeline", handleListPipeline(db))
		r.Get("/pipeline/{id}", handleGetPipeline(db))
		r.Post("/pipeline/trigger", handleTriggerPipeline(db))
		r.Post("/pipeline/retry/{id}", handleRetryPipeline(db))

		// RSS polling
		r.Post("/rss/poll", handleRSSPoll())
		r.Post("/rss/test", handleRSSTest())
	})
}

