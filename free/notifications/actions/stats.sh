#!/bin/bash
# =============================================================================
# Notifications Statistics Action
# View notification metrics and analytics
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Statistics
# =============================================================================

show_overview() {
    plugin_info "Notification Statistics Overview"
    printf "\n"

    # Overall stats
    plugin_info "Overall (last 30 days)"
    printf "\n"

    plugin_db_query "
        SELECT
            channel,
            COUNT(*) AS total,
            COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
            COUNT(*) FILTER (WHERE status = 'failed') AS failed,
            COUNT(*) FILTER (WHERE status = 'pending') AS pending,
            ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'delivered') / NULLIF(COUNT(*), 0), 2) AS delivery_rate
        FROM notifications
        WHERE created_at >= NOW() - INTERVAL '30 days'
        GROUP BY channel
        ORDER BY channel;
    " 2>/dev/null | column -t -s '|'

    printf "\n"

    # By category
    plugin_info "By Category"
    printf "\n"

    plugin_db_query "
        SELECT
            category,
            COUNT(*) AS total,
            COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
            ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'delivered') / NULLIF(COUNT(*), 0), 2) AS delivery_rate
        FROM notifications
        WHERE created_at >= NOW() - INTERVAL '30 days'
        GROUP BY category
        ORDER BY total DESC;
    " 2>/dev/null | column -t -s '|'

    printf "\n"

    # Queue status
    plugin_info "Queue Status"
    printf "\n"

    plugin_db_query "
        SELECT
            status,
            COUNT(*) AS count,
            AVG(attempts) AS avg_attempts
        FROM notification_queue
        GROUP BY status
        ORDER BY status;
    " 2>/dev/null | column -t -s '|'

    printf "\n"
}

show_delivery_rates() {
    local days="${1:-7}"

    plugin_info "Delivery Rates (last $days days)"
    printf "\n"

    plugin_db_query "
        SELECT * FROM notification_delivery_rates
        WHERE date >= NOW() - INTERVAL '$days days'
        ORDER BY date DESC, channel
        LIMIT 50;
    " 2>/dev/null | column -t -s '|'
}

show_engagement() {
    local days="${1:-7}"

    plugin_info "Email Engagement (last $days days)"
    printf "\n"

    plugin_db_query "
        SELECT * FROM notification_engagement
        WHERE date >= NOW() - INTERVAL '$days days'
        ORDER BY date DESC
        LIMIT 50;
    " 2>/dev/null | column -t -s '|'
}

show_provider_health() {
    plugin_info "Provider Health Status"
    printf "\n"

    plugin_db_query "
        SELECT * FROM notification_provider_health
        ORDER BY type, success_rate DESC;
    " 2>/dev/null | column -t -s '|'
}

show_top_templates() {
    local limit="${1:-10}"

    plugin_info "Top Templates (last 30 days)"
    printf "\n"

    plugin_db_query "
        SELECT
            template_name,
            COUNT(*) AS total_sent,
            COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
            COUNT(*) FILTER (WHERE status = 'failed') AS failed,
            ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'delivered') / NULLIF(COUNT(*), 0), 2) AS delivery_rate
        FROM notifications
        WHERE created_at >= NOW() - INTERVAL '30 days'
          AND template_name IS NOT NULL
        GROUP BY template_name
        ORDER BY total_sent DESC
        LIMIT $limit;
    " 2>/dev/null | column -t -s '|'
}

show_failures() {
    local limit="${1:-20}"

    plugin_info "Recent Failures"
    printf "\n"

    plugin_db_query "
        SELECT
            to_char(created_at, 'YYYY-MM-DD HH24:MI') AS time,
            channel,
            template_name,
            error_code,
            error_message
        FROM notifications
        WHERE status = 'failed'
        ORDER BY created_at DESC
        LIMIT $limit;
    " 2>/dev/null | column -t -s '|'
}

show_hourly_volume() {
    local hours="${1:-24}"

    plugin_info "Hourly Volume (last $hours hours)"
    printf "\n"

    plugin_db_query "
        SELECT
            to_char(date_trunc('hour', created_at), 'YYYY-MM-DD HH24:00') AS hour,
            channel,
            COUNT(*) AS count
        FROM notifications
        WHERE created_at >= NOW() - INTERVAL '$hours hours'
        GROUP BY date_trunc('hour', created_at), channel
        ORDER BY date_trunc('hour', created_at) DESC, channel;
    " 2>/dev/null | column -t -s '|'
}

export_stats() {
    local format="${1:-json}"
    local output_file="${2:-stats-$(date +%Y%m%d-%H%M%S).$format}"

    plugin_info "Exporting statistics to: $output_file"

    case "$format" in
        json)
            plugin_db_query "
                SELECT json_build_object(
                    'generated_at', NOW(),
                    'overview', (
                        SELECT json_agg(row_to_json(t))
                        FROM (
                            SELECT
                                channel,
                                COUNT(*) AS total,
                                COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
                                COUNT(*) FILTER (WHERE status = 'failed') AS failed
                            FROM notifications
                            WHERE created_at >= NOW() - INTERVAL '30 days'
                            GROUP BY channel
                        ) t
                    ),
                    'providers', (SELECT json_agg(row_to_json(notification_provider_health)) FROM notification_provider_health),
                    'delivery_rates', (SELECT json_agg(row_to_json(notification_delivery_rates)) FROM notification_delivery_rates WHERE date >= NOW() - INTERVAL '7 days')
                );
            " > "$output_file" 2>/dev/null
            ;;
        csv)
            plugin_db_query "
                COPY (
                    SELECT
                        created_at,
                        channel,
                        category,
                        template_name,
                        status,
                        provider,
                        retry_count
                    FROM notifications
                    WHERE created_at >= NOW() - INTERVAL '30 days'
                    ORDER BY created_at DESC
                ) TO STDOUT WITH CSV HEADER;
            " > "$output_file" 2>/dev/null
            ;;
        *)
            plugin_error "Unsupported format: $format (use json or csv)"
            return 1
            ;;
    esac

    if [[ $? -eq 0 ]]; then
        plugin_success "Exported to: $output_file"
    else
        plugin_error "Export failed"
        return 1
    fi
}

# =============================================================================
# Main
# =============================================================================

show_stats() {
    local stat_type="${1:-overview}"
    shift || true

    case "$stat_type" in
        overview|summary)
            show_overview
            ;;
        delivery|deliveries)
            show_delivery_rates "$@"
            ;;
        engagement|email)
            show_engagement "$@"
            ;;
        providers|provider)
            show_provider_health
            ;;
        templates|template)
            show_top_templates "$@"
            ;;
        failures|errors|failed)
            show_failures "$@"
            ;;
        hourly|volume)
            show_hourly_volume "$@"
            ;;
        export)
            export_stats "$@"
            ;;
        *)
            show_help
            return 1
            ;;
    esac
}

# Show help
show_help() {
    printf "Usage: nself plugin notifications stats <type> [args]\n\n"
    printf "View notification statistics and analytics.\n\n"
    printf "Statistics types:\n"
    printf "  overview             Overall statistics (default)\n"
    printf "  delivery [days]      Delivery rates (default: 7 days)\n"
    printf "  engagement [days]    Email engagement metrics (default: 7 days)\n"
    printf "  providers            Provider health status\n"
    printf "  templates [limit]    Top templates (default: 10)\n"
    printf "  failures [limit]     Recent failures (default: 20)\n"
    printf "  hourly [hours]       Hourly volume (default: 24 hours)\n"
    printf "  export [format] [file]  Export stats (json or csv)\n\n"
    printf "Examples:\n"
    printf "  nself plugin notifications stats overview\n"
    printf "  nself plugin notifications stats delivery 30\n"
    printf "  nself plugin notifications stats failures 50\n"
    printf "  nself plugin notifications stats export json stats.json\n"
}

# Parse arguments
if [[ $# -eq 0 ]]; then
    show_overview
    exit 0
fi

case "${1:-}" in
    -h|--help|help)
        show_help
        ;;
    *)
        show_stats "$@"
        ;;
esac
