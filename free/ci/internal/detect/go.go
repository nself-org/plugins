package detect

import "strings"

func detectGo(s Snapshot) []Fact {
	var out []Fact
	for _, p := range s.Paths("go.mod") {
		b, err := s.read(p)
		if err != nil {
			out = append(out, unknown("manifest", p, "unreadable go.mod"))
			continue
		}
		if !validGoMod(b) {
			out = append(out, unknown("manifest", p, "invalid go.mod: missing module line"))
			continue
		}
		out = append(out, fact("stack", "go", p), fact("test_framework", "go test", p))
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

func validGoMod(b []byte) bool {
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[0] == "module" && !strings.HasPrefix(fields[1], "//") {
			return true
		}
	}
	return false
}
