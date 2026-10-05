package main

// Purpose: locate (or write) the dev-watch.sh script `dev` runs. Port of
// findDevWatchScript, sdkRepoPath and writeInlineDevWatchScript in core
// cmd/commands/plugin_dev.go.
// Inputs:  the SDK checkout next to the binary, GOPATH, the executable path.
// Outputs: a script path; writes /tmp/nself-dev-watch.sh (os.TempDir) when no
// SDK script is found.
// Constraints: the inline script text is core's, byte for byte.

import (
	"fmt"
	"os"
	"path/filepath"
)

// findDevWatchScript locates dev-watch.sh in the SDK devkit or falls back to
// the inline copy.
func findDevWatchScript() (string, error) {
	candidates := []string{
		filepath.Join(sdkRepoPath(), "devkit", "tools", "dev-watch.sh"),
		filepath.Join(os.Getenv("GOPATH"), "pkg", "mod", "github.com", "nself-org", "plugin-sdk-go*", "devkit", "tools", "dev-watch.sh"),
	}
	for _, c := range candidates {
		matches, err := filepath.Glob(c)
		if err != nil || len(matches) == 0 {
			continue
		}
		if _, err := os.Stat(matches[0]); err == nil {
			return matches[0], nil
		}
	}
	return writeInlineDevWatchScript()
}

// sdkRepoPath returns the expected sibling repo path for monorepo setups.
func sdkRepoPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	return filepath.Join(projectRoot, "plugins-pro", "plugin-sdk-go")
}

// inlineDevWatch is the minimal dev-watch.sh used when the SDK devkit is absent.
const inlineDevWatch = `#!/usr/bin/env bash
set -euo pipefail
ENTRY="${1:-./cmd}"
if command -v air >/dev/null 2>&1; then
  if [ ! -f ./.air.toml ]; then
    cat >./.air.toml <<'TOML'
root = "."
tmp_dir = "tmp"
[build]
  cmd = "go build -o ./tmp/plugin ./cmd"
  bin = "tmp/plugin"
  delay = 500
  include_ext = ["go", "yaml", "yml"]
  exclude_dir = ["tmp", "vendor", ".git"]
[log]
  time = true
TOML
  fi
  exec air
fi
if command -v fswatch >/dev/null 2>&1; then
  printf 'dev-watch: air not installed; falling back to fswatch\n' >&2
  while true; do
    (go run "$ENTRY" &); pid=$!
    fswatch -1 -e ".*" -i "\\.go$" -i "\\.ya?ml$" . >/dev/null
    kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; sleep 0.5
  done
fi
printf 'dev-watch: neither air nor fswatch installed.\nInstall: go install github.com/air-verse/air@latest\n' >&2
exit 1
`

// writeInlineDevWatchScript writes the inline script to a temp file.
func writeInlineDevWatchScript() (string, error) {
	tmpFile := filepath.Join(os.TempDir(), "nself-dev-watch.sh")
	if err := os.WriteFile(tmpFile, []byte(inlineDevWatch), 0750); err != nil {
		return "", fmt.Errorf("writing inline dev-watch script: %w", err)
	}
	return tmpFile, nil
}
