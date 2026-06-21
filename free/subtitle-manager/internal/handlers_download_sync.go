package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	sdk "github.com/nself-org/plugin-sdk"
)

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

