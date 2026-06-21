package internal

import (
	"fmt"
	"log"
	"strings"
)

func assToWebVTT(content string) string {
	var lines []string
	lines = append(lines, "WEBVTT", "")

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	eventsMatch := reEventsSection.FindStringSubmatch(normalized)
	if eventsMatch == nil {
		log.Println("subtitle-manager: no [Events] section found in ASS/SSA file")
		return strings.Join(lines, "\n")
	}

	eventsBlock := eventsMatch[1]
	eventLines := strings.Split(eventsBlock, "\n")

	// Parse Format line
	formatColumns := []string{"layer", "start", "end", "style", "name", "marginl", "marginr", "marginv", "effect", "text"}
	for _, line := range eventLines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "format:") {
			colStr := strings.TrimPrefix(trimmed, trimmed[:strings.Index(trimmed, ":")+1])
			colStr = strings.TrimSpace(colStr)
			cols := strings.Split(colStr, ",")
			formatColumns = make([]string, len(cols))
			for i, c := range cols {
				formatColumns[i] = strings.TrimSpace(strings.ToLower(c))
			}
			break
		}
	}

	startIdx := indexOf(formatColumns, "start")
	endIdx := indexOf(formatColumns, "end")
	textIdx := indexOf(formatColumns, "text")

	if startIdx < 0 || endIdx < 0 || textIdx < 0 {
		log.Println("subtitle-manager: could not find required columns in ASS/SSA Format line")
		return strings.Join(lines, "\n")
	}

	for _, line := range eventLines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "dialogue:") {
			continue
		}

		parts := trimmed[strings.Index(trimmed, ":")+1:]
		parts = strings.TrimSpace(parts)
		columns := splitASSDialogue(parts, len(formatColumns))
		if len(columns) < len(formatColumns) {
			continue
		}

		startTime := columns[startIdx]
		endTime := columns[endIdx]
		rawText := columns[textIdx]

		vttStart := assTimestampToVTT(startTime)
		vttEnd := assTimestampToVTT(endTime)
		if vttStart == "" || vttEnd == "" {
			continue
		}

		cleanText := stripASSTags(rawText)
		if strings.TrimSpace(cleanText) == "" {
			continue
		}

		lines = append(lines, fmt.Sprintf("%s --> %s", vttStart, vttEnd))
		lines = append(lines, cleanText)
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// splitASSDialogue splits an ASS dialogue line, keeping the last field (Text) intact
// even if it contains commas.
func splitASSDialogue(line string, columnCount int) []string {
	var result []string
	remaining := line

	for i := 0; i < columnCount-1; i++ {
		commaIdx := strings.Index(remaining, ",")
		if commaIdx < 0 {
			break
		}
		result = append(result, strings.TrimSpace(remaining[:commaIdx]))
		remaining = remaining[commaIdx+1:]
	}
	// The rest is the Text field
	result = append(result, strings.TrimSpace(remaining))
	return result
}

// assTimestampToVTT converts ASS timestamp (H:MM:SS.cc) to WebVTT (HH:MM:SS.mmm).
func assTimestampToVTT(timestamp string) string {
	m := reASSTimestamp.FindStringSubmatch(strings.TrimSpace(timestamp))
	if m == nil {
		return ""
	}

	hours := m[1]
	if len(hours) < 2 {
		hours = "0" + hours
	}
	minutes := m[2]
	seconds := m[3]
	ms := m[4]
	// ASS uses centiseconds (2 digits), VTT uses milliseconds (3 digits)
	if len(ms) == 2 {
		ms = ms + "0"
	}

	return fmt.Sprintf("%s:%s:%s.%s", hours, minutes, seconds, ms)
}

// stripASSTags removes all ASS override tags from text.
func stripASSTags(text string) string {
	cleaned := reASSTagBlock.ReplaceAllString(text, "")
	// Convert \N to newline (ASS line break)
	cleaned = strings.ReplaceAll(cleaned, "\\N", "\n")
	// Convert \n (soft line break) to space
	cleaned = strings.ReplaceAll(cleaned, "\\n", " ")
	// Remove \h (hard space)
	cleaned = strings.ReplaceAll(cleaned, "\\h", " ")
	return strings.TrimSpace(cleaned)
}

// ---------------------------------------------------------------------------
// Encoding normalization
// ---------------------------------------------------------------------------

// normalizeEncoding detects encoding via BOM, strips BOM, and normalizes line endings.
