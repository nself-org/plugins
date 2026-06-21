package internal

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// SubtitleQC performs deterministic quality checks on subtitle files.
type SubtitleQC struct{}

// NewSubtitleQC creates a new SubtitleQC instance.
func NewSubtitleQC() *SubtitleQC {
	return &SubtitleQC{}
}

// ValidateSubtitle runs all QC checks against a subtitle file.
func (q *SubtitleQC) ValidateSubtitle(subtitlePath string, videoDurationMs *int64) (*QCResult, error) {
	log.Printf("subtitle-manager: running QC validation path=%s", subtitlePath)

	data, err := os.ReadFile(subtitlePath)
	if err != nil {
		return nil, fmt.Errorf("read subtitle file: %w", err)
	}

	content := string(data)
	isVTT := strings.HasSuffix(strings.ToLower(subtitlePath), ".vtt")

	var cues []SubtitleCue
	if isVTT {
		cues = parseVTTCues(content)
	} else {
		cues = parseSRTCues(content)
	}

	var checks []QCCheck
	var issues []QCIssue

	// Check 1: Timestamps within [0, video_duration]
	if videoDurationMs != nil {
		c, iss := checkTimestampsInRange(cues, *videoDurationMs)
		checks = append(checks, c)
		issues = append(issues, iss...)
	}

	// Check 2: First cue within first 10 minutes
	{
		c, iss := checkFirstCueEarly(cues)
		checks = append(checks, c)
		issues = append(issues, iss...)
	}

	// Check 3: Last cue within 5 minutes of video end
	if videoDurationMs != nil {
		c, iss := checkLastCueNearEnd(cues, *videoDurationMs)
		checks = append(checks, c)
		issues = append(issues, iss...)
	}

	// Check 4: No negative cue durations
	{
		c, iss := checkNoNegativeDurations(cues)
		checks = append(checks, c)
		issues = append(issues, iss...)
	}

	// Check 5: No massive overlap rate (>10%)
	{
		c, iss := checkOverlapRate(cues)
		checks = append(checks, c)
		issues = append(issues, iss...)
	}

	// Check 6: CPS within 5-35
	{
		c, iss := checkCPS(cues)
		checks = append(checks, c)
		issues = append(issues, iss...)
	}

	// Check 7: Line length heuristic
	{
		c, iss := checkLineLength(cues)
		checks = append(checks, c)
		issues = append(issues, iss...)
	}

	// Determine overall status
	hasErrors := false
	hasWarnings := false
	for _, iss := range issues {
		if iss.Severity == "error" {
			hasErrors = true
		}
		if iss.Severity == "warning" {
			hasWarnings = true
		}
	}

	status := "pass"
	if hasErrors {
		status = "fail"
	} else if hasWarnings {
		status = "warn"
	}

	var totalDurationMs int64
	if len(cues) > 0 {
		minStart := cues[0].StartMs
		maxEnd := cues[0].EndMs
		for _, c := range cues {
			if c.StartMs < minStart {
				minStart = c.StartMs
			}
			if c.EndMs > maxEnd {
				maxEnd = c.EndMs
			}
		}
		totalDurationMs = maxEnd - minStart
	}

	result := &QCResult{
		Status:          status,
		Checks:          checks,
		Issues:          issues,
		CueCount:        len(cues),
		TotalDurationMs: totalDurationMs,
	}

	log.Printf("subtitle-manager: QC complete status=%s cues=%d issues=%d", status, len(cues), len(issues))
	return result, nil
}

