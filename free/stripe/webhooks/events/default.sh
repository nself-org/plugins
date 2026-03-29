#!/bin/bash
# =============================================================================
# Default Stripe Event Handler
# Handles events that don't have specific handlers
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Default Handler
# =============================================================================

handle_default_event() {
    local event_type="$1"
    local payload="$2"

    plugin_debug "Default handler for: $event_type"

    # Extract object type from event type (e.g., customer.created -> customer)
    local object_type
    object_type=$(echo "$event_type" | cut -d'.' -f1)

    # Log unhandled event
    plugin_info "No specific handler for $event_type - event recorded for audit"

    return 0
}

# Run
handle_default_event "$@"
