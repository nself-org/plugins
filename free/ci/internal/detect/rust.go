package detect

import (
	"strings"
)

func detectRust(s Snapshot) []Fact {
	var out []Fact
	for _, p := range s.Paths("Cargo.toml") {
		b, err := s.read(p)
		if err != nil {
			out = append(out, unknown("manifest", p, "unreadable Cargo.toml"))
			continue
		}
		if !validCargo(b) {
			out = append(out, unknown("manifest", p, "invalid Cargo.toml: missing [package] or [workspace]"))
			continue
		}
		out = append(out, fact("stack", "rust", p), fact("test_framework", "cargo test", p))
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

func validCargo(b []byte) bool {
	for _, line := range strings.Split(string(b), "\n") {
		switch strings.TrimSpace(line) {
		case "[package]", "[workspace]":
			return true
		}
	}
	return false
}
