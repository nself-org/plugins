package legacy

import (
	"encoding/json"
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

// extractJSONObject decodes string pairs from a named top-level JSON object,
// preserving exact keys, including script names containing colons.
func extractJSONObject(data, key string) map[string]string {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &object); err != nil {
		return nil
	}
	var result map[string]string
	if err := json.Unmarshal(object[key], &result); err != nil {
		return nil
	}
	return result
}

// extractJSONStringArray extracts a top-level JSON string-array field (e.g.
// "workspaces": ["packages/*", "apps/*"]) using simple string
// parsing. Handles only a
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
//
//	.gitignore) or must fall back to a raw filesystem walk.
//
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
