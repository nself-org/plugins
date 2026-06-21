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
