#!/usr/bin/env bats
# webhooks_test.bats
# CLI + HTTP tests for the webhooks free plugin.
# Complements the existing TypeScript tests in ts/tests/webhooks.test.ts.

WEBHOOKS_PORT=3403

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

skip_if_no_docker() {
  if ! docker_available; then
    skip "Docker not available"
  fi
}

plugin_running() {
  curl -fsS "http://localhost:${WEBHOOKS_PORT}/health" >/dev/null 2>&1
}

skip_if_not_running() {
  if ! plugin_running; then
    skip "webhooks plugin not running on port $WEBHOOKS_PORT"
  fi
}

# ---------------------------------------------------------------------------
# Install / remove (dry-run)
# ---------------------------------------------------------------------------

@test "webhooks: nself plugin install --dry-run succeeds" {
  run nself plugin install webhooks --dry-run
  assert_success
}

@test "webhooks: nself plugin remove --dry-run succeeds" {
  run nself plugin remove webhooks --dry-run
  assert_success
}

# ---------------------------------------------------------------------------
# Health check
# ---------------------------------------------------------------------------

@test "webhooks: GET /health returns 200" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${WEBHOOKS_PORT}/health"
  assert_success
  assert_output --partial "ok"
}

@test "webhooks: GET /health includes plugin field" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${WEBHOOKS_PORT}/health"
  assert_success
  assert_output --partial "webhooks"
}

# ---------------------------------------------------------------------------
# Endpoint registration
# ---------------------------------------------------------------------------

@test "webhooks: POST /endpoints with missing url returns error" {
  skip_if_no_docker
  skip_if_not_running
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost:${WEBHOOKS_PORT}/endpoints" \
    -H "Content-Type: application/json" \
    -d '{"events":["test.created"]}')
  [ "$STATUS" = "400" ] || [ "$STATUS" = "422" ]
}

@test "webhooks: GET /endpoints returns list" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${WEBHOOKS_PORT}/endpoints"
  assert_success
}

# ---------------------------------------------------------------------------
# Delivery and retry
# ---------------------------------------------------------------------------

@test "webhooks: GET /deliveries returns list" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${WEBHOOKS_PORT}/deliveries"
  assert_success
}
