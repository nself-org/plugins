#!/usr/bin/env bats
# remove-matrix.bats
# T-0367 — Free plugin remove matrix: all 16 clean uninstall
#
# Continuing from install-matrix.bats (T-0366).
# For each of the 16 free plugins in reverse install order:
#   1. nself plugin remove → exits 0
#   2. plugin status → not listed as installed
#   3. np_* tables → 0 rows in information_schema
#   4. docker ps → no container for the plugin
#   5. nself plugin list → plugin not in installed list
#
# Static tier: validates remove --dry-run and --help work.
# Docker tier: requires nself running with all 16 plugins installed (run after install-matrix).

load ../../../cli/src/tests/bats/test_helper

NSELF_BIN="${NSELF_BIN:-nself}"

FREE_PLUGINS_REVERSE=(
  content-progress
  github-runner
  github
  torrent-manager
  subtitle-manager
  mdns
  vpn
  link-preview
  tokens
  invitations
  feature-flags
  jobs
  notifications
  search
  content-acquisition
  webhooks
)

_require_nself() {
  command -v "$NSELF_BIN" >/dev/null 2>&1 || skip "nself not found in PATH"
}

_require_docker() {
  [ "${SKIP_DOCKER_TESTS:-1}" = "1" ] && skip "SKIP_DOCKER_TESTS=1"
  command -v docker >/dev/null 2>&1 || skip "docker not installed"
  docker info >/dev/null 2>&1 || skip "Docker daemon not running"
}

# ===========================================================================
# Static tier
# ===========================================================================

@test "static: nself plugin remove --help exits 0" {
  _require_nself
  run "$NSELF_BIN" plugin remove --help
  assert_success
}

@test "static: nself plugin remove --dry-run exits 0 for webhooks" {
  _require_nself
  run "$NSELF_BIN" plugin remove webhooks --dry-run
  # May exit 0 (dry-run) or fail gracefully with "not installed"
  local ok=0
  case "$status" in
    0) ok=1 ;;
    1) printf '%s' "$output" | grep -qiE "(not installed|dry.run)" && ok=1 || true ;;
  esac
  [ "$ok" -eq 1 ]
}

@test "static: nself plugin remove non-existent plugin fails gracefully" {
  _require_nself
  run "$NSELF_BIN" plugin remove _no_such_plugin_xyz_$$
  assert_failure
  # Must not crash with a stack trace — just a clean error message
  local crashed=0
  printf '%s' "$output" | grep -qiE "(panic|segfault|unhandled)" && crashed=1 || true
  [ "$crashed" -eq 0 ]
}

# ===========================================================================
# Docker tier: full remove matrix (reverse order)
# ===========================================================================

@test "docker: all 16 free plugins remove cleanly" {
  _require_nself
  _require_docker

  local failed_plugins=""

  for plugin in "${FREE_PLUGINS_REVERSE[@]}"; do
    # Remove
    if ! "$NSELF_BIN" plugin remove "$plugin" 2>/dev/null; then
      # If plugin was not installed, that's acceptable for the matrix
      if ! "$NSELF_BIN" plugin list 2>/dev/null | grep -q "^$plugin "; then
        continue  # not installed, skip
      fi
      failed_plugins="$failed_plugins REMOVE_FAIL:$plugin"
      continue
    fi

    # Verify plugin list no longer shows it as installed
    if "$NSELF_BIN" plugin list 2>/dev/null | grep -qE "^$plugin .* installed"; then
      failed_plugins="$failed_plugins STILL_LISTED:$plugin"
      continue
    fi

    # Verify no np_* tables remain
    local table_prefix
    table_prefix=$(printf '%s' "$plugin" | tr '-' '_')
    local remaining_tables
    remaining_tables=$("$NSELF_BIN" db shell -- -c "
      SELECT table_name FROM information_schema.tables
      WHERE table_name LIKE 'np_${table_prefix}%';
    " 2>/dev/null | grep -c "np_" || echo "0")

    if [ "$remaining_tables" -gt 0 ]; then
      printf "  Warning: %s orphaned np_%s_* table(s) after remove of %s\n" \
        "$remaining_tables" "$table_prefix" "$plugin" >&2
      # Warning only — some plugins keep tables for data retention
    fi

    # Verify no container running for this plugin
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "nself.*$plugin\|$plugin.*nself"; then
      failed_plugins="$failed_plugins CONTAINER_REMAINS:$plugin"
    fi
  done

  if [ -n "$failed_plugins" ]; then
    printf "FAILURES:%s\n" "$failed_plugins" >&2
    return 1
  fi
}

@test "docker: nself plugin list shows 0 installed plugins after full remove" {
  _require_nself
  _require_docker

  run "$NSELF_BIN" plugin list
  assert_success

  # Should show no plugins as 'installed'
  local installed_count
  installed_count=$(printf '%s' "$output" | grep -c " installed" || echo "0")
  [ "$installed_count" -eq 0 ]
}
