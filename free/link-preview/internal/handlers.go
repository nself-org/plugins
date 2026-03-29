package internal

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

const defaultCacheTTL = 7 * 24 * time.Hour // 168 hours

// RegisterRoutes mounts all link-preview endpoints on the given router.
func RegisterRoutes(r chi.Router, pool *pgxpool.Pool) {
	r.Post("/v1/preview", handleFetchPreview(pool))
	r.Get("/v1/preview", handleGetPreview(pool))
	r.Delete("/v1/cache", handleClearCache(pool))
}

// --- Request / Response types ------------------------------------------------

// FetchPreviewRequest is the JSON body for POST /v1/preview.
type FetchPreviewRequest struct {
	URL   string `json:"url"`
	Force bool   `json:"force"`
}

// --- Handlers ----------------------------------------------------------------

// handleFetchPreview extracts metadata from a URL, caches it, and returns the preview.
func handleFetchPreview(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req FetchPreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		if req.URL == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}

		// Check cache unless forced
		if !req.Force {
			cached, err := GetPreviewByURL(pool, req.URL)
			if err != nil {
				log.Printf("[link-preview] cache lookup error: %v", err)
			}
			if cached != nil {
				sdk.Respond(w, http.StatusOK, cached)
				return
			}
		}

		// Extract metadata
		meta, err := ExtractMetadata(req.URL)
		if err != nil {
			log.Printf("[link-preview] extraction failed for %s: %v", req.URL, err)
			sdk.Respond(w, http.StatusBadGateway, map[string]string{
				"error": "failed to fetch URL metadata",
			})
			return
		}

		now := time.Now().UTC()
		expires := now.Add(defaultCacheTTL)

		preview := &LinkPreview{
			ID:          uuid.New().String(),
			URL:         req.URL,
			Title:       strPtr(meta.Title),
			Description: strPtr(meta.Description),
			Image:       strPtr(meta.Image),
			SiteName:    strPtr(meta.SiteName),
			Type:        strPtr(meta.Type),
			FetchedAt:   now,
			ExpiresAt:   &expires,
		}

		if err := InsertPreview(pool, preview); err != nil {
			log.Printf("[link-preview] db insert error: %v", err)
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to cache preview"})
			return
		}

		sdk.Respond(w, http.StatusOK, preview)
	}
}

// handleGetPreview returns a cached preview for a URL query parameter.
func handleGetPreview(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		url := r.URL.Query().Get("url")
		if url == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "url query parameter is required"})
			return
		}

		preview, err := GetPreviewByURL(pool, url)
		if err != nil {
			log.Printf("[link-preview] cache lookup error: %v", err)
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}

		if preview == nil {
			sdk.Respond(w, http.StatusNotFound, map[string]string{"error": "no cached preview for this URL"})
			return
		}

		sdk.Respond(w, http.StatusOK, preview)
	}
}

// handleClearCache deletes all cached previews.
func handleClearCache(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		count, err := DeleteAllPreviews(pool)
		if err != nil {
			log.Printf("[link-preview] cache clear error: %v", err)
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to clear cache"})
			return
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"cleared": count,
		})
	}
}

// strPtr returns a pointer to s, or nil if s is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
