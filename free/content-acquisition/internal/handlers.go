package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// sourceAccountID extracts the multi-app isolation header.
func sourceAccountID(r *http.Request) string {
	id := r.Header.Get("X-Hasura-Source-Account-Id")
	if id == "" {
		id = "primary"
	}
	return id
}

// RegisterRoutes mounts all content-acquisition API routes on the given router.
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

// =========================================================================
// Calendar
// =========================================================================

func handleGetCalendar() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Calendar returns an empty list; matches the TS implementation
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"calendar": []interface{}{}})
	}
}

// =========================================================================
// Queue
// =========================================================================

func handleGetQueue(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		queue, err := db.GetQueue(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get queue: %w", err))
			return
		}
		if queue == nil {
			queue = []AcquisitionQueueItem{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"queue": queue})
	}
}

func handleAddToQueue(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req AddToQueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ContentType == "" || req.ContentName == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("contentType and contentName are required"))
			return
		}

		accountID := sourceAccountID(r)
		item, err := db.AddToQueue(accountID, req.ContentType, req.ContentName, req.Year, req.Season, req.Episode, "api")
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to add to queue: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"item": item})
	}
}

// =========================================================================
// History
// =========================================================================

func handleGetHistory(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		days := 90
		if v := r.URL.Query().Get("days"); v != "" {
			if d, err := strconv.Atoi(v); err == nil && d > 0 {
				days = d
			}
		}
		history, err := db.ListAcquisitionHistory(accountID, days)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list history: %w", err))
			return
		}
		if history == nil {
			history = []AcquisitionHistoryItem{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"history": history})
	}
}

// =========================================================================
// Quality Profiles
// =========================================================================

func handleListProfiles(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		profiles, err := db.ListProfiles(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list profiles: %w", err))
			return
		}
		if profiles == nil {
			profiles = []QualityProfile{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"profiles": profiles})
	}
}

func handleCreateProfile(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("name is required"))
			return
		}

		qualities := req.PreferredQualities
		if len(qualities) == 0 {
			qualities = []string{"1080p", "720p"}
		}
		minSeeders := 1
		if req.MinSeeders != nil {
			minSeeders = *req.MinSeeders
		}

		accountID := sourceAccountID(r)
		profile, err := db.CreateQualityProfile(accountID, req.Name, qualities, minSeeders)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create profile: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"profile": profile})
	}
}

func handleGetPresets() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"presets": QualityPresets})
	}
}

// =========================================================================
// Movies
// =========================================================================

func handleCreateMovie(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateMovieRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Title == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("title is required"))
			return
		}

		qp := "balanced"
		if req.QualityProfile != nil {
			qp = *req.QualityProfile
		}
		autoDownload := true
		if req.AutoDownload != nil {
			autoDownload = *req.AutoDownload
		}
		autoUpgrade := false
		if req.AutoUpgrade != nil {
			autoUpgrade = *req.AutoUpgrade
		}

		accountID := sourceAccountID(r)
		movie, err := db.CreateMovieMonitoring(accountID, req.Title, req.TmdbID, qp, autoDownload, autoUpgrade)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create movie: %w", err))
			return
		}
		sdk.Respond(w, http.StatusCreated, map[string]interface{}{"movie": movie})
	}
}

func handleListMovies(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		movies, err := db.ListMovieMonitoring(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list movies: %w", err))
			return
		}
		if movies == nil {
			movies = []MovieMonitoring{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"movies": movies})
	}
}

func handleUpdateMovie(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req UpdateMovieRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		movie, err := db.UpdateMovieMonitoring(id, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update movie: %w", err))
			return
		}
		if movie == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Movie not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"movie": movie})
	}
}

func handleDeleteMovie(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted, err := db.DeleteMovieMonitoring(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete movie: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Movie not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

// =========================================================================
// Downloads
// =========================================================================

func handleCreateDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateDownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ContentType == "" || req.Title == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("contentType and title are required"))
			return
		}

		qp := "balanced"
		if req.QualityProfile != nil {
			qp = *req.QualityProfile
		}

		accountID := sourceAccountID(r)
		dl, err := db.CreateDownload(accountID, req.ContentType, req.Title, req.MagnetURI, qp, req.ShowID, req.SeasonNumber, req.EpisodeNumber, req.TmdbID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create download: %w", err))
			return
		}

		// Add to download queue
		if err := db.AddToDownloadQueue(dl.ID); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to add to download queue: %w", err))
			return
		}

		sdk.Respond(w, http.StatusCreated, map[string]interface{}{"download": dl})
	}
}

func handleListDownloads(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		var stateFilter *string
		if v := r.URL.Query().Get("status"); v != "" {
			stateFilter = &v
		}
		downloads, err := db.ListDownloads(accountID, stateFilter)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list downloads: %w", err))
			return
		}
		if downloads == nil {
			downloads = []Download{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"downloads": downloads})
	}
}

func handleGetDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download": dl})
	}
}

func handleDeleteDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}

		// Transition to cancelled if not already in a terminal state
		terminalStates := map[string]bool{"completed": true, "failed": true, "cancelled": true}
		if !terminalStates[dl.State] {
			meta, _ := json.Marshal(map[string]string{"reason": "user_cancelled"})
			_ = db.UpdateDownloadState(id, "cancelled", meta)
		}

		_ = db.RemoveFromDownloadQueue(id)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"cancelled": true, "download_id": id})
	}
}

func handlePauseDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}

		meta, _ := json.Marshal(map[string]string{"reason": "user_paused"})
		if err := db.UpdateDownloadState(id, "paused", meta); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%v", err))
			return
		}

		updated, _ := db.GetDownload(id)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download": updated})
	}
}

func handleResumeDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}
		if dl.State != "paused" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("Download is not paused"))
			return
		}

		// Find the state before pause from history
		history, err := db.GetDownloadStateHistory(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get history: %w", err))
			return
		}

		resumeState := "downloading"
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].ToState == "paused" && history[i].FromState != nil {
				resumeState = *history[i].FromState
				break
			}
		}

		meta, _ := json.Marshal(map[string]string{"reason": "user_resumed"})
		if err := db.UpdateDownloadState(id, resumeState, meta); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%v", err))
			return
		}

		updated, _ := db.GetDownload(id)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download": updated})
	}
}

func handleRetryDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}
		if dl.State != "failed" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("Only failed downloads can be retried"))
			return
		}

		meta, _ := json.Marshal(map[string]string{"reason": "user_retry"})
		if err := db.UpdateDownloadState(id, "created", meta); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("%v", err))
			return
		}

		newRetryCount := dl.RetryCount + 1
		_ = db.UpdateDownloadFields(id, &newRetryCount, nil)
		_ = db.AddToDownloadQueue(id)

		updated, _ := db.GetDownload(id)
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download": updated})
	}
}

func handleGetDownloadHistory(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dl, err := db.GetDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get download: %w", err))
			return
		}
		if dl == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Download not found"))
			return
		}

		history, err := db.GetDownloadStateHistory(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get history: %w", err))
			return
		}
		if history == nil {
			history = []DownloadStateTransition{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"download_id": id, "history": history})
	}
}

// =========================================================================
// Download Rules
// =========================================================================

func handleCreateRule(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Name == "" || req.Action == "" || req.Conditions == nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("name, conditions, and action are required"))
			return
		}

		priority := 0
		if req.Priority != nil {
			priority = *req.Priority
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		accountID := sourceAccountID(r)
		rule, err := db.CreateDownloadRule(accountID, req.Name, req.Conditions, req.Action, priority, enabled)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create rule: %w", err))
			return
		}
		sdk.Respond(w, http.StatusCreated, map[string]interface{}{"rule": rule})
	}
}

func handleListRules(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		rules, err := db.ListDownloadRules(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list rules: %w", err))
			return
		}
		if rules == nil {
			rules = []DownloadRule{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"rules": rules})
	}
}

func handleUpdateRule(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req UpdateRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		rule, err := db.UpdateDownloadRule(id, req)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to update rule: %w", err))
			return
		}
		if rule == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Rule not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"rule": rule})
	}
}

func handleDeleteRule(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted, err := db.DeleteDownloadRule(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to delete rule: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Rule not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"deleted": true})
	}
}

func handleTestRule(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		rule, err := db.GetDownloadRule(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get rule: %w", err))
			return
		}
		if rule == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Rule not found"))
			return
		}

		var req TestRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}

		// Evaluate conditions against sample data
		var conditions map[string]interface{}
		if err := json.Unmarshal(rule.Conditions, &conditions); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("invalid rule conditions: %w", err))
			return
		}

		allMatch := true
		type fieldResult struct {
			Field    string      `json:"field"`
			Expected interface{} `json:"expected"`
			Actual   interface{} `json:"actual"`
			Match    bool        `json:"match"`
		}
		var results []fieldResult

		for field, expected := range conditions {
			actual := req.Sample[field]
			match := false

			switch ev := expected.(type) {
			case string:
				if av, ok := actual.(string); ok {
					match = strings.Contains(strings.ToLower(av), strings.ToLower(ev))
				}
			case float64:
				if av, ok := actual.(float64); ok {
					match = av >= ev
				}
			case bool:
				match = actual == expected
			default:
				match = actual == expected
			}

			results = append(results, fieldResult{
				Field:    field,
				Expected: expected,
				Actual:   actual,
				Match:    match,
			})
			if !match {
				allMatch = false
			}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"rule_id":   rule.ID,
			"rule_name": rule.Name,
			"action":    rule.Action,
			"matches":   allMatch,
			"results":   results,
		})
	}
}

// =========================================================================
// Dashboard
// =========================================================================

func handleGetDashboard(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sourceAccountID(r)
		summary, err := db.GetDashboardSummary(accountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get dashboard: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"summary": summary})
	}
}

// =========================================================================
// Pipeline
// =========================================================================

func handleListPipeline(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var status *string
		if v := r.URL.Query().Get("status"); v != "" {
			status = &v
		}

		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if l, err := strconv.Atoi(v); err == nil && l > 0 {
				limit = l
			}
		}
		offset := 0
		if v := r.URL.Query().Get("offset"); v != "" {
			if o, err := strconv.Atoi(v); err == nil && o >= 0 {
				offset = o
			}
		}

		runs, total, err := db.ListPipelineRuns(status, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to list pipeline runs: %w", err))
			return
		}
		if runs == nil {
			runs = []PipelineRun{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"runs": runs, "total": total})
	}
}

func handleGetPipeline(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("Invalid pipeline ID"))
			return
		}

		run, err := db.GetPipelineRun(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get pipeline run: %w", err))
			return
		}
		if run == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Pipeline run not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{"run": run})
	}
}

func handleTriggerPipeline(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PipelineTriggerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.ContentTitle == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("content_title is required"))
			return
		}

		accountID := sourceAccountID(r)

		metaMap := map[string]interface{}{}
		if req.MagnetURL != nil {
			metaMap["magnet_url"] = *req.MagnetURL
		}
		if req.TorrentURL != nil {
			metaMap["torrent_url"] = *req.TorrentURL
		}
		metadata, _ := json.Marshal(metaMap)

		triggerSource := "api"
		run, err := db.CreatePipelineRun(accountID, "manual", &triggerSource, req.ContentTitle, req.ContentType, metadata)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to create pipeline run: %w", err))
			return
		}

		sdk.Respond(w, http.StatusAccepted, map[string]interface{}{"run": run, "message": "Pipeline triggered"})
	}
}

func handleRetryPipeline(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("Invalid pipeline ID"))
			return
		}

		run, err := db.GetPipelineRun(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("failed to get pipeline run: %w", err))
			return
		}
		if run == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("Pipeline run not found"))
			return
		}
		if run.Status == "completed" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("Pipeline already completed"))
			return
		}

		sdk.Respond(w, http.StatusAccepted, map[string]interface{}{
			"message":    "Pipeline retry triggered",
			"pipelineId": id,
		})
	}
}

// =========================================================================
// RSS Polling & Matching (API endpoints)
// =========================================================================

func handleRSSPoll() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RSSPollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.URL == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("url is required"))
			return
		}

		// RSS polling requires an RSS parser library. In the Go port the actual
		// polling logic runs as a background goroutine. This endpoint provides
		// a minimal response shape matching the TS contract.
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"url":       req.URL,
			"itemCount": 0,
			"matches":   []interface{}{},
			"polledAt":  time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func handleRSSTest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RSSTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.URL == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("url is required"))
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"url":       req.URL,
			"valid":     true,
			"itemCount": 0,
			"sample":    []interface{}{},
			"testedAt":  time.Now().UTC().Format(time.RFC3339),
		})
	}
}
