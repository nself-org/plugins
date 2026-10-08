package detect

import (
	"strings"
)

func detectRust(s Snapshot) []Fact {
	var out []Fact
	for _, p := range s.Paths("Cargo.toml") {
		out = append(out, fact("stack", "rust", p), fact("test_framework", "cargo test", p))
		b, err := s.read(p)
		if err != nil || (!strings.Contains(string(b), "[package]") && !strings.Contains(string(b), "[workspace]")) {
			out = append(out, unknown("manifest", p, "invalid Cargo.toml"))
			continue
		}
		if strings.Contains(string(b), "nextest") {
			out = append(out, fact("test_framework", "cargo nextest", p))
		}
		if strings.Contains(string(b), "clippy") {
			out = append(out, fact("lint_tool", "clippy", p))
		}
	}
	for _, p := range s.Paths("nextest.toml") {
		if strings.Contains(p, ".config/") {
			out = append(out, fact("test_framework", "cargo nextest", p))
		}
	}
	return out
}
