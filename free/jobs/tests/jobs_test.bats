#!/usr/bin/env bats
# jobs_test.bats
# CLI + HTTP tests for the jobs (BullMQ queue) free plugin.
# HTTP tests require nself services running with jobs plugin installed.

JOBS_PORT=3105

docker_available() {
  command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1
}

skip_if_no_docker() {
  if ! docker_available; then
    skip "Docker not available"
  fi
}

plugin_running() {
  curl -fsS "http://localhost:${JOBS_PORT}/health" >/dev/null 2>&1
}

skip_if_not_running() {
  if ! plugin_running; then
    skip "jobs plugin not running on port $JOBS_PORT"
  fi
}

# ---------------------------------------------------------------------------
# Install / remove (dry-run)
# ---------------------------------------------------------------------------

@test "jobs: nself plugin install --dry-run succeeds" {
  run nself plugin install jobs --dry-run
  assert_success
}

@test "jobs: nself plugin remove --dry-run succeeds" {
  run nself plugin remove jobs --dry-run
  assert_success
}

# ---------------------------------------------------------------------------
# Health check
# ---------------------------------------------------------------------------

@test "jobs: GET /health returns 200" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${JOBS_PORT}/health"
  assert_success
  assert_output --partial "ok\|healthy"
}

@test "jobs: GET /health includes plugin name" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${JOBS_PORT}/health"
  assert_success
  assert_output --partial "jobs"
}

# ---------------------------------------------------------------------------
# Queue endpoint tests
# ---------------------------------------------------------------------------

@test "jobs: GET /queues returns list" {
  skip_if_no_docker
  skip_if_not_running
  run curl -fsS "http://localhost:${JOBS_PORT}/queues"
  assert_success
}

@test "jobs: POST /jobs with missing fields returns 400" {
  skip_if_no_docker
  skip_if_not_running
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost:${JOBS_PORT}/jobs" \
    -H "Content-Type: application/json" \
    -d '{}')
  [ "$STATUS" = "400" ] || [ "$STATUS" = "422" ]
}

@test "jobs: POST /jobs with valid payload returns 201 or 200" {
  skip_if_no_docker
  skip_if_not_running
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "http://localhost:${JOBS_PORT}/jobs" \
    -H "Content-Type: application/json" \
    -d '{"queue":"test-queue","name":"test-job","data":{"key":"value"}}')
  [ "$STATUS" = "200" ] || [ "$STATUS" = "201" ]
}
