#!/bin/sh
# nself-ci.sh — nSelf CI gate runner wrapper
#
# Purpose: Build (if needed) and run the nself-ci gate binary from plugins/free/ci.
#   Posts a nself-ci GitHub commit status via gh OAuth on success or failure.
# Usage:
#   nself-ci.sh [--check] [--no-gitleaks] [--no-status] [repo-root]
#
# Requirements: go, gh CLI (with repo scope)
# SPORT: PLUGINS-CI-003

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_DIR="$(cd "$SCRIPT_DIR/../free/ci" && pwd)"
BINARY="$PLUGIN_DIR/nself-ci"

# Build the binary if it does not exist or source is newer.
needs_build=0
if [ ! -f "$BINARY" ]; then
  needs_build=1
elif [ "$PLUGIN_DIR/cmd/main.go" -nt "$BINARY" ] 2>/dev/null; then
  needs_build=1
elif [ "$PLUGIN_DIR/internal/gate.go" -nt "$BINARY" ] 2>/dev/null; then
  needs_build=1
elif [ "$PLUGIN_DIR/internal/status.go" -nt "$BINARY" ] 2>/dev/null; then
  needs_build=1
fi

if [ "$needs_build" = "1" ]; then
  printf "[nself-ci] building gate binary...\n"
  if ! (cd "$PLUGIN_DIR" && go build -o nself-ci ./cmd/); then
    printf "[nself-ci] build failed\n" >&2
    exit 1
  fi
  printf "[nself-ci] built %s\n" "$BINARY"
fi

exec "$BINARY" "$@"
