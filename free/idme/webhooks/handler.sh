#!/bin/bash
# =============================================================================
# ID.me Webhook Handler
# Process incoming webhooks from ID.me
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Configuration
# =============================================================================

IDME_WEBHOOK_SECRET="${IDME_WEBHOOK_SECRET:-}"

# =============================================================================
# Webhook Processing
# =============================================================================

verify_signature() {
    local payload="$1"
    local signature="$2"

    if [[ -z "$IDME_WEBHOOK_SECRET" ]]; then
        plugin_warn "IDME_WEBHOOK_SECRET not set - skipping signature verification"
        return 0
    fi

    # Compute expected signature
    local expected
    expected=$(echo -n "$payload" | openssl dgst -sha256 -hmac "$IDME_WEBHOOK_SECRET" | cut -d' ' -f2)

    if [[ "$signature" != "$expected" ]]; then
        plugin_error "Invalid webhook signature"
        return 1
    fi

    return 0
}

process_webhook() {
    local event_type="$1"
    local payload="$2"

    plugin_info "Processing webhook: $event_type"

    # Extract common fields
    local event_id
    event_id=$(echo "$payload" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"\([^"]*\)"/\1/')

    local user_id
    user_id=$(echo "$payload" | grep -o '"user_id":"[^"]*"' | head -1 | sed 's/"user_id":"\([^"]*\)"/\1/' || echo "")

    # Store webhook event
    plugin_db_query "
        INSERT INTO idme_webhook_events (event_id, event_type, user_id, payload, received_at)
        VALUES ('$event_id', '$event_type', $([ -n "$user_id" ] && echo "'$user_id'" || echo "NULL"), '$payload'::jsonb, NOW())
        ON CONFLICT (event_id) DO UPDATE SET retry_count = idme_webhook_events.retry_count + 1
    " || plugin_error "Failed to store webhook event"

    # Route to specific handler
    local handler="${PLUGIN_DIR}/webhooks/events/${event_type}.sh"

    if [[ -f "$handler" ]]; then
        bash "$handler" "$payload" || plugin_error "Handler failed: $event_type"
    else
        # Use default handler
        bash "${PLUGIN_DIR}/webhooks/events/default.sh" "$event_type" "$payload"
    fi

    # Mark as processed
    plugin_db_query "
        UPDATE idme_webhook_events
        SET processed = TRUE, processed_at = NOW()
        WHERE event_id = '$event_id'
    " || true

    plugin_success "Webhook processed: $event_type"
}

# =============================================================================
# Main
# =============================================================================

main() {
    # Read from stdin if no args provided (for HTTP server integration)
    if [[ $# -eq 0 ]]; then
        local payload
        payload=$(cat)

        local event_type
        event_type=$(echo "$payload" | grep -o '"type":"[^"]*"' | sed 's/"type":"\([^"]*\)"/\1/')

        local signature="${HTTP_X_IDME_SIGNATURE:-}"

        # Verify signature
        if ! verify_signature "$payload" "$signature"; then
            plugin_error "Signature verification failed"
            return 1
        fi

        process_webhook "$event_type" "$payload"
    else
        # Manual invocation for testing
        local event_type="$1"
        local payload="$2"

        process_webhook "$event_type" "$payload"
    fi
}

main "$@"
