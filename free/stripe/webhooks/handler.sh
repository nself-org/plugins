#!/bin/bash
# =============================================================================
# Stripe Webhook Handler
# Receives and processes Stripe webhook events
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Configuration
# =============================================================================

STRIPE_API_KEY="${STRIPE_API_KEY:-}"
STRIPE_WEBHOOK_SECRET="${STRIPE_WEBHOOK_SECRET:-}"

# =============================================================================
# Signature Verification
# =============================================================================

verify_stripe_signature() {
    local payload="$1"
    local signature_header="$2"
    local secret="$3"

    # Parse Stripe signature header
    local timestamp signature
    timestamp=$(echo "$signature_header" | sed -n 's/.*t=\([0-9]*\).*/\1/p')
    signature=$(echo "$signature_header" | sed -n 's/.*v1=\([a-f0-9]*\).*/\1/p')

    if [[ -z "$timestamp" || -z "$signature" ]]; then
        plugin_error "Invalid signature header format"
        return 1
    fi

    # Check timestamp tolerance (5 minutes)
    local current_time
    current_time=$(date +%s)
    local tolerance=300

    if (( current_time - timestamp > tolerance )); then
        plugin_error "Webhook timestamp too old"
        return 1
    fi

    # Compute expected signature
    local signed_payload="${timestamp}.${payload}"
    local expected_signature
    expected_signature=$(printf '%s' "$signed_payload" | openssl dgst -sha256 -hmac "$secret" | sed 's/^.* //')

    if [[ "$signature" != "$expected_signature" ]]; then
        plugin_error "Signature verification failed"
        return 1
    fi

    return 0
}

# =============================================================================
# Event Recording
# =============================================================================

record_webhook_event() {
    local event_id="$1"
    local event_type="$2"
    local api_version="$3"
    local data="$4"
    local object_type="$5"
    local object_id="$6"
    local created_at="$7"
    local livemode="$8"

    # Escape data for SQL
    local escaped_data
    escaped_data=$(printf '%s' "$data" | sed "s/'/''/g")

    plugin_db_query "
        INSERT INTO stripe_webhook_events (
            id, type, api_version, data, object_type, object_id,
            created_at, livemode, received_at
        ) VALUES (
            '$event_id',
            '$event_type',
            '$api_version',
            '$escaped_data'::jsonb,
            '$object_type',
            '$object_id',
            to_timestamp($created_at),
            $livemode,
            NOW()
        )
        ON CONFLICT (id) DO UPDATE SET
            received_at = NOW(),
            retry_count = stripe_webhook_events.retry_count + 1;
    " >/dev/null
}

mark_event_processed() {
    local event_id="$1"
    local error="${2:-}"

    if [[ -n "$error" ]]; then
        local escaped_error
        escaped_error=$(printf '%s' "$error" | sed "s/'/''/g")
        plugin_db_query "
            UPDATE stripe_webhook_events
            SET processed = FALSE, error = '$escaped_error'
            WHERE id = '$event_id';
        " >/dev/null
    else
        plugin_db_query "
            UPDATE stripe_webhook_events
            SET processed = TRUE, processed_at = NOW(), error = NULL
            WHERE id = '$event_id';
        " >/dev/null
    fi
}

# =============================================================================
# Event Processing
# =============================================================================

process_webhook() {
    local payload="$1"
    local signature_header="${2:-}"

    plugin_debug "Processing Stripe webhook..."

    # Verify signature if secret is configured
    if [[ -n "$STRIPE_WEBHOOK_SECRET" && -n "$signature_header" ]]; then
        if ! verify_stripe_signature "$payload" "$signature_header" "$STRIPE_WEBHOOK_SECRET"; then
            plugin_error "Webhook signature verification failed"
            return 1
        fi
        plugin_debug "Signature verified"
    fi

    # Parse event
    local event_id event_type api_version data object_type object_id created_at livemode

    event_id=$(plugin_json_get "$payload" "id")
    event_type=$(plugin_json_get "$payload" "type")
    api_version=$(plugin_json_get "$payload" "api_version")
    created_at=$(printf '%s' "$payload" | grep -o '"created":[0-9]*' | head -1 | sed 's/"created"://')
    livemode=$(printf '%s' "$payload" | grep -o '"livemode":[a-z]*' | head -1 | sed 's/"livemode"://')

    # Extract object info
    object_type=$(printf '%s' "$payload" | grep -o '"object":"[^"]*"' | head -1 | sed 's/"object":"\([^"]*\)"/\1/')
    object_id=$(printf '%s' "$payload" | grep -o '"data":{[^}]*"object":{[^}]*"id":"[^"]*"' | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"\([^"]*\)"/\1/')

    # Get data object
    data=$(printf '%s' "$payload" | grep -o '"data":{.*}' | head -1)

    plugin_info "Received event: $event_type ($event_id)"

    # Record event in database
    record_webhook_event "$event_id" "$event_type" "$api_version" "$data" "$object_type" "$object_id" "$created_at" "$livemode"

    # Find and execute event handler
    local event_handler_dir="${PLUGIN_DIR}/webhooks/events"
    local event_file
    event_file=$(echo "$event_type" | tr '.' '_')

    local handler_result=0

    if [[ -f "${event_handler_dir}/${event_file}.sh" ]]; then
        plugin_debug "Executing handler: ${event_file}.sh"
        if bash "${event_handler_dir}/${event_file}.sh" "$payload"; then
            plugin_success "Event processed: $event_type"
        else
            handler_result=$?
            plugin_error "Handler failed for: $event_type"
        fi
    else
        plugin_debug "No specific handler for $event_type, using default sync"
        # Default: sync the object
        bash "${event_handler_dir}/default.sh" "$event_type" "$payload" || handler_result=$?
    fi

    # Mark event as processed
    if [[ $handler_result -eq 0 ]]; then
        mark_event_processed "$event_id"
    else
        mark_event_processed "$event_id" "Handler returned exit code $handler_result"
    fi

    plugin_log_webhook "stripe" "$event_type" "$event_id" "$([[ $handler_result -eq 0 ]] && echo 'success' || echo 'failed')"

    return $handler_result
}

# =============================================================================
# Main
# =============================================================================

main() {
    local payload="${1:-}"
    local signature_header="${STRIPE_SIGNATURE:-}"

    if [[ -z "$payload" ]]; then
        # Read from stdin if no argument
        payload=$(cat)
    fi

    if [[ -z "$payload" ]]; then
        plugin_error "No payload received"
        return 1
    fi

    process_webhook "$payload" "$signature_header"
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
