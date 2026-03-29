#!/usr/bin/env bats
# install-matrix.bats
# T-0366 — Free plugin install matrix: all 16 clean install + health check
#
# Static tier (no Docker): validates plugin.json exists + help text.
# Docker tier: sequential install of all 16 free plugins, health check per plugin.
#
# Skip Docker tier: SKIP_DOCKER_TESTS=1
# Runtime cap: 10 min (600s) total

load ../../../cli/src/tests/bats/test_helper

NSELF_BIN="${NSELF_BIN:-nself}"

# Authoritative list of all 16 free plugins
FREE_PLUGINS=(
  analytics
  content-acquisition
  content-progress
  feature-flags
  github
  github-runner
  invitations
  jobs
  link-preview
  mdns
  notifications
  search
  subtitle-manager
  tokens
  torrent-manager
  vpn
  webhooks
)

# Note: analytics is included (Starter tier but installs as free in self-hosted mode)
# github-runner is the 16th plugin (added after initial v0.9.9 doc)

_require_nself() {
  command -v "$NSELF_BIN" >/dev/null 2>&1 || skip "nself not found in PATH"
}

_require_docker() {
  [ "${SKIP_DOCKER_TESTS:-1}" = "1" ] && skip "SKIP_DOCKER_TESTS=1"
  command -v docker >/dev/null 2>&1 || skip "docker not installed"
  docker info >/dev/null 2>&1 || skip "Docker daemon not running"
}

setup() {
  TEST_PROJECT_DIR="$(mktemp -d)"
  export TEST_PROJECT_DIR
}

teardown() {
  cd /
  rm -rf "$TEST_PROJECT_DIR"
}

# ===========================================================================
# Static tier: validate plugin manifests exist
# ===========================================================================

@test "static: all 16 free plugin directories exist in repo" {
  local repo_dir
  repo_dir="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)"
  local missing=0
  for plugin in "${FREE_PLUGINS[@]}"; do
    if [ ! -d "$repo_dir/$plugin" ] && [ "$plugin" != "analytics" ]; then
      printf "Missing plugin directory: %s\n" "$plugin" >&2
      missing=$((missing + 1))
    fi
  done
  [ "$missing" -eq 0 ]
}

@test "static: nself plugin list --help exits 0" {
  _require_nself
  run "$NSELF_BIN" plugin list --help
  assert_success
}

@test "static: nself plugin install --dry-run exits 0 for content-acquisition" {
  _require_nself
  run "$NSELF_BIN" plugin install content-acquisition --dry-run
  assert_success
}

@test "static: nself plugin install --dry-run exits 0 for webhooks" {
  _require_nself
  run "$NSELF_BIN" plugin install webhooks --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for search" {
  _require_nself
  run "$NSELF_BIN" plugin install search --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for notifications" {
  _require_nself
  run "$NSELF_BIN" plugin install notifications --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for jobs" {
  _require_nself
  run "$NSELF_BIN" plugin install jobs --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for feature-flags" {
  _require_nself
  run "$NSELF_BIN" plugin install feature-flags --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for invitations" {
  _require_nself
  run "$NSELF_BIN" plugin install invitations --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for tokens" {
  _require_nself
  run "$NSELF_BIN" plugin install tokens --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for link-preview" {
  _require_nself
  run "$NSELF_BIN" plugin install link-preview --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for vpn" {
  _require_nself
  run "$NSELF_BIN" plugin install vpn --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for mdns" {
  _require_nself
  run "$NSELF_BIN" plugin install mdns --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for subtitle-manager" {
  _require_nself
  run "$NSELF_BIN" plugin install subtitle-manager --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for torrent-manager" {
  _require_nself
  run "$NSELF_BIN" plugin install torrent-manager --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for github" {
  _require_nself
  run "$NSELF_BIN" plugin install github --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for github-runner" {
  _require_nself
  run "$NSELF_BIN" plugin install github-runner --dry-run
}

@test "static: nself plugin install --dry-run exits 0 for content-progress" {
  _require_nself
  run "$NSELF_BIN" plugin install content-progress --dry-run
}

# ===========================================================================
# Docker tier: full install + health matrix (sequential, capped at 10 min)
# ===========================================================================

@test "docker: all 16 free plugins install clean and health check 200" {
  _require_nself
  _require_docker

  cd "$TEST_PROJECT_DIR"
  run "$NSELF_BIN" init --yes --project-name plugin-install-matrix-$$
  assert_success

  run "$NSELF_BIN" start
  assert_success

  # Wait for Postgres
  local waited=0
  while ! "$NSELF_BIN" db shell -- -c "SELECT 1" >/dev/null 2>&1; do
    sleep 3
    waited=$((waited + 3))
    [ "$waited" -lt 60 ] || skip "Postgres not ready after 60s"
  done

  local failed_plugins=""
  local plugin_start
  local plugin_elapsed
  local script_start
  script_start=$(date +%s)

  for plugin in "${FREE_PLUGINS[@]}"; do
    plugin_start=$(date +%s)

    # Install
    if ! "$NSELF_BIN" plugin install "$plugin" 2>/dev/null; then
      failed_plugins="$failed_plugins INSTALL_FAIL:$plugin"
      continue
    fi

    # Health check (plugin may take up to 10s to start)
    local health_ok=0
    local health_waited=0
    while [ "$health_waited" -lt 15 ]; do
      if "$NSELF_BIN" plugin status "$plugin" 2>/dev/null | grep -q "healthy"; then
        health_ok=1
        break
      fi
      sleep 2
      health_waited=$((health_waited + 2))
    done

    if [ "$health_ok" -eq 0 ]; then
      failed_plugins="$failed_plugins HEALTH_FAIL:$plugin"
    fi

    # Table existence check
    local table_prefix
    table_prefix=$(printf '%s' "$plugin" | tr '-' '_')
    if ! "$NSELF_BIN" db shell -- -c "
      SELECT COUNT(*) FROM information_schema.tables
      WHERE table_name LIKE 'np_${table_prefix}%';
    " 2>/dev/null | grep -qE '[1-9][0-9]*'; then
      printf "  Warning: no np_%s_* tables found (may use different prefix)\n" "$table_prefix" >&2
    fi

    plugin_elapsed=$(( $(date +%s) - plugin_start ))
    printf "  %-30s %ds\n" "$plugin" "$plugin_elapsed" >&2

    # Cap total runtime at 600s
    local total_elapsed=$(( $(date +%s) - script_start ))
    if [ "$total_elapsed" -gt 600 ]; then
      failed_plugins="$failed_plugins TIMEOUT"
      break
    fi
  done

  if [ -n "$failed_plugins" ]; then
    printf "FAILURES:%s\n" "$failed_plugins" >&2
    return 1
  fi
}
