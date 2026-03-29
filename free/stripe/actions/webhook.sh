#!/bin/bash
# =============================================================================
# Stripe Webhook Action
# Manage webhook configuration and view event history
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Commands
# =============================================================================

show_status() {
    printf "\n=== Stripe Webhook Status ===\n\n"

    printf "Configuration:\n"
    printf "  Webhook Secret: %s\n" "$([[ -n "${STRIPE_WEBHOOK_SECRET:-}" ]] && echo "Configured" || echo "Not set")"
    printf "  Endpoint Path:  /webhooks/stripe\n"

    printf "\nRecent events:\n"
    plugin_db_query "
        SELECT
            type,
            COUNT(*) AS count,
            SUM(CASE WHEN processed THEN 1 ELSE 0 END) AS processed,
            SUM(CASE WHEN error IS NOT NULL THEN 1 ELSE 0 END) AS errors
        FROM stripe_webhook_events
        WHERE received_at > NOW() - INTERVAL '24 hours'
        GROUP BY type
        ORDER BY count DESC
        LIMIT 10;
    "

    printf "\nLast 24 hours summary:\n"
    plugin_db_query "
        SELECT
            COUNT(*) AS total_events,
            SUM(CASE WHEN processed THEN 1 ELSE 0 END) AS processed,
            SUM(CASE WHEN error IS NOT NULL THEN 1 ELSE 0 END) AS with_errors
        FROM stripe_webhook_events
        WHERE received_at > NOW() - INTERVAL '24 hours';
    "
}

list_events() {
    local limit="${1:-20}"
    local type_filter="${2:-}"

    printf "\n=== Recent Webhook Events ===\n\n"

    local where_clause=""
    if [[ -n "$type_filter" ]]; then
        where_clause="AND type LIKE '%${type_filter}%'"
    fi

    plugin_db_query "
        SELECT
            id,
            type,
            object_id,
            CASE WHEN processed THEN 'Yes' ELSE 'No' END AS processed,
            CASE WHEN error IS NOT NULL THEN 'Error' ELSE '-' END AS status,
            to_char(received_at, 'YYYY-MM-DD HH24:MI:SS') AS received
        FROM stripe_webhook_events
        WHERE 1=1 $where_clause
        ORDER BY received_at DESC
        LIMIT $limit;
    "
}

get_event() {
    local event_id="$1"

    printf "\n=== Webhook Event: %s ===\n\n" "$event_id"

    plugin_db_query "
        SELECT
            id,
            type,
            api_version,
            object_type,
            object_id,
            processed,
            processed_at,
            error,
            retry_count,
            livemode,
            created_at,
            received_at
        FROM stripe_webhook_events
        WHERE id = '$event_id';
    "

    printf "\nEvent Data (truncated):\n"
    plugin_db_query "
        SELECT LEFT(data::text, 500) AS data_preview
        FROM stripe_webhook_events
        WHERE id = '$event_id';
    "
}

show_errors() {
    local limit="${1:-20}"

    printf "\n=== Failed Webhook Events ===\n\n"

    plugin_db_query "
        SELECT
            id,
            type,
            error,
            retry_count,
            to_char(received_at, 'YYYY-MM-DD HH24:MI') AS received
        FROM stripe_webhook_events
        WHERE error IS NOT NULL
        ORDER BY received_at DESC
        LIMIT $limit;
    "
}

retry_event() {
    local event_id="$1"

    plugin_info "Retrying event: $event_id"

    # Get event data
    local event_data
    event_data=$(plugin_db_query "SELECT data::text FROM stripe_webhook_events WHERE id = '$event_id';")

    if [[ -z "$event_data" ]]; then
        plugin_error "Event not found: $event_id"
        return 1
    fi

    # Reconstruct payload
    local event_type
    event_type=$(plugin_db_query "SELECT type FROM stripe_webhook_events WHERE id = '$event_id';")
    event_type=$(echo "$event_type" | xargs)

    local payload
    payload=$(printf '{"id":"%s","type":"%s",%s}' "$event_id" "$event_type" "${event_data:1}")

    # Process through handler
    if bash "${PLUGIN_DIR}/webhooks/handler.sh" "$payload"; then
        plugin_success "Event reprocessed successfully"
    else
        plugin_error "Event reprocessing failed"
        return 1
    fi
}

retry_all_failed() {
    local limit="${1:-10}"

    plugin_info "Retrying up to $limit failed events..."

    local failed_ids
    failed_ids=$(plugin_db_query "
        SELECT id FROM stripe_webhook_events
        WHERE error IS NOT NULL
          AND retry_count < 3
        ORDER BY received_at DESC
        LIMIT $limit;
    ")

    local count=0
    local success=0

    while IFS= read -r event_id; do
        event_id=$(echo "$event_id" | xargs)
        [[ -z "$event_id" ]] && continue

        ((count++))
        if retry_event "$event_id" 2>/dev/null; then
            ((success++))
        fi
    done <<< "$failed_ids"

    printf "\n"
    plugin_info "Retried $count events, $success succeeded"
}

# =============================================================================
# Main
# =============================================================================

show_help() {
    printf "Usage: nself plugin stripe webhook <command> [args]\n\n"
    printf "Commands:\n"
    printf "  status                 Show webhook configuration and stats\n"
    printf "  events [limit] [type]  List recent webhook events\n"
    printf "  get <event_id>         Get event details\n"
    printf "  errors [limit]         Show failed events\n"
    printf "  retry <event_id>       Retry a failed event\n"
    printf "  retry-all [limit]      Retry all failed events (default: 10)\n\n"
    printf "Examples:\n"
    printf "  nself plugin stripe webhook status\n"
    printf "  nself plugin stripe webhook events 50 customer\n"
    printf "  nself plugin stripe webhook retry evt_abc123\n"
}

main() {
    local command="${1:-status}"
    shift || true

    case "$command" in
        status)
            show_status
            ;;
        events|list)
            list_events "${1:-20}" "${2:-}"
            ;;
        get)
            if [[ -z "${1:-}" ]]; then
                plugin_error "Event ID required"
                return 1
            fi
            get_event "$1"
            ;;
        errors|failed)
            show_errors "${1:-20}"
            ;;
        retry)
            if [[ -z "${1:-}" ]]; then
                plugin_error "Event ID required"
                return 1
            fi
            retry_event "$1"
            ;;
        retry-all)
            retry_all_failed "${1:-10}"
            ;;
        -h|--help|help)
            show_help
            ;;
        *)
            plugin_error "Unknown command: $command"
            show_help
            return 1
            ;;
    esac
}

main "$@"
