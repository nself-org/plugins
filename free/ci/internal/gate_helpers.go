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

// workspaceMembers returns the directories of a pnpm/npm workspace's member
// packages, or nil when root is not a workspace.
//
// Purpose:     Let the Node gates see scripts that live in member packages
//              rather than the root package.json.
// Inputs:      root string — repo root
// Outputs:     []string — absolute member directories containing a package.json
// Constraints: Handles pnpm-workspace.yaml globs and package.json "workspaces".
//              Skips node_modules. Glob depth is whatever the pattern states.
func workspaceMembers(root string) []string {
	var patterns []string

	if data, err := os.ReadFile(filepath.Join(root, "pnpm-workspace.yaml")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- ") {
				continue
			}
			pat := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `'"`)
			// Members outside the repo (sibling checkouts) are not ours to gate.
			if pat != "" && !strings.HasPrefix(pat, "..") {
				patterns = append(patterns, pat)
			}
		}
	}

	if len(patterns) == 0 {
		if ws, ok := loadPackageJSON(root)["workspaces"].([]interface{}); ok {
			for _, w := range ws {
				if str, ok := w.(string); ok {
					patterns = append(patterns, str)
				}
			}
		}
	}

	seen := map[string]bool{}
	var members []string
	for _, pat := range patterns {
		matches, err := filepath.Glob(filepath.Join(root, pat))
		if err != nil {
			continue
		}
		for _, m := range matches {
			if strings.Contains(m, "node_modules") || seen[m] {
				continue
			}
			if fileExists(filepath.Join(m, "package.json")) {
				seen[m] = true
				members = append(members, m)
			}
		}
	}
	return members
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
