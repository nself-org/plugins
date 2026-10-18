#!/bin/sh
# =============================================================================
# Forgejo bootstrap — runs inside forgejo-init container (one-shot, restart:no)
#
# Purpose:
#   1. Create the Forgejo admin user (idempotent — safe to re-run).
#
# This stack has no in-stack runner (ADR 0027, Epic TRUST D22), so no runner
# registration token is created or stored here.
#
# Inputs  (env vars set by docker-compose.plugin.yml):
#   NSELF_FORGEJO_ADMIN_USER        — admin username to create
#   NSELF_FORGEJO_ADMIN_PASSWORD    — admin password
#   NSELF_FORGEJO_ADMIN_EMAIL       — admin email address
#
# Constraints:
#   - Runs inside the forgejo image (has the forgejo binary in PATH).
#   - Forgejo server is healthy before this script runs (depends_on health check).
#   - Script is idempotent: re-running when the admin user exists is safe.
#
# SPORT: F08-SERVICE-INVENTORY — forgejo (ops profile)
# =============================================================================

set -eu

log() { echo "[forgejo-bootstrap] $*"; }

# Validate required env vars.
: "${NSELF_FORGEJO_ADMIN_USER:?NSELF_FORGEJO_ADMIN_USER is required}"
: "${NSELF_FORGEJO_ADMIN_PASSWORD:?NSELF_FORGEJO_ADMIN_PASSWORD is required}"
: "${NSELF_FORGEJO_ADMIN_EMAIL:?NSELF_FORGEJO_ADMIN_EMAIL is required}"

# ─── Create admin user (idempotent) ───────────────────────────────────
log "Creating admin user '${NSELF_FORGEJO_ADMIN_USER}' (idempotent)..."
# forgejo admin user create exits 0 if the user already exists (Forgejo ≥1.19).
# We pipe stderr to /dev/null to suppress the "already exists" message.
forgejo admin user create \
  --admin \
  --username "${NSELF_FORGEJO_ADMIN_USER}" \
  --password "${NSELF_FORGEJO_ADMIN_PASSWORD}" \
  --email    "${NSELF_FORGEJO_ADMIN_EMAIL}" \
  --must-change-password=false \
  --config /data/gitea/conf/app.ini \
  2>/dev/null || log "Admin user already exists — skipping creation."

log "Bootstrap complete. Forgejo runs no CI jobs itself; register an external runner on a CI node (see README)."
