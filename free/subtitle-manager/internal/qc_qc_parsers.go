package internal

import (
	"regexp"
	"strconv"
	"strings"
)

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

