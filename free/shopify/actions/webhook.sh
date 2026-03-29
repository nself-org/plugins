#!/usr/bin/env bash
# Shopify Plugin - Webhook Management Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin shopify webhook - Webhook event management"
    echo ""
    echo "Usage: nself plugin shopify webhook [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List received webhook events"
    echo "  show <id>         Show event details"
    echo "  pending           List unprocessed events"
    echo "  retry <id>        Retry processing an event"
    echo "  stats             Webhook statistics"
    echo ""
}

list_events() {
    local topic=""
    local limit=50

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --topic) topic="$2"; shift 2 ;;
            --limit) limit="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local where_clause="WHERE 1=1"
    [[ -n "$topic" ]] && where_clause+=" AND topic = '$topic'"

    printf "%-36s %-25s %-10s %-20s\n" "ID" "Topic" "Processed" "Received"
    printf "%-36s %-25s %-10s %-20s\n" "------------------------------------" "-------------------------" "----------" "--------------------"

    plugin_db_query "SELECT id, topic, processed, received_at
                     FROM shopify_webhook_events
                     $where_clause
                     ORDER BY received_at DESC
                     LIMIT $limit" | while IFS='|' read -r id topic processed received; do
        local proc_str="No"
        [[ "$processed" == "t" ]] && proc_str="Yes"
        printf "%-36s %-25s %-10s %-20s\n" "${id:0:36}" "$topic" "$proc_str" "${received:0:19}"
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
        'Topic: ' || topic,
        'Shop: ' || COALESCE(shop_domain, 'n/a'),
        'Processed: ' || processed,
        'Processed At: ' || COALESCE(processed_at::text, 'n/a'),
        'Error: ' || COALESCE(error, 'none'),
        'Received: ' || received_at
    FROM shopify_webhook_events
    WHERE id = '$event_id'"
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

    plugin_log "info" "Retrying event: $event_id"
    plugin_db_query "UPDATE shopify_webhook_events SET processed = false, error = NULL WHERE id = '$event_id'"

    if [[ -x "${PLUGIN_DIR}/webhooks/handler.sh" ]]; then
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

    echo "By Topic:"
    plugin_db_query "SELECT topic, COUNT(*), COUNT(*) FILTER (WHERE processed = true)
                     FROM shopify_webhook_events
                     GROUP BY topic
                     ORDER BY COUNT(*) DESC" | while IFS='|' read -r topic total processed; do
        printf "  %-25s %5s total, %5s processed\n" "$topic" "$total" "$processed"
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
