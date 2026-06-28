#!/bin/sh
# =============================================================================
# Forgejo runner entrypoint — executed inside forgejo-runner container.
#
# Purpose:
#   Wait for the registration token written by forgejo-init, then register
#   the runner against the local Forgejo instance (idempotent) and start
#   the daemon.
#
# Inputs (env vars):
#   FORGEJO_INSTANCE_URL     — Forgejo server URL (e.g. http://forgejo:3000)
#   FORGEJO_RUNNER_NAME      — Runner display name
#   FORGEJO_RUNNER_LABELS    — Comma-separated label:image pairs
#   FORGEJO_RUNNER_CAPACITY  — Max concurrent jobs
#
# Inputs (files on shared volume /data):
#   /data/token   — Registration token written by forgejo-bootstrap.sh
#   /data/.runner — State file written by forgejo-runner register (if present,
#                   registration is already done — skip)
#
# Outputs: starts `forgejo-runner daemon` as PID 1.
#
# Constraints:
#   - Only sh is guaranteed (no bash, no jq).
#   - Re-entrant: if /data/.runner exists, skip registration entirely.
#   - Token wait: 60s timeout (30 × 2s sleeps) — fails hard if forgejo-init
#     did not complete within that window.
# =============================================================================

set -eu

TOKEN_FILE="/data/token"
RUNNER_STATE="/data/.runner"

log() { echo "[runner-entrypoint] $*"; }
die() { echo "[runner-entrypoint] ERROR: $*" >&2; exit 1; }

# ─── Wait for registration token ─────────────────────────────────────────────
if [ ! -f "$TOKEN_FILE" ]; then
  log "Waiting for token from forgejo-init (up to 60s)..."
  i=0
  while [ "$i" -lt 30 ]; do
    [ -f "$TOKEN_FILE" ] && break
    i=$((i + 1))
    log "  ($i/30 — waiting 2s...)"
    sleep 2
  done
  if [ ! -f "$TOKEN_FILE" ]; then
    die "Token file not found after 60s. Did forgejo-init complete successfully? Check: docker logs <project>_forgejo_init"
  fi
fi

TOKEN=$(cat "$TOKEN_FILE")
[ -z "$TOKEN" ] && die "Token file is empty — forgejo-init may have failed."

# ─── Register (idempotent) ────────────────────────────────────────────────────
if [ -f "$RUNNER_STATE" ]; then
  log "Runner already registered (.runner state file present) — skipping registration."
else
  log "Registering runner '${FORGEJO_RUNNER_NAME:-nself-runner}' against ${FORGEJO_INSTANCE_URL:-http://forgejo:3000}..."
  forgejo-runner register \
    --instance "${FORGEJO_INSTANCE_URL:-http://forgejo:3000}" \
    --token    "$TOKEN" \
    --name     "${FORGEJO_RUNNER_NAME:-nself-runner}" \
    --labels   "${FORGEJO_RUNNER_LABELS:-ubuntu-latest:docker://node:20-bullseye}" \
    --no-interactive
  log "Registration complete."
fi

# ─── Start daemon ─────────────────────────────────────────────────────────────
log "Starting daemon (capacity=${FORGEJO_RUNNER_CAPACITY:-1})..."
exec forgejo-runner daemon --capacity "${FORGEJO_RUNNER_CAPACITY:-1}"
