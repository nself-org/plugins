#!/bin/bash
# =============================================================================
# nself Plugin Webhook Handler
# Generic webhook processing for nself plugins
# =============================================================================

set -euo pipefail

# Source plugin utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/plugin-utils.sh"

# =============================================================================
# Configuration
# =============================================================================

WEBHOOK_PORT="${WEBHOOK_PORT:-8088}"
WEBHOOK_HOST="${WEBHOOK_HOST:-0.0.0.0}"
WEBHOOK_LOG_LEVEL="${WEBHOOK_LOG_LEVEL:-info}"

# =============================================================================
# Webhook Server Functions
# =============================================================================

# Start webhook listener using netcat (for development/testing)
webhook_start_listener() {
    local plugin_name="$1"
    local handler_script="$2"

    plugin_info "Starting webhook listener for $plugin_name on port $WEBHOOK_PORT"

    while true; do
        # Read HTTP request
        local request=""
        local content_length=0
        local body=""

        {
            # Read headers
            while IFS= read -r line; do
                line="${line%$'\r'}"
                [[ -z "$line" ]] && break

                if [[ "$line" =~ ^Content-Length:\ ([0-9]+) ]]; then
                    content_length="${BASH_REMATCH[1]}"
                fi

                request+="$line"$'\n'
            done

            # Read body
            if [[ $content_length -gt 0 ]]; then
                body=$(head -c "$content_length")
            fi

            # Process webhook
            local response
            if response=$("$handler_script" "$body" 2>&1); then
                printf "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"received\":true}"
            else
                printf "HTTP/1.1 500 Internal Server Error\r\nContent-Type: application/json\r\n\r\n{\"error\":\"%s\"}" "$response"
            fi

        } | nc -l "$WEBHOOK_PORT"
    done
}

# =============================================================================
# Webhook Processing
# =============================================================================

# Process incoming webhook
webhook_process() {
    local plugin_name="$1"
    local event_type="$2"
    local payload="$3"
    local signature="${4:-}"

    plugin_debug "Processing webhook: $plugin_name/$event_type"

    # Find and execute the event handler
    local handler_dir="${PLUGIN_DIR}/${plugin_name}/webhooks/events"
    local event_file
    event_file=$(echo "$event_type" | tr '.' '_')

    if [[ -f "${handler_dir}/${event_file}.sh" ]]; then
        plugin_debug "Executing handler: ${event_file}.sh"
        bash "${handler_dir}/${event_file}.sh" "$payload"
    elif [[ -f "${handler_dir}/default.sh" ]]; then
        plugin_debug "Executing default handler"
        bash "${handler_dir}/default.sh" "$event_type" "$payload"
    else
        plugin_warn "No handler found for event: $event_type"
        return 0
    fi
}

# =============================================================================
# Webhook Queue (for reliable processing)
# =============================================================================

WEBHOOK_QUEUE_DIR="${WEBHOOK_QUEUE_DIR:-$HOME/.nself/queue/webhooks}"

# Add webhook to queue
webhook_queue_add() {
    local plugin_name="$1"
    local event_type="$2"
    local payload="$3"

    local queue_dir="${WEBHOOK_QUEUE_DIR}/${plugin_name}"
    mkdir -p "$queue_dir"

    local event_id
    event_id=$(date +%s%N)
    local event_file="${queue_dir}/${event_id}.json"

    printf '{"event_type":"%s","payload":%s,"queued_at":"%s"}' \
        "$event_type" "$payload" "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" > "$event_file"

    plugin_debug "Queued webhook event: $event_id"
    printf '%s' "$event_id"
}

# Process webhook queue
webhook_queue_process() {
    local plugin_name="$1"
    local queue_dir="${WEBHOOK_QUEUE_DIR}/${plugin_name}"

    [[ ! -d "$queue_dir" ]] && return 0

    local processed=0
    local failed=0

    for event_file in "$queue_dir"/*.json; do
        [[ ! -f "$event_file" ]] && continue

        local event_data
        event_data=$(cat "$event_file")

        local event_type
        local payload
        event_type=$(plugin_json_get "$event_data" "event_type")
        payload=$(printf '%s' "$event_data" | grep -o '"payload":[^}]*}' | sed 's/"payload"://')

        if webhook_process "$plugin_name" "$event_type" "$payload"; then
            rm -f "$event_file"
            ((processed++))
        else
            ((failed++))
        fi
    done

    plugin_info "Processed $processed webhooks, $failed failed"
}

# =============================================================================
# Main
# =============================================================================

webhook_main() {
    local action="${1:-}"
    local plugin_name="${2:-}"

    case "$action" in
        start)
            webhook_start_listener "$plugin_name" "${3:-}"
            ;;
        process)
            local event_type="$3"
            local payload="$4"
            webhook_process "$plugin_name" "$event_type" "$payload"
            ;;
        queue-add)
            local event_type="$3"
            local payload="$4"
            webhook_queue_add "$plugin_name" "$event_type" "$payload"
            ;;
        queue-process)
            webhook_queue_process "$plugin_name"
            ;;
        *)
            printf "Usage: webhook-handler.sh <action> <plugin_name> [args...]\n"
            printf "Actions: start, process, queue-add, queue-process\n"
            return 1
            ;;
    esac
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    webhook_main "$@"
fi
