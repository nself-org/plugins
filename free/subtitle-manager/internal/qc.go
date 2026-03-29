package internal

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
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

// ---------------------------------------------------------------------------
// SRT parser
// ---------------------------------------------------------------------------

var reSRTTimestamp = regexp.MustCompile(
	`(\d{1,2}):(\d{2}):(\d{2})[,.](\d{3})\s*-->\s*(\d{1,2}):(\d{2}):(\d{2})[,.](\d{3})`)

func parseSRTCues(content string) []SubtitleCue {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	blocks := splitBlocks(normalized)
	var cues []SubtitleCue

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		if len(lines) < 2 {
			continue
		}

		tsLineIdx := -1
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				tsLineIdx = i
				break
			}
		}
		if tsLineIdx < 0 {
			continue
		}

		m := reSRTTimestamp.FindStringSubmatch(lines[tsLineIdx])
		if m == nil {
			continue
		}

		startMs := parseTimeParts(m[1], m[2], m[3], m[4])
		endMs := parseTimeParts(m[5], m[6], m[7], m[8])

		indexStr := ""
		if tsLineIdx > 0 {
			indexStr = strings.TrimSpace(lines[0])
		}
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			index = len(cues) + 1
		}

		textLines := lines[tsLineIdx+1:]
		text := strings.TrimSpace(strings.Join(textLines, "\n"))

		cues = append(cues, SubtitleCue{
			Index:   index,
			StartMs: startMs,
			EndMs:   endMs,
			Text:    text,
		})
	}
	return cues
}

// ---------------------------------------------------------------------------
// WebVTT parser
// ---------------------------------------------------------------------------

var reVTTTimestamp = regexp.MustCompile(
	`(?:(\d{1,2}):)?(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(?:(\d{1,2}):)?(\d{2}):(\d{2})\.(\d{3})`)

func parseVTTCues(content string) []SubtitleCue {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	// Remove WEBVTT header
	headerEnd := strings.Index(normalized, "\n\n")
	if headerEnd < 0 {
		return nil
	}
	body := normalized[headerEnd+2:]

	blocks := splitBlocks(body)
	var cues []SubtitleCue
	cueIndex := 1

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		if len(lines) < 2 {
			continue
		}

		tsLineIdx := -1
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				tsLineIdx = i
				break
			}
		}
		if tsLineIdx < 0 {
			continue
		}

		m := reVTTTimestamp.FindStringSubmatch(lines[tsLineIdx])
		if m == nil {
			continue
		}

		h1 := m[1]
		if h1 == "" {
			h1 = "0"
		}
		h2 := m[5]
		if h2 == "" {
			h2 = "0"
		}

		startMs := parseTimeParts(h1, m[2], m[3], m[4])
		endMs := parseTimeParts(h2, m[6], m[7], m[8])

		textLines := lines[tsLineIdx+1:]
		text := strings.TrimSpace(strings.Join(textLines, "\n"))

		cues = append(cues, SubtitleCue{
			Index:   cueIndex,
			StartMs: startMs,
			EndMs:   endMs,
			Text:    text,
		})
		cueIndex++
	}
	return cues
}

// ---------------------------------------------------------------------------
// QC check implementations
// ---------------------------------------------------------------------------

func checkTimestampsInRange(cues []SubtitleCue, videoDurationMs int64) (QCCheck, []QCIssue) {
	var issues []QCIssue
	allInRange := true

	for _, cue := range cues {
		if cue.StartMs < 0 || cue.EndMs < 0 {
			allInRange = false
			idx := cue.Index
			issues = append(issues, QCIssue{
				Severity: "error",
				Check:    "timestamps_in_range",
				CueIndex: &idx,
				Message:  fmt.Sprintf("Cue %d has negative timestamp: start=%dms, end=%dms", cue.Index, cue.StartMs, cue.EndMs),
			})
		}
		if cue.EndMs > videoDurationMs+5000 {
			allInRange = false
			idx := cue.Index
			issues = append(issues, QCIssue{
				Severity: "error",
				Check:    "timestamps_in_range",
				CueIndex: &idx,
				Message:  fmt.Sprintf("Cue %d extends beyond video duration: endMs=%d > videoDuration=%dms", cue.Index, cue.EndMs, videoDurationMs),
			})
		}
	}

	msg := fmt.Sprintf("All %d cues within video duration", len(cues))
	if !allInRange {
		msg = fmt.Sprintf("%d cue(s) have out-of-range timestamps", len(issues))
	}

	return QCCheck{
		Name:    "timestamps_in_range",
		Passed:  allInRange,
		Message: msg,
	}, issues
}

func checkFirstCueEarly(cues []SubtitleCue) (QCCheck, []QCIssue) {
	var issues []QCIssue
	const maxFirstCueMs int64 = 600000 // 10 minutes

	if len(cues) == 0 {
		issues = append(issues, QCIssue{
			Severity: "error",
			Check:    "first_cue_early",
			Message:  "No cues found in subtitle file",
		})
		return QCCheck{Name: "first_cue_early", Passed: false, Message: "No cues found"}, issues
	}

	first := cues[0]
	passed := first.StartMs <= maxFirstCueMs
	if !passed {
		idx := first.Index
		issues = append(issues, QCIssue{
			Severity: "error",
			Check:    "first_cue_early",
			CueIndex: &idx,
			Message:  fmt.Sprintf("First cue starts at %dms (%.1f min), expected within first 10 minutes", first.StartMs, float64(first.StartMs)/60000),
		})
	}

	msg := fmt.Sprintf("First cue at %dms", first.StartMs)
	if !passed {
		msg = fmt.Sprintf("First cue too late at %dms", first.StartMs)
	}

	return QCCheck{Name: "first_cue_early", Passed: passed, Message: msg}, issues
}

func checkLastCueNearEnd(cues []SubtitleCue, videoDurationMs int64) (QCCheck, []QCIssue) {
	var issues []QCIssue
	const maxGapMs int64 = 300000 // 5 minutes

	if len(cues) == 0 {
		issues = append(issues, QCIssue{
			Severity: "error",
			Check:    "last_cue_near_end",
			Message:  "No cues found",
		})
		return QCCheck{Name: "last_cue_near_end", Passed: false, Message: "No cues found"}, issues
	}

	last := cues[len(cues)-1]
	gap := videoDurationMs - last.EndMs
	passed := gap <= maxGapMs

	if !passed {
		idx := last.Index
		issues = append(issues, QCIssue{
			Severity: "error",
			Check:    "last_cue_near_end",
			CueIndex: &idx,
			Message:  fmt.Sprintf("Last cue ends at %dms, %.1f min before video end (%dms). Max gap: 5 min", last.EndMs, float64(gap)/60000, videoDurationMs),
		})
	}

	msg := fmt.Sprintf("Last cue ends %.1fs before video end", float64(gap)/1000)
	if !passed {
		msg = fmt.Sprintf("Last cue ends %.1f min before video end", float64(gap)/60000)
	}

	return QCCheck{Name: "last_cue_near_end", Passed: passed, Message: msg}, issues
}

func checkNoNegativeDurations(cues []SubtitleCue) (QCCheck, []QCIssue) {
	var issues []QCIssue

	for _, cue := range cues {
		if cue.EndMs < cue.StartMs {
			idx := cue.Index
			issues = append(issues, QCIssue{
				Severity: "error",
				Check:    "no_negative_durations",
				CueIndex: &idx,
				Message:  fmt.Sprintf("Cue %d has negative duration: start=%dms > end=%dms", cue.Index, cue.StartMs, cue.EndMs),
			})
		}
	}

	passed := len(issues) == 0
	msg := "No negative durations found"
	if !passed {
		msg = fmt.Sprintf("%d cue(s) have negative durations", len(issues))
	}

	return QCCheck{Name: "no_negative_durations", Passed: passed, Message: msg}, issues
}

func checkOverlapRate(cues []SubtitleCue) (QCCheck, []QCIssue) {
	var issues []QCIssue
	overlapCount := 0

	for i := 0; i < len(cues)-1; i++ {
		if cues[i].EndMs > cues[i+1].StartMs {
			overlapCount++
			if len(issues) < 20 {
				idx := cues[i].Index
				issues = append(issues, QCIssue{
					Severity: "warning",
					Check:    "overlap_rate",
					CueIndex: &idx,
					Message:  fmt.Sprintf("Cue %d overlaps with cue %d: end=%dms > nextStart=%dms", cues[i].Index, cues[i+1].Index, cues[i].EndMs, cues[i+1].StartMs),
				})
			}
		}
	}

	overlapRate := float64(0)
	if len(cues) > 1 {
		overlapRate = float64(overlapCount) / float64(len(cues)-1)
	}
	passed := overlapRate <= 0.10

	if !passed {
		// Promote severity to error
		for i := range issues {
			issues[i].Severity = "error"
		}
	}

	msg := fmt.Sprintf("Overlap rate: %.1f%% (%d of %d cues)", overlapRate*100, overlapCount, len(cues))
	if !passed {
		msg = fmt.Sprintf("Excessive overlap rate: %.1f%% (%d of %d cues) exceeds 10%% threshold", overlapRate*100, overlapCount, len(cues))
	}

	return QCCheck{Name: "overlap_rate", Passed: passed, Message: msg}, issues
}

var reHTMLTags = regexp.MustCompile(`<[^>]+>`)
var reASSTags = regexp.MustCompile(`\{[^}]+\}`)

func checkCPS(cues []SubtitleCue) (QCCheck, []QCIssue) {
	var issues []QCIssue
	const minCPS = 5.0
	const maxCPS = 35.0
	outOfBounds := 0

	for _, cue := range cues {
		durationSec := float64(cue.EndMs-cue.StartMs) / 1000
		if durationSec <= 0 {
			continue
		}

		plainText := reHTMLTags.ReplaceAllString(cue.Text, "")
		plainText = reASSTags.ReplaceAllString(plainText, "")
		charCount := len([]rune(plainText))
		if charCount == 0 {
			continue
		}

		cps := float64(charCount) / durationSec
		if cps < minCPS || cps > maxCPS {
			outOfBounds++
			if len(issues) < 20 {
				idx := cue.Index
				issues = append(issues, QCIssue{
					Severity: "warning",
					Check:    "cps_bounds",
					CueIndex: &idx,
					Message:  fmt.Sprintf("Cue %d has %.1f CPS (%d chars / %.1fs). Expected %.0f-%.0f CPS", cue.Index, cps, charCount, durationSec, minCPS, maxCPS),
				})
			}
		}
	}

	passed := outOfBounds == 0
	msg := "All cues within CPS bounds (5-35)"
	if !passed {
		msg = fmt.Sprintf("%d cue(s) outside CPS bounds (5-35)", outOfBounds)
	}

	return QCCheck{Name: "cps_bounds", Passed: passed, Message: msg}, issues
}

func checkLineLength(cues []SubtitleCue) (QCCheck, []QCIssue) {
	var issues []QCIssue
	const maxLineLen = 80

	for _, cue := range cues {
		lines := strings.Split(cue.Text, "\n")
		for _, line := range lines {
			plainLine := reHTMLTags.ReplaceAllString(line, "")
			plainLine = reASSTags.ReplaceAllString(plainLine, "")
			if len([]rune(plainLine)) > maxLineLen {
				idx := cue.Index
				issues = append(issues, QCIssue{
					Severity: "warning",
					Check:    "line_length",
					CueIndex: &idx,
					Message:  fmt.Sprintf("Cue %d has line with %d chars (max %d)", cue.Index, len([]rune(plainLine)), maxLineLen),
				})
				break // one warning per cue
			}
		}
	}

	passed := len(issues) == 0
	msg := fmt.Sprintf("All lines within %d character limit", maxLineLen)
	if !passed {
		msg = fmt.Sprintf("%d cue(s) have lines exceeding %d characters", len(issues), maxLineLen)
	}

	return QCCheck{Name: "line_length", Passed: passed, Message: msg}, issues
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func splitBlocks(s string) []string {
	// Split on two or more consecutive newlines
	re := regexp.MustCompile(`\n\n+`)
	return re.Split(s, -1)
}

func parseTimeParts(h, m, s, ms string) int64 {
	hours, _ := strconv.ParseInt(h, 10, 64)
	mins, _ := strconv.ParseInt(m, 10, 64)
	secs, _ := strconv.ParseInt(s, 10, 64)
	millis, _ := strconv.ParseInt(ms, 10, 64)
	return hours*3600000 + mins*60000 + secs*1000 + millis
}

