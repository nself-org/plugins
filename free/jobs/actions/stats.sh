#!/bin/bash
# =============================================================================
# Jobs Stats Action
# View job statistics and metrics
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Statistics
# =============================================================================

show_stats() {
    local queue_name="${1:-}"
    local hours="${2:-24}"

    plugin_info "Job Statistics"
    printf "\n"

    # Overall stats
    printf "=== Overall Statistics (last %s hours) ===\n" "$hours"
    plugin_db_query "
        SELECT
            metric,
            value
        FROM get_job_stats($([ -n "$queue_name" ] && echo "'$queue_name'" || echo "NULL"), $hours)
        ORDER BY
            CASE metric
                WHEN 'total_jobs' THEN 1
                WHEN 'waiting' THEN 2
                WHEN 'active' THEN 3
                WHEN 'completed' THEN 4
                WHEN 'failed' THEN 5
            END;
    " | column -t -s '|'

    printf "\n"

    # Queue stats
    printf "=== Queue Statistics ===\n"
    plugin_db_query "
        SELECT
            queue_name,
            waiting,
            active,
            completed,
            failed,
            delayed,
            total,
            ROUND(avg_duration_seconds::numeric, 2) AS avg_duration_sec
        FROM queue_stats
        $([ -n "$queue_name" ] && echo "WHERE queue_name = '$queue_name'")
        ORDER BY queue_name;
    " | column -t -s '|'

    printf "\n"

    # Job type stats
    printf "=== Job Type Statistics ===\n"
    plugin_db_query "
        SELECT
            job_type,
            total_jobs,
            completed,
            failed,
            COALESCE(success_rate, 0) AS success_rate,
            ROUND(COALESCE(avg_duration_seconds, 0)::numeric, 2) AS avg_duration_sec
        FROM job_type_stats
        ORDER BY total_jobs DESC
        LIMIT 20;
    " | column -t -s '|'

    printf "\n"

    # Recent failures
    printf "=== Recent Failures (last 24 hours) ===\n"
    plugin_db_query "
        SELECT
            job_type,
            queue_name,
            error_message,
            attempt_number,
            CASE WHEN will_retry THEN 'Yes' ELSE 'No' END AS will_retry,
            TO_CHAR(failed_at, 'YYYY-MM-DD HH24:MI:SS') AS failed_at
        FROM recent_failures
        ORDER BY failed_at DESC
        LIMIT 10;
    " | column -t -s '|'

    printf "\n"

    # Active jobs
    printf "=== Active Jobs ===\n"
    plugin_db_query "
        SELECT
            job_type,
            queue_name,
            progress || '%' AS progress,
            running_seconds || 's' AS running,
            worker_id
        FROM jobs_active
        ORDER BY started_at
        LIMIT 10;
    " | column -t -s '|'

    printf "\n"

    # Scheduled jobs
    printf "=== Scheduled Jobs ===\n"
    plugin_db_query "
        SELECT
            name,
            job_type,
            cron_expression,
            CASE WHEN enabled THEN 'Yes' ELSE 'No' END AS enabled,
            total_runs,
            COALESCE(success_rate, 0) AS success_rate,
            CASE
                WHEN seconds_until_next_run < 0 THEN 'Overdue'
                WHEN seconds_until_next_run < 60 THEN seconds_until_next_run || 's'
                WHEN seconds_until_next_run < 3600 THEN ROUND(seconds_until_next_run / 60.0) || 'm'
                ELSE ROUND(seconds_until_next_run / 3600.0) || 'h'
            END AS next_run_in
        FROM scheduled_jobs_overview
        ORDER BY next_run_at
        LIMIT 10;
    " | column -t -s '|'

    printf "\n"
}

# Show performance metrics
show_performance() {
    plugin_info "Performance Metrics (last 24 hours)"
    printf "\n"

    # Job throughput
    printf "=== Job Throughput ===\n"
    plugin_db_query "
        SELECT
            DATE_TRUNC('hour', created_at) AS hour,
            COUNT(*) AS total_jobs,
            COUNT(*) FILTER (WHERE status = 'completed') AS completed,
            COUNT(*) FILTER (WHERE status = 'failed') AS failed,
            ROUND(AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) FILTER (WHERE status = 'completed'), 2) AS avg_duration_sec
        FROM jobs
        WHERE created_at > NOW() - INTERVAL '24 hours'
        GROUP BY DATE_TRUNC('hour', created_at)
        ORDER BY hour DESC
        LIMIT 24;
    " | column -t -s '|'

    printf "\n"

    # Slowest jobs
    printf "=== Slowest Jobs (last 24 hours) ===\n"
    plugin_db_query "
        SELECT
            job_type,
            queue_name,
            ROUND(EXTRACT(EPOCH FROM (completed_at - started_at)), 2) AS duration_sec,
            TO_CHAR(completed_at, 'YYYY-MM-DD HH24:MI:SS') AS completed_at
        FROM jobs
        WHERE status = 'completed'
          AND completed_at > NOW() - INTERVAL '24 hours'
          AND completed_at IS NOT NULL
          AND started_at IS NOT NULL
        ORDER BY (completed_at - started_at) DESC
        LIMIT 10;
    " | column -t -s '|'

    printf "\n"

    # Retry stats
    printf "=== Retry Statistics ===\n"
    plugin_db_query "
        SELECT
            job_type,
            COUNT(*) AS total_with_retries,
            AVG(retry_count) AS avg_retries,
            MAX(retry_count) AS max_retries
        FROM jobs
        WHERE retry_count > 0
          AND created_at > NOW() - INTERVAL '24 hours'
        GROUP BY job_type
        ORDER BY total_with_retries DESC;
    " | column -t -s '|'

    printf "\n"
}

# Show help
show_help() {
    printf "Usage: nself plugin jobs stats [OPTIONS]\n\n"
    printf "View job statistics and performance metrics.\n\n"
    printf "Options:\n"
    printf "  -q, --queue QUEUE    Filter by queue name\n"
    printf "  -t, --time HOURS     Time window in hours (default: 24)\n"
    printf "  -p, --performance    Show detailed performance metrics\n"
    printf "  -w, --watch          Watch mode (refresh every 5 seconds)\n"
    printf "  -h, --help           Show this help\n\n"
    printf "Examples:\n"
    printf "  nself plugin jobs stats                    # Overall stats\n"
    printf "  nself plugin jobs stats -q default         # Stats for 'default' queue\n"
    printf "  nself plugin jobs stats -t 48              # Last 48 hours\n"
    printf "  nself plugin jobs stats -p                 # Performance metrics\n"
    printf "  nself plugin jobs stats -w                 # Watch mode\n"
}

# Parse arguments
QUEUE=""
HOURS=24
PERFORMANCE=false
WATCH=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        -q|--queue)
            QUEUE="$2"
            shift 2
            ;;
        -t|--time)
            HOURS="$2"
            shift 2
            ;;
        -p|--performance)
            PERFORMANCE=true
            shift
            ;;
        -w|--watch)
            WATCH=true
            shift
            ;;
        -h|--help|help)
            show_help
            exit 0
            ;;
        *)
            plugin_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Main execution
if [[ "$WATCH" == "true" ]]; then
    while true; do
        clear
        if [[ "$PERFORMANCE" == "true" ]]; then
            show_performance
        else
            show_stats "$QUEUE" "$HOURS"
        fi
        printf "Refreshing in 5 seconds... (Ctrl+C to stop)\n"
        sleep 5
    done
else
    if [[ "$PERFORMANCE" == "true" ]]; then
        show_performance
    else
        show_stats "$QUEUE" "$HOURS"
    fi
fi
