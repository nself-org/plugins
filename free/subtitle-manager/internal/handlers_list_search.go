package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	sdk "github.com/nself-org/plugin-sdk"
)

func handleListSubtitles(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.URL.Query().Get("media_id")
		if mediaID == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("media_id query parameter is required"))
			return
		}
		language := r.URL.Query().Get("language")
		if language == "" {
			language = "en"
		}
		sourceAccountID := getSourceAccountID(r)

		subtitles, err := db.SearchSubtitles(mediaID, language, sourceAccountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("search subtitles: %w", err))
			return
		}
		if subtitles == nil {
			subtitles = []SubtitleRecord{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"subtitles": subtitles,
		})
	}
}

// ---------------------------------------------------------------------------
// GET /v1/downloads
// ---------------------------------------------------------------------------

func handleListDownloads(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)
		limit := parseIntParam(r.URL.Query().Get("limit"), 50)
		offset := parseIntParam(r.URL.Query().Get("offset"), 0)

		downloads, total, err := db.ListDownloads(sourceAccountID, limit, offset)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("list downloads: %w", err))
			return
		}
		if downloads == nil {
			downloads = []DownloadRecord{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"downloads": downloads,
			"total":     total,
		})
	}
}

// ---------------------------------------------------------------------------
// GET /v1/stats
// ---------------------------------------------------------------------------

func handleGetStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sourceAccountID := getSourceAccountID(r)

		stats, err := db.GetStats(sourceAccountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("get stats: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"stats": stats,
		})
	}
}

// ---------------------------------------------------------------------------
// POST /v1/search
// ---------------------------------------------------------------------------

func handleSearch(osClient *OpenSubtitlesClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.Query == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("query is required"))
			return
		}
		if len(req.Languages) == 0 {
			req.Languages = []string{"en"}
		}

		results, err := osClient.SearchByQuery(req.Query, req.Languages)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("search failed: %w", err))
			return
		}
		if results == nil {
			results = []OSSearchResult{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"results": results,
			"count":   len(results),
		})
	}
}

// ---------------------------------------------------------------------------
// POST /v1/search/hash
// ---------------------------------------------------------------------------

func handleSearchHash(osClient *OpenSubtitlesClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req HashSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.MovieHash == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("moviehash is required"))
			return
		}
		if req.MovieByteSize < 1 {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("moviebytesize must be >= 1"))
			return
		}
		if len(req.Languages) == 0 {
			req.Languages = []string{"en"}
		}

		results, err := osClient.SearchByHash(req.MovieHash, req.MovieByteSize, req.Languages)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("hash search failed: %w", err))
			return
		}
		if results == nil {
			results = []OSSearchResult{}
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"results": results,
			"count":   len(results),
		})
	}
}

// ---------------------------------------------------------------------------
// POST /v1/download
// ---------------------------------------------------------------------------

