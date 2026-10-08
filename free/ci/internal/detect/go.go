package detect

import "strings"

func detectGo(s Snapshot) []Fact {
	var out []Fact
	for _, p := range s.Paths("go.mod") {
		out = append(out, fact("stack", "go", p), fact("test_framework", "go test", p))
		b, err := s.read(p)
		if err != nil || !strings.Contains(string(b), "module ") {
			out = append(out, unknown("manifest", p, "invalid go.mod"))
		}
	}
	for _, p := range s.Paths(".golangci.yml") {
		out = append(out, fact("lint_tool", "golangci-lint", p))
	}
	for _, p := range s.Paths(".golangci.yaml") {
		out = append(out, fact("lint_tool", "golangci-lint", p))
	}
	for _, p := range s.Paths(".golangci.toml") {
		out = append(out, fact("lint_tool", "golangci-lint", p))
	}
	return out
}
