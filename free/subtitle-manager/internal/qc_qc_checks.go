package internal

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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


