package internal

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SubtitleFormat represents a detected subtitle format.
type SubtitleFormat string

const (
	FormatSRT     SubtitleFormat = "srt"
	FormatVTT     SubtitleFormat = "vtt"
	FormatASS     SubtitleFormat = "ass"
	FormatSSA     SubtitleFormat = "ssa"
	FormatUnknown SubtitleFormat = "unknown"
)

// Normalizer handles conversion of subtitle formats to WebVTT.
type Normalizer struct{}

// NewNormalizer creates a new Normalizer.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// NormalizeToWebVTT converts any supported subtitle format to valid WebVTT.
// Returns the path to the output file.
func (n *Normalizer) NormalizeToWebVTT(inputPath string, outputPath string) (string, error) {
	log.Printf("subtitle-manager: normalizing subtitle to WebVTT input=%s", inputPath)

	rawBytes, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("read input file: %w", err)
	}

	content := normalizeEncoding(rawBytes)
	format := detectFormat(content)

	log.Printf("subtitle-manager: detected format=%s", format)

	var vttContent string
	switch format {
	case FormatSRT:
		vttContent = srtToWebVTT(content)
	case FormatVTT:
		vttContent = cleanWebVTT(content)
	case FormatASS, FormatSSA:
		vttContent = assToWebVTT(content)
	default:
		return "", fmt.Errorf("unsupported subtitle format: %s. Supported: srt, vtt, ass, ssa", format)
	}

	if outputPath == "" {
		outputPath = replaceExtension(inputPath, ".vtt")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(outputPath, []byte(vttContent), 0644); err != nil {
		return "", fmt.Errorf("write output file: %w", err)
	}

	log.Printf("subtitle-manager: WebVTT normalization complete output=%s", outputPath)
	return outputPath, nil
}

// ---------------------------------------------------------------------------
// Format detection
// ---------------------------------------------------------------------------

var (
	reWebVTTHeader = regexp.MustCompile(`(?i)^WEBVTT`)
	reScriptInfo   = regexp.MustCompile(`(?i)\[Script Info\]`)
	reASSv4Plus    = regexp.MustCompile(`(?i)ScriptType:\s*v4\.00\+`)
	reSRTPattern1  = regexp.MustCompile(`(?m)^\d+\s*\n\d{1,2}:\d{2}:\d{2}[,.]\d{3}\s*-->\s*\d{1,2}:\d{2}:\d{2}[,.]\d{3}`)
	reSRTPattern2  = regexp.MustCompile(`\d{1,2}:\d{2}:\d{2}[,.]\d{3}\s*-->\s*\d{1,2}:\d{2}:\d{2}[,.]\d{3}`)
)

func detectFormat(content string) SubtitleFormat {
	trimmed := strings.TrimSpace(content)

	if reWebVTTHeader.MatchString(trimmed) {
		return FormatVTT
	}

	if reScriptInfo.MatchString(trimmed) {
		if reASSv4Plus.MatchString(trimmed) {
			return FormatASS
		}
		return FormatSSA
	}

	if reSRTPattern1.MatchString(trimmed) {
		return FormatSRT
	}
	if reSRTPattern2.MatchString(trimmed) {
		return FormatSRT
	}

	return FormatUnknown
}

// ---------------------------------------------------------------------------
// SRT -> WebVTT
// ---------------------------------------------------------------------------

var (
	reFontTag     = regexp.MustCompile(`(?i)</?font[^>]*>`)
	reASSAnTag    = regexp.MustCompile(`(?i)\{\\an?\d+\}`)
	reASSOverride = regexp.MustCompile(`\{\\[^}]+\}`)
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
func normalizeEncoding(buf []byte) string {
	var content string

	if len(buf) >= 3 && buf[0] == 0xEF && buf[1] == 0xBB && buf[2] == 0xBF {
		// UTF-8 BOM
		content = string(buf[3:])
	} else if len(buf) >= 2 && buf[0] == 0xFF && buf[1] == 0xFE {
		// UTF-16 LE BOM
		content = decodeUTF16LE(buf[2:])
	} else if len(buf) >= 2 && buf[0] == 0xFE && buf[1] == 0xFF {
		// UTF-16 BE BOM - swap bytes to LE then decode
		swapped := make([]byte, len(buf)-2)
		for i := 2; i < len(buf)-1; i += 2 {
			swapped[i-2] = buf[i+1]
			swapped[i-1] = buf[i]
		}
		content = decodeUTF16LE(swapped)
	} else {
		// Assume UTF-8
		content = string(buf)
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content
}

// decodeUTF16LE decodes a UTF-16 LE byte slice to a Go string.
func decodeUTF16LE(b []byte) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	runes := make([]rune, 0, len(b)/2)
	for i := 0; i < len(b)-1; i += 2 {
		r := rune(b[i]) | rune(b[i+1])<<8
		runes = append(runes, r)
	}
	return string(runes)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// cleanWebVTT normalizes an existing WebVTT file (encoding and whitespace).
func cleanWebVTT(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "WEBVTT") {
		lines = append([]string{"WEBVTT"}, lines...)
	}
	return strings.Join(lines, "\n")
}

func replaceExtension(filePath string, newExt string) string {
	ext := filepath.Ext(filePath)
	return filePath[:len(filePath)-len(ext)] + newExt
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
