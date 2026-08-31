#!/bin/bash
# =============================================================================
# ID.me Group Verified Handler
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Handler
# =============================================================================

PAYLOAD="${1:-{}}"

plugin_info "Processing group verification"

# Extract data
IDME_USER_ID=$(echo "$PAYLOAD" | grep -o '"user_id":"[^"]*"' | sed 's/"user_id":"\([^"]*\)"/\1/')
GROUP_TYPE=$(echo "$PAYLOAD" | grep -o '"group_type":"[^"]*"' | sed 's/"group_type":"\([^"]*\)"/\1/')
GROUP_NAME=$(echo "$PAYLOAD" | grep -o '"group_name":"[^"]*"' | sed 's/"group_name":"\([^"]*\)"/\1/')

# Get verification ID
VERIFICATION_ID=$(plugin_db_query "
    SELECT id FROM idme_verifications WHERE idme_user_id = '$IDME_USER_ID'
" | tr -d '[:space:]')

if [[ -z "$VERIFICATION_ID" ]]; then
    plugin_error "Verification not found for user: $IDME_USER_ID"
    return 1
fi

# Get user ID
USER_ID=$(plugin_db_query "
    SELECT user_id FROM idme_verifications WHERE id = '$VERIFICATION_ID'
" | tr -d '[:space:]')

# Update or insert group
plugin_db_query "
    INSERT INTO idme_groups (verification_id, user_id, group_type, group_name, verified, verified_at)
    VALUES ('$VERIFICATION_ID', '$USER_ID', '$GROUP_TYPE', '$GROUP_NAME', TRUE, NOW())
    ON CONFLICT (verification_id, group_type)
    DO UPDATE SET verified = TRUE, verified_at = NOW(), updated_at = NOW()
" || plugin_error "Failed to update group"

# Create badge
plugin_db_query "
    INSERT INTO idme_badges (verification_id, user_id, badge_type, badge_name, verified_at, active)
    VALUES ('$VERIFICATION_ID', '$USER_ID', '$GROUP_TYPE', '$GROUP_NAME', NOW(), TRUE)
    ON CONFLICT (verification_id, badge_type)
    DO UPDATE SET active = TRUE, verified_at = NOW(), updated_at = NOW()
" || plugin_error "Failed to create badge"

plugin_success "Group verified: $GROUP_TYPE for user $IDME_USER_ID"
