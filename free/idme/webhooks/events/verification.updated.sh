#!/bin/bash
# =============================================================================
# ID.me Verification Updated Handler
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Handler
# =============================================================================

PAYLOAD="${1:-{}}"

plugin_info "Processing verification update"

# Extract verification data
IDME_USER_ID=$(echo "$PAYLOAD" | grep -o '"user_id":"[^"]*"' | sed 's/"user_id":"\([^"]*\)"/\1/')
EMAIL=$(echo "$PAYLOAD" | grep -o '"email":"[^"]*"' | sed 's/"email":"\([^"]*\)"/\1/')
VERIFIED=$(echo "$PAYLOAD" | grep -o '"verified":[a-z]*' | sed 's/"verified"://')

# Update verification record
plugin_db_query "
    UPDATE idme_verifications
    SET
        verified = $VERIFIED,
        verified_at = CASE WHEN $VERIFIED THEN NOW() ELSE NULL END,
        last_synced_at = NOW(),
        updated_at = NOW()
    WHERE idme_user_id = '$IDME_USER_ID'
" || plugin_error "Failed to update verification"

plugin_success "Verification updated for: $EMAIL"
