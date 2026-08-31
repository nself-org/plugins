#!/bin/bash
# =============================================================================
# ID.me Default Webhook Event Handler
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Handler
# =============================================================================

EVENT_TYPE="${1:-unknown}"
PAYLOAD="${2:-{}}"

plugin_info "Default handler for event: $EVENT_TYPE"

# Just log the event
plugin_info "Payload: $PAYLOAD"

# Store in database if not already stored
EVENT_ID=$(echo "$PAYLOAD" | grep -o '"id":"[^"]*"' | sed 's/"id":"\([^"]*\)"/\1/' || echo "unknown-$(date +%s)")

plugin_db_query "
    INSERT INTO idme_webhook_events (event_id, event_type, payload, received_at)
    VALUES ('$EVENT_ID', '$EVENT_TYPE', '$PAYLOAD'::jsonb, NOW())
    ON CONFLICT (event_id) DO NOTHING
" || true

plugin_success "Event logged: $EVENT_TYPE"
