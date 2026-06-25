package internal

import (
	"path/filepath"
	"strings"
)

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
