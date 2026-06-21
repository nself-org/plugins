package internal

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

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
