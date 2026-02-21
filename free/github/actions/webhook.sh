#!/usr/bin/env bash
# GitHub Plugin - Webhook Management Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin github webhook - Webhook event management"
    echo ""
    echo "Usage: nself plugin github webhook [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List received webhook events"
    echo "  show <id>         Show event details"
    echo "  pending           List unprocessed events"
    echo "  retry <id>        Retry processing an event"
    echo "  stats             Webhook statistics"
    echo ""
    echo "Options:"
    echo "  --event <type>    Filter by event type"
    echo "  --repo <name>     Filter by repository"
    echo "  --limit <n>       Limit results (default: 50)"
    echo ""
}

list_events() {
    local event_type=""
    local repo=""
    local limit=50

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --event) event_type="$2"; shift 2 ;;
            --repo) repo="$2"; shift 2 ;;
            --limit) limit="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local where_clause="WHERE 1=1"
    [[ -n "$event_type" ]] && where_clause+=" AND event = '$event_type'"
    [[ -n "$repo" ]] && where_clause+=" AND repo_full_name = '$repo'"

    printf "%-36s %-20s %-15s %-10s %-20s\n" "ID" "Event" "Action" "Processed" "Received"
    printf "%-36s %-20s %-15s %-10s %-20s\n" "------------------------------------" "--------------------" "---------------" "----------" "--------------------"

    plugin_db_query "SELECT id, event, COALESCE(action, '-'), processed, received_at
                     FROM github_webhook_events
                     $where_clause
                     ORDER BY received_at DESC
                     LIMIT $limit" | while IFS='|' read -r id event action processed received; do
        local proc_str="No"
        [[ "$processed" == "t" ]] && proc_str="Yes"
        printf "%-36s %-20s %-15s %-10s %-20s\n" "${id:0:36}" "$event" "$action" "$proc_str" "${received:0:19}"
    done
}

show_event() {
    local event_id="$1"

    if [[ -z "$event_id" ]]; then
        plugin_log "error" "Event ID required"
        return 1
    fi

    echo "Webhook Event Details"
    echo "====================="

    plugin_db_query "SELECT
        'ID: ' || id,
        'Event: ' || event,
        'Action: ' || COALESCE(action, 'n/a'),
        'Repository: ' || COALESCE(repo_full_name, 'n/a'),
        'Sender: ' || COALESCE(sender_login, 'n/a'),
        'Processed: ' || processed,
        'Processed At: ' || COALESCE(processed_at::text, 'n/a'),
        'Error: ' || COALESCE(error, 'none'),
        'Received: ' || received_at
    FROM github_webhook_events
    WHERE id = '$event_id'"

    echo ""
    echo "Payload (first 500 chars):"
    plugin_db_query "SELECT SUBSTRING(data::text, 1, 500) FROM github_webhook_events WHERE id = '$event_id'"
}

list_pending() {
    list_events --processed false "$@"
}

retry_event() {
    local event_id="$1"

    if [[ -z "$event_id" ]]; then
        plugin_log "error" "Event ID required"
        return 1
    fi

    # Get event data
    local event_data
    event_data=$(plugin_db_query "SELECT event, action, data FROM github_webhook_events WHERE id = '$event_id'" 2>/dev/null)

    if [[ -z "$event_data" ]]; then
        plugin_log "error" "Event not found: $event_id"
        return 1
    fi

    plugin_log "info" "Retrying event: $event_id"

    # Reset processed status
    plugin_db_query "UPDATE github_webhook_events SET processed = false, error = NULL WHERE id = '$event_id'"

    # Dispatch to handler
    local event_type action
    event_type=$(echo "$event_data" | cut -d'|' -f1)
    action=$(echo "$event_data" | cut -d'|' -f2)

    if [[ -x "${PLUGIN_DIR}/webhooks/handler.sh" ]]; then
        # Re-process the event
        bash "${PLUGIN_DIR}/webhooks/handler.sh" "$event_id"
        plugin_log "success" "Event reprocessed"
    else
        plugin_log "error" "Webhook handler not found"
        return 1
    fi
}

show_stats() {
    echo "Webhook Statistics"
    echo "=================="
    echo ""

    echo "By Event Type:"
    plugin_db_query "SELECT event, COUNT(*), COUNT(*) FILTER (WHERE processed = true)
                     FROM github_webhook_events
                     GROUP BY event
                     ORDER BY COUNT(*) DESC" | while IFS='|' read -r event total processed; do
        printf "  %-25s %5s total, %5s processed\n" "$event" "$total" "$processed"
    done

    echo ""
    echo "Recent Errors:"
    plugin_db_query "SELECT event, error, received_at
                     FROM github_webhook_events
                     WHERE error IS NOT NULL
                     ORDER BY received_at DESC
                     LIMIT 5" | while IFS='|' read -r event error received; do
        printf "  %-20s %s\n" "$event" "${error:0:50}"
    done
}

main() {
    local subcommand="${1:-list}"
    shift 2>/dev/null || true

    case "$subcommand" in
        list) list_events "$@" ;;
        show) show_event "$@" ;;
        pending) list_pending "$@" ;;
        retry) retry_event "$@" ;;
        stats) show_stats ;;
        -h|--help) show_help ;;
        *) show_help; return 1 ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
