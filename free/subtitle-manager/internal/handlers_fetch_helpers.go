package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"sync"
	sdk "github.com/nself-org/plugin-sdk"
)

// Size-cap exception: single-responsibility HTTP route handler — 82L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func handleFetchBest(db *DB, cfg *Config, osClient *OpenSubtitlesClient, syncer *Synchronizer, norm *Normalizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req FetchBestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.VideoPath == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("video_path is required"))
			return
		}
		if len(req.Languages) == 0 {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("languages is required and must be non-empty"))
			return
		}

		// Path-traversal guard: the video path is passed to the sync subprocess.
		videoPath, err := validateMediaPath(cfg.MediaRoot, req.VideoPath)
		if err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("video_path: %w", err))
			return
		}

		sourceAccountID := getSourceAccountID(r)
		maxAlts := req.MaxAlternatives
		if maxAlts <= 0 {
			maxAlts = 3
		}
		mediaType := req.MediaType
		if mediaType == "" {
			mediaType = "movie"
		}

		// Process each language in parallel
		type langResult struct {
			result FetchBestLanguageResult
			err    error
		}

		var wg sync.WaitGroup
		results := make([]langResult, len(req.Languages))

		for i, lang := range req.Languages {
			wg.Add(1)
			go func(idx int, language string) {
				defer wg.Done()
				res := fetchBestForLanguage(fetchBestParams{
					videoPath:       videoPath,
					language:        language,
					maxAlternatives: maxAlts,
					sourceAccountID: sourceAccountID,
					mediaID:         req.MediaID,
					mediaType:       mediaType,
					mediaTitle:      req.MediaTitle,
					cfg:             cfg,
					osClient:        osClient,
					syncer:          syncer,
					norm:            norm,
					db:              db,
				})
				results[idx] = langResult{result: res}
			}(i, lang)
		}
		wg.Wait()

		subtitles := make([]FetchBestLanguageResult, len(results))
		foundCount := 0
		for i, lr := range results {
			subtitles[i] = lr.result
			if lr.result.Path != nil {
				foundCount++
			}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success":             true,
			"subtitles":           subtitles,
			"languages_requested": len(req.Languages),
			"languages_found":     foundCount,
		})
	}
}

// ---------------------------------------------------------------------------
// DELETE /v1/downloads/:id
// ---------------------------------------------------------------------------

func handleDeleteDownload(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted, err := db.DeleteDownload(id)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("delete download: %w", err))
			return
		}
		if !deleted {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("download not found"))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
		})
	}
}

// ---------------------------------------------------------------------------
// Fetch-best cascade logic
// ---------------------------------------------------------------------------

type fetchBestParams struct {
	videoPath       string
	language        string
	maxAlternatives int
	sourceAccountID string
	mediaID         string
	mediaType       string
	mediaTitle      string
	cfg             *Config
	osClient        *OpenSubtitlesClient
	syncer          *Synchronizer
	norm            *Normalizer
	db              *DB
}
