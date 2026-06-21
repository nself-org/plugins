package internal

import (
	"os"
	"path/filepath"
	"strings"
)

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadPackageJSON reads the scripts section of package.json as a raw map.
func loadPackageJSON(root string) map[string]interface{} {
	path := filepath.Join(root, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	// Minimal JSON parse for scripts section — avoid pulling in dependencies.
	scripts := extractJSONObject(string(data), "scripts")
	result := make(map[string]interface{})
	for k, v := range scripts {
		result[k] = v
	}
	return result
}

// hasScript returns true if the package.json scripts map contains the key.
func hasScript(pkg map[string]interface{}, key string) bool {
	if pkg == nil {
		return false
	}
	_, ok := pkg[key]
	return ok
}

// extractJSONObject extracts key→value string pairs from a named JSON object
// using simple string parsing (no external JSON library to keep zero deps).
func extractJSONObject(json, key string) map[string]string {
	result := make(map[string]string)
	// Find "key":
	search := `"` + key + `"`
	idx := strings.Index(json, search)
	if idx < 0 {
		return result
	}
	// Find the opening brace after the key.
	start := strings.Index(json[idx:], "{")
	if start < 0 {
		return result
	}
	start += idx + 1

	// Walk until matching closing brace.
	depth := 1
	end := start
	for end < len(json) && depth > 0 {
		switch json[end] {
		case '{':
			depth++
		case '}':
			depth--
		}
		end++
	}
	block := json[start : end-1]

	// Extract "name": "value" pairs.
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `"`) {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.Trim(strings.TrimSpace(parts[0]), `"`)
		v := strings.Trim(strings.TrimSpace(strings.TrimRight(parts[1], ",")), `"`)
		if k != "" {
			result[k] = v
		}
	}
	return result
}
