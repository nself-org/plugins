package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	sdk "github.com/nself-org/plugin-sdk"
)

// RegisterRoutes mounts all subtitle-manager API routes on the given router.
func RegisterRoutes(r chi.Router, db *DB, cfg *Config, osClient *OpenSubtitlesClient) {
	syncer := NewSynchronizer(cfg)
	qc := NewSubtitleQC()
	norm := NewNormalizer()

	r.Route("/v1", func(r chi.Router) {
		r.Get("/subtitles", handleListSubtitles(db))
		r.Get("/downloads", handleListDownloads(db))
		r.Get("/stats", handleGetStats(db))
		r.Post("/search", handleSearch(osClient))
		r.Post("/search/hash", handleSearchHash(osClient))
		r.Post("/download", handleDownload(db, cfg, osClient, qc))
		r.Post("/sync", handleSync(cfg, syncer))
		r.Post("/qc", handleQC(db, qc))
		r.Post("/normalize", handleNormalize(norm))
		r.Post("/fetch-best", handleFetchBest(db, cfg, osClient, syncer, norm))
		r.Delete("/downloads/{id}", handleDeleteDownload(db))
	})
}

// ---------------------------------------------------------------------------
// GET /v1/subtitles
// ---------------------------------------------------------------------------

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

func handleDownload(db *DB, cfg *Config, osClient *OpenSubtitlesClient, qc *SubtitleQC) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req DownloadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.FileID < 1 {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("file_id must be >= 1"))
			return
		}
		if req.MediaID == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("media_id is required"))
			return
		}

		sourceAccountID := getSourceAccountID(r)
		lang := req.Language
		if lang == "" {
			lang = "en"
		}
		mediaType := req.MediaType
		if mediaType == "" {
			mediaType = "movie"
		}

		// Check if already downloaded (cache hit)
		existing, err := db.GetDownloadByMediaID(req.MediaID, lang, sourceAccountID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("check cache: %w", err))
			return
		}
		if existing != nil {
			sdk.Respond(w, http.StatusOK, map[string]interface{}{
				"success":  true,
				"download": existing,
				"source":   "cache",
			})
			return
		}

		// Download from OpenSubtitles
		subtitleData, err := osClient.DownloadSubtitle(req.FileID)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("download subtitle: %w", err))
			return
		}
		if subtitleData == nil {
			sdk.Error(w, http.StatusNotFound, fmt.Errorf("subtitle not found or download failed"))
			return
		}

		// Save to disk
		dir := filepath.Join(cfg.StoragePath, sourceAccountID, req.MediaID)
		if err := os.MkdirAll(dir, 0755); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("create storage dir: %w", err))
			return
		}
		filePath := filepath.Join(dir, lang+".srt")
		if err := os.WriteFile(filePath, subtitleData, 0644); err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("write subtitle file: %w", err))
			return
		}

		log.Printf("subtitle-manager: saved subtitle to disk path=%s bytes=%d", filePath, len(subtitleData))

		// Track in database
		download, err := db.InsertDownload(InsertDownloadInput{
			SourceAccountID:     sourceAccountID,
			MediaID:             req.MediaID,
			MediaType:           mediaType,
			MediaTitle:          req.MediaTitle,
			Language:            lang,
			FilePath:            filePath,
			FileSizeBytes:       int64(len(subtitleData)),
			OpensubtitlesFileID: req.FileID,
			Source:              "opensubtitles",
		})
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("insert download: %w", err))
			return
		}

		// Optionally run QC
		var qcResult *QCResult
		if req.RunQC {
			result, qcErr := qc.ValidateSubtitle(filePath, nil)
			if qcErr != nil {
				log.Printf("subtitle-manager: QC after download failed: %v", qcErr)
			} else {
				qcResult = result
				_, insertErr := db.InsertQCResult(InsertQCResultInput{
					SourceAccountID: sourceAccountID,
					DownloadID:      download.ID,
					Status:          result.Status,
					Checks:          result.Checks,
					Issues:          result.Issues,
					CueCount:        result.CueCount,
					TotalDurationMs: result.TotalDurationMs,
				})
				if insertErr != nil {
					log.Printf("subtitle-manager: insert QC result failed: %v", insertErr)
				}
				updateErr := db.UpdateDownloadQC(download.ID, result.Status, QualityCheckDetails{
					CueCount:   result.CueCount,
					IssueCount: len(result.Issues),
				})
				if updateErr != nil {
					log.Printf("subtitle-manager: update download QC failed: %v", updateErr)
				}
			}
		}

		resp := map[string]interface{}{
			"success":  true,
			"download": download,
			"source":   "opensubtitles",
		}
		if qcResult != nil {
			resp["qc"] = qcResult
		}
		sdk.Respond(w, http.StatusOK, resp)
	}
}

// ---------------------------------------------------------------------------
// POST /v1/sync
// ---------------------------------------------------------------------------

func handleSync(cfg *Config, syncer *Synchronizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.VideoPath == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("video_path is required"))
			return
		}
		if req.SubtitlePath == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("subtitle_path is required"))
			return
		}

		lang := req.Language
		if lang == "" {
			lang = "en"
		}
		sourceAccountID := getSourceAccountID(r)

		outputDir := filepath.Join(cfg.StoragePath, sourceAccountID, "synced")
		baseName := strings.TrimSuffix(filepath.Base(req.SubtitlePath), filepath.Ext(req.SubtitlePath))
		outputPath := filepath.Join(outputDir, fmt.Sprintf("%s.synced.%s.srt", baseName, lang))

		result, err := syncer.SyncSubtitle(req.VideoPath, req.SubtitlePath, outputPath, nil)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("sync failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"result":  result,
		})
	}
}

// ---------------------------------------------------------------------------
// POST /v1/qc
// ---------------------------------------------------------------------------

func handleQC(db *DB, qc *SubtitleQC) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req QCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.SubtitlePath == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("subtitle_path is required"))
			return
		}

		sourceAccountID := getSourceAccountID(r)

		result, err := qc.ValidateSubtitle(req.SubtitlePath, req.VideoDurationMs)
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("QC validation failed: %w", err))
			return
		}

		// If download_id is provided, store result and update download record
		if req.DownloadID != "" {
			_, insertErr := db.InsertQCResult(InsertQCResultInput{
				SourceAccountID: sourceAccountID,
				DownloadID:      req.DownloadID,
				Status:          result.Status,
				Checks:          result.Checks,
				Issues:          result.Issues,
				CueCount:        result.CueCount,
				TotalDurationMs: result.TotalDurationMs,
			})
			if insertErr != nil {
				log.Printf("subtitle-manager: insert QC result failed: %v", insertErr)
			}
			updateErr := db.UpdateDownloadQC(req.DownloadID, result.Status, QualityCheckDetails{
				CueCount:   result.CueCount,
				IssueCount: len(result.Issues),
			})
			if updateErr != nil {
				log.Printf("subtitle-manager: update download QC failed: %v", updateErr)
			}
		}

		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"result":  result,
		})
	}
}

// ---------------------------------------------------------------------------
// POST /v1/normalize
// ---------------------------------------------------------------------------

func handleNormalize(norm *Normalizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req NormalizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
		if req.InputPath == "" {
			sdk.Error(w, http.StatusBadRequest, fmt.Errorf("input_path is required"))
			return
		}

		outputPath, err := norm.NormalizeToWebVTT(req.InputPath, "")
		if err != nil {
			sdk.Error(w, http.StatusInternalServerError, fmt.Errorf("normalization failed: %w", err))
			return
		}
		sdk.Respond(w, http.StatusOK, map[string]interface{}{
			"success":     true,
			"output_path": outputPath,
		})
	}
}

// ---------------------------------------------------------------------------
// POST /v1/fetch-best
// ---------------------------------------------------------------------------

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
					videoPath:       req.VideoPath,
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

func fetchBestForLanguage(p fetchBestParams) FetchBestLanguageResult {
	const syncThresholdMs = 500.0

	failed := FetchBestLanguageResult{
		Language:    p.language,
		Path:        nil,
		Format:      "none",
		SyncQuality: "failed",
		SyncWarning: true,
		OffsetMs:    0,
		ToolUsed:    "none",
	}

	// Step 1: Search for subtitles
	searchQuery := p.mediaTitle
	if searchQuery == "" {
		base := filepath.Base(p.videoPath)
		searchQuery = strings.TrimSuffix(base, filepath.Ext(base))
	}

	searchResults, err := p.osClient.SearchByQuery(searchQuery, []string{p.language})
	if err != nil || len(searchResults) == 0 {
		log.Printf("subtitle-manager: no subtitles found for language=%s query=%s", p.language, searchQuery)
		return failed
	}

	// Filter to results with files, rank by downloads and rating
	filtered := make([]OSSearchResult, 0, len(searchResults))
	for _, r := range searchResults {
		if len(r.Attributes.Files) > 0 {
			filtered = append(filtered, r)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		aScore := float64(filtered[i].Attributes.DownloadCount)*0.4 + filtered[i].Attributes.Ratings*0.3
		bScore := float64(filtered[j].Attributes.DownloadCount)*0.4 + filtered[j].Attributes.Ratings*0.3
		return bScore < aScore
	})

	if len(filtered) > p.maxAlternatives {
		filtered = filtered[:p.maxAlternatives]
	}

	type bestCandidate struct {
		path        string
		offsetMs    float64
		toolUsed    string
		syncQuality string
	}
	var best *bestCandidate
	bestOffsetMs := math.Inf(1)

	// Step 2-5: Try each alternative subtitle
	for _, result := range filtered {
		if len(result.Attributes.Files) == 0 {
			continue
		}
		fileID := result.Attributes.Files[0].FileID
		if fileID == 0 {
			continue
		}

		subtitleData, dlErr := p.osClient.DownloadSubtitle(fileID)
		if dlErr != nil || subtitleData == nil {
			continue
		}

		// Save raw to temp location
		mediaDir := p.mediaID
		if mediaDir == "" {
			mediaDir = "temp"
		}
		dir := filepath.Join(p.cfg.StoragePath, p.sourceAccountID, mediaDir, p.language)
		if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
			continue
		}
		rawPath := filepath.Join(dir, fmt.Sprintf("raw_%d.srt", fileID))
		if wErr := os.WriteFile(rawPath, subtitleData, 0644); wErr != nil {
			continue
		}

		// Try sync
		syncedPath := filepath.Join(dir, fmt.Sprintf("synced_%d.srt", fileID))
		offsetMs := float64(0)
		toolUsed := "none"

		// Try alass first
		syncRes, syncErr := p.syncer.SyncSubtitle(p.videoPath, rawPath, syncedPath, &SyncOptions{AlassOnly: true})
		if syncErr == nil {
			offsetMs = math.Abs(syncRes.OffsetMs)
			toolUsed = "alass"

			// If offset > threshold, try ffsubsync
			if offsetMs > syncThresholdMs {
				ffRes, ffErr := p.syncer.SyncSubtitle(p.videoPath, rawPath, syncedPath, &SyncOptions{FfsubsyncOnly: true})
				if ffErr == nil {
					ffOffset := math.Abs(ffRes.OffsetMs)
					if ffOffset < offsetMs {
						offsetMs = ffOffset
						toolUsed = "ffsubsync"
					}
				}
			}
		} else {
			// Sync failed, use raw file
			copyFile(rawPath, syncedPath)
			toolUsed = "raw"
		}

		// Track best result
		if offsetMs < bestOffsetMs {
			bestOffsetMs = offsetMs
			quality := "warning"
			if offsetMs <= syncThresholdMs {
				quality = "good"
			}
			best = &bestCandidate{
				path:        syncedPath,
				offsetMs:    offsetMs,
				toolUsed:    toolUsed,
				syncQuality: quality,
			}
		}

		// If good sync achieved, stop trying alternatives
		if offsetMs <= syncThresholdMs {
			break
		}
	}

	if best == nil {
		return failed
	}

	// Step 6: Normalize to WebVTT
	vttPath, normErr := p.norm.NormalizeToWebVTT(best.path, "")
	if normErr != nil {
		vttPath = best.path
	}

	// Track download in database
	if p.mediaID != "" {
		syncScore := 1.0
		if best.syncQuality != "good" {
			syncScore = 0.5
		}
		scorePtr := &syncScore
		_, dbErr := p.db.InsertDownload(InsertDownloadInput{
			SourceAccountID: p.sourceAccountID,
			MediaID:         p.mediaID,
			MediaType:       p.mediaType,
			MediaTitle:      p.mediaTitle,
			Language:        p.language,
			FilePath:        vttPath,
			Source:          "opensubtitles",
			SyncScore:       scorePtr,
		})
		if dbErr != nil {
			// DB tracking failure is non-critical
			log.Printf("subtitle-manager: fetch-best DB track failed: %v", dbErr)
		}
	}

	format := "srt"
	if strings.HasSuffix(vttPath, ".vtt") {
		format = "webvtt"
	}

	return FetchBestLanguageResult{
		Language:    p.language,
		Path:        &vttPath,
		Format:      format,
		SyncQuality: best.syncQuality,
		SyncWarning: best.syncQuality != "good",
		OffsetMs:    best.offsetMs,
		ToolUsed:    best.toolUsed,
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// getSourceAccountID extracts the source account ID from the X-Source-Account-ID header.
func getSourceAccountID(r *http.Request) string {
	id := r.Header.Get("X-Source-Account-ID")
	if id == "" {
		return "primary"
	}
	return id
}

func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
