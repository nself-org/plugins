#!/bin/sh
# =============================================================================
# Forgejo bootstrap — runs inside forgejo-init container (one-shot, restart:no)
#
# Purpose:
#   1. Create the Forgejo admin user (idempotent — safe to re-run).
#   2. Obtain the Actions runner registration token via the Forgejo REST API.
#   3. Write the token to /runner-config/token (shared volume) so the
#      forgejo-runner container can self-register without any human input.
#
# Inputs  (env vars set by docker-compose.plugin.yml):
#   NSELF_FORGEJO_ADMIN_USER        — admin username to create
#   NSELF_FORGEJO_ADMIN_PASSWORD    — admin password
#   NSELF_FORGEJO_ADMIN_EMAIL       — admin email address
#   FORGEJO_RUNNER_REGISTRATION_TOKEN — optional pre-shared token (skip API call
#                                       when provided; useful for re-provisioning)
#
# Outputs:
#   /runner-config/token  — runner registration token (plaintext, chmod 600)
#
# Constraints:
#   - Runs inside the forgejo image (has the forgejo binary in PATH).
#   - Forgejo server is healthy before this script runs (depends_on health check).
#   - Script is idempotent: re-running after a token is already written is safe.
#   - Script uses only sh + wget (available in the forgejo container image).
#
# SPORT: F08-SERVICE-INVENTORY — forgejo, forgejo-runner (ops profile)
# =============================================================================

set -eu

FORGEJO_INTERNAL_URL="http://forgejo:3000"
TOKEN_FILE="/runner-config/token"

log() { echo "[forgejo-bootstrap] $*"; }
die() { echo "[forgejo-bootstrap] ERROR: $*" >&2; exit 1; }

# Validate required env vars.
: "${NSELF_FORGEJO_ADMIN_USER:?NSELF_FORGEJO_ADMIN_USER is required}"
: "${NSELF_FORGEJO_ADMIN_PASSWORD:?NSELF_FORGEJO_ADMIN_PASSWORD is required}"
: "${NSELF_FORGEJO_ADMIN_EMAIL:?NSELF_FORGEJO_ADMIN_EMAIL is required}"

# ─── Step 1: Create admin user (idempotent) ───────────────────────────────────
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

# ─── Step 2: Obtain runner registration token ──────────────────────────────────
if [ -n "${FORGEJO_RUNNER_REGISTRATION_TOKEN:-}" ]; then
  # Pre-shared token provided — skip API call (idempotent re-provision path).
  log "Pre-shared FORGEJO_RUNNER_REGISTRATION_TOKEN provided — using it directly."
  TOKEN="${FORGEJO_RUNNER_REGISTRATION_TOKEN}"
elif [ -f "${TOKEN_FILE}" ]; then
  # Token already written by a previous run — skip.
  log "Token already present at ${TOKEN_FILE} — skipping generation."
  exit 0
else
  # Generate a fresh runner token via the Forgejo REST API.
  # POST /api/v1/user/actions/runners/registration-token
  # Returns: {"token":"<value>"}
  log "Requesting runner registration token from Forgejo API..."
  RESPONSE=$(wget -qO- \
    --header "Content-Type: application/json" \
    --post-data "" \
    --auth-no-challenge \
    --http-user="${NSELF_FORGEJO_ADMIN_USER}" \
    --http-password="${NSELF_FORGEJO_ADMIN_PASSWORD}" \
    "${FORGEJO_INTERNAL_URL}/api/v1/user/actions/runners/registration-token") \
    || die "Failed to reach Forgejo API at ${FORGEJO_INTERNAL_URL} — is the server healthy?"

  # Extract token from JSON: {"token":"<value>"} using sh + sed (no jq needed).
  TOKEN=$(echo "${RESPONSE}" | sed 's/.*"token":"\([^"]*\)".*/\1/')
  if [ -z "${TOKEN}" ] || [ "${TOKEN}" = "${RESPONSE}" ]; then
    die "Could not parse token from Forgejo API response: ${RESPONSE}"
  fi
fi

# ─── Step 3: Write token to shared volume ─────────────────────────────────────
mkdir -p /runner-config
printf '%s' "${TOKEN}" > "${TOKEN_FILE}"
chmod 600 "${TOKEN_FILE}"
log "Runner registration token written to ${TOKEN_FILE}."
log "Bootstrap complete. forgejo-runner will self-register on startup."
