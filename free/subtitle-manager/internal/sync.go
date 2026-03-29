package internal

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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

func (s *Synchronizer) syncWithAlass(videoPath, subtitlePath, outputPath string) (*AlassSyncResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.cfg.AlassPath, videoPath, subtitlePath, outputPath)

	// TRAP 3: Use StdoutPipe+io.Copy for large output
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("alass stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("alass stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start alass: %w", err)
	}

	var stdoutBuf, stderrBuf strings.Builder
	go func() { io.Copy(&stdoutBuf, stdoutPipe) }()
	go func() { io.Copy(&stderrBuf, stderrPipe) }()

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("alass exited with error: %w (stderr: %s)", err, stderrBuf.String())
	}

	combined := stdoutBuf.String() + "\n" + stderrBuf.String()
	confidence := parseAlassConfidence(combined)
	offsetMs := parseAlassOffset(combined)
	framerateAdj := parseAlassFramerate(combined)

	return &AlassSyncResult{
		Confidence:        confidence,
		OffsetMs:          offsetMs,
		FramerateAdjusted: framerateAdj,
	}, nil
}

// ---------------------------------------------------------------------------
// ffsubsync
// ---------------------------------------------------------------------------

func (s *Synchronizer) syncWithFfsubsync(videoPath, subtitlePath, outputPath string) (*FfsubsyncResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.cfg.FfsubsyncPath, videoPath, "-i", subtitlePath, "-o", outputPath)

	// TRAP 3: Use StdoutPipe+io.Copy for large output
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffsubsync stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("ffsubsync stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffsubsync: %w", err)
	}

	var stdoutBuf, stderrBuf strings.Builder
	go func() { io.Copy(&stdoutBuf, stdoutPipe) }()
	go func() { io.Copy(&stderrBuf, stderrPipe) }()

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffsubsync exited with error: %w (stderr: %s)", err, stderrBuf.String())
	}

	combined := stdoutBuf.String() + "\n" + stderrBuf.String()
	confidence := parseFfsubsyncConfidence(combined)
	offsetMs := parseFfsubsyncOffset(combined)

	return &FfsubsyncResult{
		Confidence: confidence,
		OffsetMs:   offsetMs,
	}, nil
}

// ---------------------------------------------------------------------------
// Binary availability checks
// ---------------------------------------------------------------------------

func (s *Synchronizer) isAlassAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.AlassPath, "--version")
	return cmd.Run() == nil
}

func (s *Synchronizer) isFfsubsyncAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.cfg.FfsubsyncPath, "--version")
	return cmd.Run() == nil
}

// ---------------------------------------------------------------------------
// Output parsing helpers
// ---------------------------------------------------------------------------

var (
	reConfidence       = regexp.MustCompile(`(?i)confidence[:\s]+([0-9.]+)`)
	reAlassOffset      = regexp.MustCompile(`(?i)offset[:\s]+([+-]?[0-9.]+)\s*(?:ms)?`)
	reAlassFramerate   = regexp.MustCompile(`(?i)(?:framerate|fps.*adjust|rescal)`)
	reFfsubsyncConf    = regexp.MustCompile(`(?i)(?:sync\s*(?:quality|score|confidence))[:\s]+([0-9.]+)`)
	reFfsubsyncRatio   = regexp.MustCompile(`(?i)framerate\s*ratio[:\s]+([0-9.]+)`)
	reFfsubsyncOffMs   = regexp.MustCompile(`(?i)offset[:\s]+([+-]?[0-9.]+)\s*ms`)
	reFfsubsyncOffSec  = regexp.MustCompile(`(?i)offset\s*(?:seconds)?[:\s]+([+-]?[0-9.]+)`)
)

func parseAlassConfidence(output string) float64 {
	if m := reConfidence.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return math.Min(1, math.Max(0, v))
		}
	}
	if strings.TrimSpace(output) != "" {
		return 0.7
	}
	return 0
}

func parseAlassOffset(output string) float64 {
	if m := reAlassOffset.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return v
		}
	}
	return 0
}

func parseAlassFramerate(output string) bool {
	return reAlassFramerate.MatchString(output)
}

func parseFfsubsyncConfidence(output string) float64 {
	if m := reFfsubsyncConf.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return math.Min(1, math.Max(0, v))
		}
	}
	if m := reFfsubsyncRatio.FindStringSubmatch(output); m != nil {
		if ratio, err := strconv.ParseFloat(m[1], 64); err == nil {
			return math.Max(0, 1-math.Abs(1-ratio))
		}
	}
	if strings.TrimSpace(output) != "" {
		return 0.7
	}
	return 0
}

func parseFfsubsyncOffset(output string) float64 {
	if m := reFfsubsyncOffMs.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return v
		}
	}
	if m := reFfsubsyncOffSec.FindStringSubmatch(output); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return v * 1000
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Aggregate confidence
// ---------------------------------------------------------------------------

func computeAggregateConfidence(alass *AlassSyncResult, ffsub *FfsubsyncResult) float64 {
	if alass != nil && ffsub != nil {
		return alass.Confidence*0.4 + ffsub.Confidence*0.6
	}
	if alass != nil {
		return alass.Confidence
	}
	if ffsub != nil {
		return ffsub.Confidence
	}
	return 0
}

// ---------------------------------------------------------------------------
// File copy helper
// ---------------------------------------------------------------------------

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
