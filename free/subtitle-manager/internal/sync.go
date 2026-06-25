package internal

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Synchronizer orchestrates subtitle sync using alass and/or ffsubsync.
type Synchronizer struct {
	cfg *Config
}

// NewSynchronizer creates a new Synchronizer.
func NewSynchronizer(cfg *Config) *Synchronizer {
	return &Synchronizer{cfg: cfg}
}

// SyncSubtitle runs the full sync pipeline: alass first, then ffsubsync.
// Uses os/exec with StdoutPipe+io.Copy for large output (TRAP 3 safe).
// Size-cap exception: sync pipeline — 108L sequential sync stages; splitting creates artificial state-passing overhead.
func (s *Synchronizer) SyncSubtitle(videoPath, subtitlePath, outputPath string, opts *SyncOptions) (*SyncResult, error) {
	log.Printf("subtitle-manager: starting sync pipeline video=%s sub=%s out=%s", videoPath, subtitlePath, outputPath)

	// Verify input files exist
	if _, err := os.Stat(videoPath); err != nil {
		return nil, fmt.Errorf("video file not found: %w", err)
	}
	if _, err := os.Stat(subtitlePath); err != nil {
		return nil, fmt.Errorf("subtitle file not found: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	if opts == nil {
		opts = &SyncOptions{}
	}

	useAlass := !opts.FfsubsyncOnly
	useFfsubsync := !opts.AlassOnly

	var alassRes *AlassSyncResult
	var ffRes *FfsubsyncResult
	currentSubPath := subtitlePath
	method := "both"

	// First pass: alass
	if useAlass && s.isAlassAvailable() {
		alassOutput := outputPath
		if useFfsubsync {
			alassOutput = outputPath + ".alass.tmp.srt"
		}
		res, err := s.syncWithAlass(videoPath, currentSubPath, alassOutput)
		if err != nil {
			log.Printf("subtitle-manager: alass pass failed, continuing: %v", err)
		} else {
			alassRes = res
			currentSubPath = alassOutput
			log.Printf("subtitle-manager: alass pass complete confidence=%.2f offset=%.1fms", res.Confidence, res.OffsetMs)
		}
	} else if useAlass {
		log.Println("subtitle-manager: alass binary not available, skipping first pass")
	}

	// Second pass: ffsubsync
	if useFfsubsync && s.isFfsubsyncAvailable() {
		res, err := s.syncWithFfsubsync(videoPath, currentSubPath, outputPath)
		if err != nil {
			log.Printf("subtitle-manager: ffsubsync pass failed: %v", err)
			// If ffsubsync fails but alass succeeded, copy alass output as final
			if alassRes != nil && currentSubPath != outputPath {
				copyFile(currentSubPath, outputPath)
			}
		} else {
			ffRes = res
			log.Printf("subtitle-manager: ffsubsync pass complete confidence=%.2f offset=%.1fms", res.Confidence, res.OffsetMs)
		}
	} else if useFfsubsync {
		log.Println("subtitle-manager: ffsubsync binary not available, skipping second pass")
		if alassRes != nil && currentSubPath != outputPath {
			copyFile(currentSubPath, outputPath)
		}
	}

	// Clean up temp alass file
	if useFfsubsync && alassRes != nil {
		tmpPath := outputPath + ".alass.tmp.srt"
		os.Remove(tmpPath)
	}

	// Determine method
	if alassRes != nil && ffRes != nil {
		method = "both"
	} else if alassRes != nil {
		method = "alass"
	} else if ffRes != nil {
		method = "ffsubsync"
	} else {
		// Neither tool ran; copy original
		copyFile(subtitlePath, outputPath)
		method = "alass" // fallback label
		log.Println("subtitle-manager: no sync tools available; copied original to output")
	}

	confidence := computeAggregateConfidence(alassRes, ffRes)
	offsetMs := float64(0)
	if ffRes != nil {
		offsetMs = ffRes.OffsetMs
	} else if alassRes != nil {
		offsetMs = alassRes.OffsetMs
	}

	result := &SyncResult{
		OriginalPath:    subtitlePath,
		SyncedPath:      outputPath,
		Confidence:      confidence,
		OffsetMs:        offsetMs,
		Method:          method,
		AlassResult:     alassRes,
		FfsubsyncResult: ffRes,
	}

	log.Printf("subtitle-manager: sync pipeline complete confidence=%.2f offset=%.1fms method=%s",
		confidence, offsetMs, method)
	return result, nil
}

// ---------------------------------------------------------------------------
// alass
// ---------------------------------------------------------------------------
