#!/usr/bin/env bats
# notifications_test.bats
# CLI + HTTP tests for the notifications free plugin.
# Help-flag tests require no running services.
# HTTP tests require nself services running with notifications plugin installed.

NOTIFICATIONS_PORT=3102

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

skip_if_no_docker() {
  if ! docker_available; then
    skip "Docker not available"
  fi
}

plugin_running() {
  curl -fsS "http://localhost:${NOTIFICATIONS_PORT}/health" >/dev/null 2>&1
}

skip_if_not_running() {
  if ! plugin_running; then
    skip "notifications plugin not running on port $NOTIFICATIONS_PORT"
  fi
}

# ---------------------------------------------------------------------------
# Install / remove (dry-run)
# ---------------------------------------------------------------------------

@test "notifications: nself plugin install --dry-run succeeds" {
  run nself plugin install notifications --dry-run
  assert_success
}

@test "notifications: nself plugin remove --dry-run succeeds" {
  run nself plugin remove notifications --dry-run
  assert_success
}

# ---------------------------------------------------------------------------
# HTTP health check
# ---------------------------------------------------------------------------

@test "notifications: GET /health returns 200" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${NOTIFICATIONS_PORT}/health"
  assert_success
  assert_output --partial "ok\|healthy\|status"
}

@test "notifications: GET /health includes plugin name" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${NOTIFICATIONS_PORT}/health"
  assert_success
  assert_output --partial "notifications"
}

# ---------------------------------------------------------------------------
# Notification send endpoint
# ---------------------------------------------------------------------------

@test "notifications: POST /send requires content-type json" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS -X POST "http://localhost:${NOTIFICATIONS_PORT}/send" \
    -d "not-json"
  assert_failure
}

@test "notifications: POST /send with missing fields returns 400" {
  skip_if_no_docker
  skip_if_not_running
  run curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost:${NOTIFICATIONS_PORT}/send" \
    -H "Content-Type: application/json" \
    -d '{}'
  assert_output "400"
}

# ---------------------------------------------------------------------------
# Channel listing
# ---------------------------------------------------------------------------

@test "notifications: GET /channels returns list" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${NOTIFICATIONS_PORT}/channels"
  assert_success
}
