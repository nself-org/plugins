package internal

import (
	"regexp"
	"strings"
)

func srtToWebVTT(content string) string {
	var lines []string
	lines = append(lines, "WEBVTT", "")

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	re := regexp.MustCompile(`\n\n+`)
	blocks := re.Split(normalized, -1)

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		blockLines := strings.Split(block, "\n")
		if len(blockLines) < 2 {
			continue
		}

		tsLineIdx := -1
		for i, line := range blockLines {
			if strings.Contains(line, "-->") {
				tsLineIdx = i
				break
			}
		}
		if tsLineIdx < 0 {
			continue
		}

		// Convert timestamp: replace commas with dots
		timestampLine := strings.ReplaceAll(blockLines[tsLineIdx], ",", ".")

		// Collect and clean text lines
		textLines := blockLines[tsLineIdx+1:]
		var cleanedLines []string
		for _, tl := range textLines {
			cleanedLines = append(cleanedLines, cleanSRTTags(tl))
		}

		lines = append(lines, timestampLine)
		lines = append(lines, strings.Join(cleanedLines, "\n"))
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// cleanSRTTags removes SRT-specific tags while keeping WebVTT-compatible ones.
func cleanSRTTags(line string) string {
	cleaned := reFontTag.ReplaceAllString(line, "")
	cleaned = reASSAnTag.ReplaceAllString(cleaned, "")
	cleaned = reASSOverride.ReplaceAllString(cleaned, "")
	return cleaned
}

// ---------------------------------------------------------------------------
// ASS/SSA -> WebVTT
// ---------------------------------------------------------------------------

var (
	reEventsSection = regexp.MustCompile(`(?i)\[Events\]\s*\n([\s\S]*?)(?:\n\[|$)`)
	reASSTimestamp   = regexp.MustCompile(`(\d+):(\d{2}):(\d{2})\.(\d{2,3})`)
	reASSTagBlock    = regexp.MustCompile(`\{\\[^}]*\}`)
)
