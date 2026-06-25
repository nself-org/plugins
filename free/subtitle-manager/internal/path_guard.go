package internal

// path_guard.go — containment guard for user-supplied filesystem paths.
//
// SECURITY (path traversal): /v1/sync, /v1/qc, /v1/normalize and /v1/fetch-best
// accept VideoPath / SubtitlePath / InputPath from the request body and hand
// them to exec.CommandContext (alass/ffsubsync) and os.ReadFile/os.WriteFile.
// Without containment a caller could pass "../../etc/passwd" or "/etc/shadow"
// and make the plugin read or overwrite arbitrary files on the host. Every
// such path must resolve to a location INSIDE the configured media root.
//
// Inputs:    media root (cfg.MediaRoot) + a user-supplied path.
// Outputs:   the cleaned, contained path, or an error when it escapes the root.
// Constraints: rejects "..", and any path that resolves outside the root.

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateMediaPath cleans p and verifies it stays within root. It returns the
// cleaned absolute-ish path on success. Relative inputs are resolved against
// root; absolute inputs must already live under root. Any path that escapes the
// root (via "..", symlink-style prefixes, or an unrelated absolute path) is
// rejected.
func validateMediaPath(root, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("path is empty")
	}

	cleanRoot := filepath.Clean(root)

	var candidate string
	if filepath.IsAbs(p) {
		candidate = filepath.Clean(p)
	} else {
		candidate = filepath.Clean(filepath.Join(cleanRoot, p))
	}

	// rel must not begin with ".." (escape) and must not be the literal "..".
	rel, err := filepath.Rel(cleanRoot, candidate)
	if err != nil {
		return "", fmt.Errorf("path %q is not within the media root", p)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the media root (traversal blocked)", p)
	}

	return candidate, nil
}
