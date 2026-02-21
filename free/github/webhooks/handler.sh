#!/usr/bin/env bash
# GitHub Plugin - Webhook Handler
# Receives and processes GitHub webhook events

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

# ============================================================================
# Signature Verification
# ============================================================================

verify_signature() {
    local payload="$1"
    local signature="$2"

    if [[ -z "${GITHUB_WEBHOOK_SECRET:-}" ]]; then
        plugin_log "warning" "GITHUB_WEBHOOK_SECRET not set, skipping verification"
        return 0
    fi

    if [[ -z "$signature" ]]; then
        plugin_log "error" "No signature provided"
        return 1
    fi

    # GitHub uses sha256 HMAC
    local expected
    expected="sha256=$(echo -n "$payload" | openssl dgst -sha256 -hmac "$GITHUB_WEBHOOK_SECRET" | sed 's/^.* //')"

    if [[ "$signature" != "$expected" ]]; then
        plugin_log "error" "Invalid webhook signature"
        return 1
    fi

    return 0
}

# ============================================================================
# Event Processing
# ============================================================================

process_event() {
    local event_id="$1"
    local event_type="$2"
    local action="$3"
    local payload="$4"

    plugin_log "info" "Processing event: $event_type ($action)"

    # Route to specific handler
    local handler_script="${PLUGIN_DIR}/webhooks/events/${event_type}.sh"
    local default_handler="${PLUGIN_DIR}/webhooks/events/default.sh"

    if [[ -x "$handler_script" ]]; then
        bash "$handler_script" "$event_id" "$action" "$payload"
    elif [[ -x "$default_handler" ]]; then
        bash "$default_handler" "$event_id" "$event_type" "$action" "$payload"
    else
        plugin_log "warning" "No handler for event: $event_type"
    fi

    # Mark as processed
    plugin_db_query "UPDATE github_webhook_events SET processed = true, processed_at = NOW() WHERE id = '$event_id'"

    return 0
}

# ============================================================================
# Main Handler
# ============================================================================

handle_webhook() {
    local delivery_id="$1"
    local event_type="$2"
    local signature="$3"
    local payload="$4"

    # If called with just event ID, load from database
    if [[ -n "$delivery_id" ]] && [[ -z "$event_type" ]]; then
        local event_data
        event_data=$(plugin_db_query "SELECT event, action, data FROM github_webhook_events WHERE id = '$delivery_id'" 2>/dev/null)

        if [[ -n "$event_data" ]]; then
            event_type=$(echo "$event_data" | cut -d'|' -f1)
            local action=$(echo "$event_data" | cut -d'|' -f2)
            payload=$(echo "$event_data" | cut -d'|' -f3)

            process_event "$delivery_id" "$event_type" "$action" "$payload"
            return $?
        fi
    fi

    # Verify signature if secret is set
    if ! verify_signature "$payload" "$signature"; then
        return 1
    fi

    # Parse payload
    local action repo_id repo_name sender
    action=$(echo "$payload" | jq -r '.action // empty')
    repo_id=$(echo "$payload" | jq -r '.repository.id // empty')
    repo_name=$(echo "$payload" | jq -r '.repository.full_name // empty')
    sender=$(echo "$payload" | jq -r '.sender.login // empty')

    # Store event
    plugin_db_query "INSERT INTO github_webhook_events (
        id, event, action, repo_id, repo_full_name, sender_login, data, received_at
    ) VALUES (
        '$delivery_id',
        '$event_type',
        $(if [[ -n "$action" ]]; then echo "'$action'"; else echo "NULL"; fi),
        $(if [[ -n "$repo_id" ]]; then echo "$repo_id"; else echo "NULL"; fi),
        $(if [[ -n "$repo_name" ]]; then echo "'$repo_name'"; else echo "NULL"; fi),
        $(if [[ -n "$sender" ]]; then echo "'$sender'"; else echo "NULL"; fi),
        '$(echo "$payload" | jq -c .)',
        NOW()
    ) ON CONFLICT (id) DO NOTHING"

    # Process the event
    process_event "$delivery_id" "$event_type" "$action" "$payload"

    return 0
}

# Handle stdin input (for HTTP handler integration)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    if [[ $# -ge 1 ]]; then
        # Called with arguments
        handle_webhook "$@"
    else
        # Read from environment (set by webhook receiver)
        delivery_id="${HTTP_X_GITHUB_DELIVERY:-}"
        event_type="${HTTP_X_GITHUB_EVENT:-}"
        signature="${HTTP_X_HUB_SIGNATURE_256:-}"
        payload=$(cat)

        if [[ -n "$delivery_id" ]] && [[ -n "$event_type" ]]; then
            handle_webhook "$delivery_id" "$event_type" "$signature" "$payload"
        else
            echo "Error: Missing required headers" >&2
            exit 1
        fi
    fi
fi
