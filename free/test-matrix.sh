#!/bin/bash
# test-matrix.sh
# Install each free plugin in sequence, check health, uninstall.
# Requires nself services running with Docker.
#
# Usage:
#   bash plugins/free/test-matrix.sh
#   bash plugins/free/test-matrix.sh --plugin search       # test single plugin
#   bash plugins/free/test-matrix.sh --skip-uninstall      # leave plugins installed

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FREE_PLUGINS_DIR="$SCRIPT_DIR"

# All free plugins (discovered from filesystem)
ALL_PLUGINS=""
for d in "$FREE_PLUGINS_DIR"/*/; do
  plugin=$(basename "$d")
  ALL_PLUGINS="$ALL_PLUGINS $plugin"
done

SINGLE_PLUGIN=""
SKIP_UNINSTALL=false
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --plugin)
      SINGLE_PLUGIN="$2"
      shift 2
      ;;
    --skip-uninstall)
      SKIP_UNINSTALL=true
      shift
      ;;
    --help|-h)
      printf "Usage: %s [--plugin NAME] [--skip-uninstall]\n" "$0"
      exit 0
      ;;
    *)
      printf "Unknown argument: %s\n" "$1" >&2
      exit 1
      ;;
  esac
done

if [ -n "$SINGLE_PLUGIN" ]; then
  ALL_PLUGINS="$SINGLE_PLUGIN"
fi

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

pass() {
  printf "\033[32mPASS\033[0m: %s\n" "$1"
  PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
  printf "\033[31mFAIL\033[0m: %s\n" "$1" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

skip_test() {
  printf "\033[33mSKIP\033[0m: %s\n" "$1"
  SKIP_COUNT=$((SKIP_COUNT + 1))
}

nself_running() {
  nself status --quiet 2>/dev/null
}

http_health() {
  local url="$1"
  local max_wait="${2:-10}"
  local waited=0
  while [ $waited -lt $max_wait ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    waited=$((waited + 2))
    # POSIX-compatible sleep
    sleep 2
  done
  return 1
}

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

if ! command -v nself >/dev/null 2>&1; then
  printf "ERROR: nself not found in PATH\n" >&2
  exit 1
fi

if ! nself_running; then
  printf "ERROR: nself services not running. Run 'nself start' first.\n" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  printf "ERROR: docker not found in PATH\n" >&2
  exit 1
fi

printf "nSelf free plugin test matrix\n"
printf "Plugins to test:%s\n\n" "$ALL_PLUGINS"

# ---------------------------------------------------------------------------
# Main loop
# ---------------------------------------------------------------------------

for plugin in $ALL_PLUGINS; do
  printf "--- Testing plugin: %s ---\n" "$plugin"

  # 1. Get port from plugin.json (if it exists)
  PLUGIN_JSON="$FREE_PLUGINS_DIR/$plugin/plugin.json"
  PLUGIN_PORT=""
  if [ -f "$PLUGIN_JSON" ] && command -v python3 >/dev/null 2>&1; then
    PLUGIN_PORT=$(python3 -c "
import json, sys
try:
    d = json.load(open('$PLUGIN_JSON'))
    print(d.get('port', '') or d.get('config', {}).get('defaultPort', ''))
except Exception:
    pass
" 2>/dev/null)
  fi

  # 2. Install
  if nself plugin install "$plugin" 2>&1; then
    pass "$plugin: install"
  else
    fail "$plugin: install failed"
    continue
  fi

  # 3. Health check (if port known)
  if [ -n "$PLUGIN_PORT" ]; then
    if http_health "http://localhost:${PLUGIN_PORT}/health" 15; then
      pass "$plugin: health check (port $PLUGIN_PORT)"
    else
      fail "$plugin: health check timed out on port $PLUGIN_PORT"
    fi
  else
    skip_test "$plugin: health check (no port in plugin.json)"
  fi

  # 4. Uninstall
  if [ "$SKIP_UNINSTALL" = "false" ]; then
    if nself plugin remove "$plugin" 2>&1; then
      pass "$plugin: uninstall"
    else
      fail "$plugin: uninstall failed"
    fi
  else
    skip_test "$plugin: uninstall (--skip-uninstall set)"
  fi

  printf "\n"
done

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

TOTAL=$((PASS_COUNT + FAIL_COUNT + SKIP_COUNT))
printf "Results: %d/%d passed, %d failed, %d skipped\n" \
  "$PASS_COUNT" "$TOTAL" "$FAIL_COUNT" "$SKIP_COUNT"

if [ "$FAIL_COUNT" -gt 0 ]; then
  exit 1
fi
exit 0
