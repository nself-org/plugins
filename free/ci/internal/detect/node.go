package detect

import (
	"encoding/json"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

type packageJSON struct {
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
	Dependencies   map[string]string `json:"dependencies"`
	Dev            map[string]string `json:"devDependencies"`
	Workspaces     json.RawMessage   `json:"workspaces"`
}

func detectNode(s Snapshot) []Fact {
	var out []Fact
	for _, p := range s.Paths("package.json") {
		b, err := s.read(p)
		var pkg packageJSON
		if err != nil || json.Unmarshal(b, &pkg) != nil {
			out = append(out, unknown("manifest", p, "invalid package.json"))
			continue
		}
		out = append(out, fact("stack", "node", p))
		manager := nodeManager(s, p, pkg.PackageManager)
		if manager != "" {
			out = append(out, fact("package_manager", manager, p))
		} else {
			out = append(out, unknown("package_manager", p, "ambiguous or unsupported package manager"))
		}
		for name, label := range map[string]string{"vitest": "vitest", "jest": "jest", "@playwright/test": "playwright"} {
			if _, ok := pkg.Dev[name]; ok {
				out = append(out, fact("test_framework", label, p))
				continue
			}
			if _, ok := pkg.Dependencies[name]; ok {
				out = append(out, fact("test_framework", label, p))
			}
		}
		for name, label := range map[string]string{"eslint": "eslint", "@biomejs/biome": "biome", "typescript": "tsc"} {
			if _, ok := pkg.Dev[name]; ok {
				out = append(out, fact("lint_tool", label, p))
				continue
			}
			if _, ok := pkg.Dependencies[name]; ok {
				out = append(out, fact("lint_tool", label, p))
			}
		}
		for _, script := range pkg.Scripts {
			if label := directNodeTool(script); label != "" {
				kind := "lint_tool"
				if label == "vitest" || label == "jest" || label == "playwright" {
					kind = "test_framework"
				}
				out = append(out, fact(kind, label, p))
			}
		}
		if len(pkg.Workspaces) > 0 {
			patterns, ok := workspacePatterns(pkg.Workspaces)
			if !ok {
				out = append(out, unknown("workspace", p, "invalid workspaces declaration"))
			} else {
				out = append(out, workspaceFacts(s, p, patterns)...)
			}
		}
		if path.Dir(p) != "." && !hasWorkspaceAncestor(s, p) {
			out = append(out, fact("workspace_member", path.Dir(p), p))
		}
	}
	for _, pair := range [][2]string{{"tsconfig.json", "tsc"}, {"biome.json", "biome"}, {"biome.jsonc", "biome"}, {"eslint.config.js", "eslint"}, {"eslint.config.mjs", "eslint"}, {".eslintrc.json", "eslint"}} {
		for _, p := range s.Paths(pair[0]) {
			out = append(out, fact("lint_tool", pair[1], p))
		}
	}
	for _, p := range s.Paths("pnpm-workspace.yaml") {
		b, err := s.read(p)
		var doc struct {
			Packages []string `yaml:"packages"`
		}
		if err != nil || yaml.Unmarshal(b, &doc) != nil {
			out = append(out, unknown("workspace", p, "invalid pnpm workspace"))
			continue
		}
		out = append(out, workspaceFacts(s, p, doc.Packages)...)
	}
	return out
}

func directNodeTool(script string) string {
	words := strings.Fields(script)
	if len(words) == 0 {
		return ""
	}
	if words[0] == "npx" || words[0] == "bunx" {
		words = words[1:]
	}
	if len(words) >= 2 && (words[0] == "pnpm" || words[0] == "yarn" || words[0] == "npm" || words[0] == "bun") && (words[1] == "exec" || words[1] == "run") {
		words = words[2:]
	}
	if len(words) == 0 {
		return ""
	}
	switch words[0] {
	case "vitest", "jest", "eslint", "tsc", "biome":
		return words[0]
	case "playwright":
		return "playwright"
	}
	return ""
}

func nodeManager(s Snapshot, p, declared string) string {
	if declared != "" {
		name := strings.SplitN(declared, "@", 2)[0]
		if name == "npm" || name == "pnpm" || name == "yarn" || name == "bun" {
			return name
		}
		return ""
	}
	dir := path.Dir(p)
	var found string
	for _, pair := range [][2]string{{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}, {"package-lock.json", "npm"}} {
		if s.has(path.Join(dir, pair[0])) {
			if found != "" && found != pair[1] {
				return ""
			}
			found = pair[1]
		}
	}
	if found != "" {
		return found
	}
	return ""
}

func hasWorkspaceAncestor(s Snapshot, p string) bool {
	for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
		parent := path.Dir(dir)
		if s.has(path.Join(parent, "pnpm-workspace.yaml")) {
			return true
		}
		if s.has(path.Join(parent, "package.json")) {
			b, err := s.read(path.Join(parent, "package.json"))
			if err == nil {
				var pkg packageJSON
				if json.Unmarshal(b, &pkg) == nil && len(pkg.Workspaces) > 0 {
					return true
				}
			}
		}
	}
	return false
}

func workspacePatterns(raw json.RawMessage) ([]string, bool) {
	var patterns []string
	if json.Unmarshal(raw, &patterns) == nil {
		return patterns, true
	}
	var object struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &object) == nil && object.Packages != nil {
		return object.Packages, true
	}
	return nil, false
}

func workspaceFacts(s Snapshot, manifest string, patterns []string) []Fact {
	var out []Fact
	root := path.Dir(manifest)
	for _, p := range s.Paths("package.json") {
		if p == path.Join(root, "package.json") {
			continue
		}
		rel := strings.TrimPrefix(path.Dir(p), root+"/")
		if root == "." {
			rel = path.Dir(p)
		}
		included := false
		for _, pattern := range patterns {
			negative := strings.HasPrefix(pattern, "!")
			pattern = strings.TrimPrefix(pattern, "!")
			if strings.HasPrefix(pattern, "../") || strings.HasPrefix(pattern, "/") {
				continue
			}
			if workspaceMatch(pattern, rel) {
				included = !negative
			}
		}
		if included {
			out = append(out, fact("workspace_member", rel, p))
		}
	}
	return out
}

func workspaceMatch(pattern, member string) bool {
	if pattern == member {
		return true
	}
	pp, mm := strings.Split(pattern, "/"), strings.Split(member, "/")
	if len(pp) != len(mm) {
		return false
	}
	for i := range pp {
		if ok, err := path.Match(pp[i], mm[i]); err != nil || !ok {
			return false
		}
	}
	return true
}
