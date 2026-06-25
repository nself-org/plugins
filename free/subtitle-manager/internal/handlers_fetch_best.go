package internal

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Size-cap exception: single-responsibility HTTP route handler — 179L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
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
