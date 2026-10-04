#!/usr/bin/env bash
# go-licenses-check-retry.sh — run `go-licenses check` for one module, retrying
#   only when the failure is a transient module-proxy/network error.
#
# Purpose: go-licenses must download every module in the build list from
#   proxy.golang.org. That fails intermittently. On 2026-10-04 the push run for
#   272961a6 went red on free/byok/go with:
#
#     golang.org/x/sys@v0.46.0: read "https://proxy.golang.org/.../v0.46.0.zip":
#       stream error: stream ID 119; INTERNAL_ERROR; received from peer
#
#   No license was involved; the same tree had passed minutes earlier on the PR.
#
# Inputs:  $1 = module directory. $2 = allowed licenses (comma list).
#          $3.. = extra go-licenses flags (e.g. --ignore ...).
#          ATTEMPTS env var (default 3).
# Outputs: go-licenses output on stdout/stderr; exit 0 if the check passes.
# Constraints: only output matching a network-error signature is retried. A real
#   license violation, an unknown license or a checksum mismatch fails on
#   attempt 1. Checksum verification stays ON (no GONOSUMDB / GOFLAGS=-mod=mod).
set -uo pipefail

DIR="${1:?usage: go-licenses-check-retry.sh <dir> <allowed> [flags...]}"
ALLOWED="${2:?allowed licenses required}"
shift 2
ATTEMPTS="${ATTEMPTS:-3}"
LOG="$(mktemp)"
trap 'rm -f "$LOG"' EXIT

# Signatures of a transient fetch failure (not a verdict about a license).
NET_RE='stream error|INTERNAL_ERROR|connection reset|connection refused|i/o timeout|TLS handshake timeout|unexpected EOF|502 Bad Gateway|503 Service Unavailable|504 Gateway Timeout|no such host|dial tcp'

i=1
while :; do
  if (cd "$DIR" && go-licenses check ./... --allowed_licenses="$ALLOWED" "$@") >"$LOG" 2>&1; then
    cat "$LOG"
    [ "$i" -gt 1 ] && echo "::notice::$DIR passed on attempt $i"
    exit 0
  fi
  cat "$LOG"
  if [ "$i" -lt "$ATTEMPTS" ] && grep -Eq "$NET_RE" "$LOG"; then
    delay=$(( i * 15 ))
    echo "::warning::$DIR: transient module-proxy error (attempt $i/$ATTEMPTS); retrying in ${delay}s"
    sleep "$delay"
    i=$(( i + 1 ))
    continue
  fi
  exit 1
done
