#!/usr/bin/env bats
# batch_plugin_tests.bats
# Batch tests for utility free plugins:
#   link-preview, tokens, search, feature-flags, github, mdns
# All tests use --dry-run for install/remove.
# HTTP tests require nself running with each plugin installed.

SEARCH_PORT=3110
TOKENS_PORT=3107
LINK_PREVIEW_PORT=3718

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

skip_if_no_docker() {
  if ! docker_available; then
    skip "Docker not available"
  fi
}

plugin_port_open() {
  local port="$1"
  curl -fsS "http://localhost:${port}/health" >/dev/null 2>&1
}

# ===========================================================================
# link-preview
# ===========================================================================

@test "link-preview: nself plugin install --dry-run succeeds" {
  run nself plugin install link-preview --dry-run
  assert_success
}

@test "link-preview: nself plugin remove --dry-run succeeds" {
  run nself plugin remove link-preview --dry-run
  assert_success
}

@test "link-preview: GET /health returns 200" {
  skip_if_no_docker
  plugin_port_open "$LINK_PREVIEW_PORT" || skip "link-preview not running"
  run curl -fsS "http://localhost:${LINK_PREVIEW_PORT}/health"
  assert_success
  assert_output --partial "ok\|healthy"
}

@test "link-preview: POST /preview with missing url returns error" {
  skip_if_no_docker
  plugin_port_open "$LINK_PREVIEW_PORT" || skip "link-preview not running"
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost:${LINK_PREVIEW_PORT}/preview" \
    -H "Content-Type: application/json" \
    -d '{}')
  [ "$STATUS" = "400" ] || [ "$STATUS" = "422" ]
}

# ===========================================================================
# tokens
# ===========================================================================

@test "tokens: nself plugin install --dry-run succeeds" {
  run nself plugin install tokens --dry-run
  assert_success
}

@test "tokens: nself plugin remove --dry-run succeeds" {
  run nself plugin remove tokens --dry-run
  assert_success
}

@test "tokens: GET /health returns 200" {
  skip_if_no_docker
  plugin_port_open "$TOKENS_PORT" || skip "tokens not running"
  run curl -fsS "http://localhost:${TOKENS_PORT}/health"
  assert_success
  assert_output --partial "ok\|healthy"
}

@test "tokens: POST /generate with missing fields returns 400" {
  skip_if_no_docker
  plugin_port_open "$TOKENS_PORT" || skip "tokens not running"
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost:${TOKENS_PORT}/generate" \
    -H "Content-Type: application/json" \
    -d '{}')
  [ "$STATUS" = "400" ] || [ "$STATUS" = "422" ]
}

# ===========================================================================
# search
# ===========================================================================

@test "search: nself plugin install --dry-run succeeds" {
  run nself plugin install search --dry-run
  assert_success
}

@test "search: nself plugin remove --dry-run succeeds" {
  run nself plugin remove search --dry-run
  assert_success
}

@test "search: GET /health returns 200" {
  skip_if_no_docker
  plugin_port_open "$SEARCH_PORT" || skip "search not running"
  run curl -fsS "http://localhost:${SEARCH_PORT}/health"
  assert_success
  assert_output --partial "ok\|healthy"
}

@test "search: GET /indexes returns list" {
  skip_if_no_docker
  plugin_port_open "$SEARCH_PORT" || skip "search not running"
  run curl -fsS "http://localhost:${SEARCH_PORT}/indexes"
  assert_success
}

@test "search: POST /search with missing query returns error" {
  skip_if_no_docker
  plugin_port_open "$SEARCH_PORT" || skip "search not running"
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost:${SEARCH_PORT}/search" \
    -H "Content-Type: application/json" \
    -d '{}')
  [ "$STATUS" = "400" ] || [ "$STATUS" = "422" ]
}

# ===========================================================================
# feature-flags
# ===========================================================================

@test "feature-flags: nself plugin install --dry-run succeeds" {
  run nself plugin install feature-flags --dry-run
  assert_success
}

@test "feature-flags: nself plugin remove --dry-run succeeds" {
  run nself plugin remove feature-flags --dry-run
  assert_success
}

# ===========================================================================
# github
# ===========================================================================

@test "github: nself plugin install --dry-run succeeds" {
  run nself plugin install github --dry-run
  assert_success
}

@test "github: nself plugin remove --dry-run succeeds" {
  run nself plugin remove github --dry-run
  assert_success
}

# ===========================================================================
# mdns
# ===========================================================================

@test "mdns: nself plugin install --dry-run succeeds" {
  run nself plugin install mdns --dry-run
  assert_success
}

@test "mdns: nself plugin remove --dry-run succeeds" {
  run nself plugin remove mdns --dry-run
  assert_success
}
