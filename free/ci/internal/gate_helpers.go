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

// extractJSONStringArray extracts a top-level JSON string-array field (e.g.
// "workspaces": ["packages/*", "apps/*"]) using the same simple string
// parsing as extractJSONObject — no external JSON library. Handles only a
// flat array of string literals, which is the shape package.json
// "workspaces" always takes in practice.
func extractJSONStringArray(json, key string) []string {
	search := `"` + key + `"`
	idx := strings.Index(json, search)
	if idx < 0 {
		return nil
	}
	start := strings.Index(json[idx:], "[")
	if start < 0 {
		return nil
	}
	start += idx + 1
	end := strings.Index(json[start:], "]")
	if end < 0 {
		return nil
	}
	block := json[start : start+end]

	var items []string
	for _, part := range strings.Split(block, ",") {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

// isGitRepo reports whether root is inside a git checkout.
//
// Purpose:     Decide whether gitleaks can scan tracked content (respecting
//              .gitignore) or must fall back to a raw filesystem walk.
// Inputs:      root string — directory to test
// Outputs:     bool
// Constraints: Walks upward, so a subdirectory of a checkout still counts.
func isGitRepo(root string) bool {
	dir := root
	for {
		if fileExists(filepath.Join(dir, ".git")) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// anyMemberHasScript reports whether at least one workspace member defines the
// named script, so a gate is only added when it will actually do something.
func anyMemberHasScript(members []string, script string) bool {
	for _, m := range members {
		if hasScript(loadPackageJSON(m), script) {
			return true
		}
	}
	return false
}
