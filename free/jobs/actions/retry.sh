#!/bin/bash
# =============================================================================
# Jobs Retry Action
# Retry failed jobs
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Retry Failed Jobs
# =============================================================================

retry_failed() {
    local queue_name="${1:-}"
    local job_type="${2:-}"
    local limit="${3:-10}"

    plugin_info "Retrying failed jobs..."

    # Build WHERE clause
    local where_clause="status = 'failed' AND retry_count < max_retries"
    if [[ -n "$queue_name" ]]; then
        where_clause="$where_clause AND queue_name = '$queue_name'"
    fi
    if [[ -n "$job_type" ]]; then
        where_clause="$where_clause AND job_type = '$job_type'"
    fi

    # Get failed jobs
    local failed_jobs
    failed_jobs=$(plugin_db_query "
        SELECT id, job_type, queue_name, retry_count, max_retries
        FROM jobs
        WHERE $where_clause
        ORDER BY failed_at DESC
        LIMIT $limit;
    ")

    if [[ -z "$failed_jobs" ]]; then
        plugin_warn "No failed jobs found to retry"
        return 0
    fi

    # Count jobs to retry
    local count
    count=$(echo "$failed_jobs" | grep -c "^" || echo "0")

    printf "\n"
    printf "Found %s failed job(s) to retry:\n" "$count"
    echo "$failed_jobs" | column -t -s '|'
    printf "\n"

    read -p "Retry these jobs? (yes/no): " confirm
    if [[ "$confirm" != "yes" ]]; then
        plugin_warn "Retry cancelled"
        return 0
    fi

    # Reset jobs to 'waiting' status
    plugin_db_query "
        UPDATE jobs
        SET
            status = 'waiting',
            updated_at = NOW()
        WHERE $where_clause
        LIMIT $limit;
    "

    # Use TypeScript CLI to re-queue jobs if available
    if [[ -f "${PLUGIN_DIR}/ts/dist/cli.js" ]]; then
        plugin_info "Re-queuing jobs in BullMQ..."
        node "${PLUGIN_DIR}/ts/dist/cli.js" retry \
            $([ -n "$queue_name" ] && echo "--queue $queue_name") \
            $([ -n "$job_type" ] && echo "--type $job_type") \
            --limit "$limit" 2>/dev/null || plugin_warn "Failed to re-queue some jobs"
    fi

    plugin_success "Retried $count job(s)"
}

# Retry specific job by ID
retry_job() {
    local job_id="$1"

    plugin_info "Retrying job: $job_id"

    # Get job details
    local job
    job=$(plugin_db_query "
        SELECT id, job_type, queue_name, status, retry_count, max_retries
        FROM jobs
        WHERE id = '$job_id';
    ")

    if [[ -z "$job" ]]; then
        plugin_error "Job not found: $job_id"
        return 1
    fi

    # Check if job can be retried
    local status
    status=$(echo "$job" | cut -d'|' -f4 | tr -d ' ')

    if [[ "$status" != "failed" ]]; then
        plugin_error "Job is not in 'failed' status (current: $status)"
        return 1
    fi

    # Reset job status
    plugin_db_query "
        UPDATE jobs
        SET
            status = 'waiting',
            updated_at = NOW()
        WHERE id = '$job_id';
    "

    # Re-queue in BullMQ
    if [[ -f "${PLUGIN_DIR}/ts/dist/cli.js" ]]; then
        node "${PLUGIN_DIR}/ts/dist/cli.js" retry --id "$job_id" 2>/dev/null || \
            plugin_warn "Failed to re-queue job in BullMQ"
    fi

    plugin_success "Job $job_id retried successfully"
}

# Show retryable jobs
show_retryable() {
    local queue_name="${1:-}"

    plugin_info "Retryable Jobs"
    printf "\n"

    local where_clause="status = 'failed' AND retry_count < max_retries"
    if [[ -n "$queue_name" ]]; then
        where_clause="$where_clause AND queue_name = '$queue_name'"
    fi

    plugin_db_query "
        SELECT
            j.id,
            j.job_type,
            j.queue_name,
            j.retry_count || '/' || j.max_retries AS retries,
            f.error_message,
            TO_CHAR(j.failed_at, 'YYYY-MM-DD HH24:MI:SS') AS failed_at
        FROM jobs j
        LEFT JOIN LATERAL (
            SELECT error_message
            FROM job_failures
            WHERE job_id = j.id
            ORDER BY failed_at DESC
            LIMIT 1
        ) f ON TRUE
        WHERE $where_clause
        ORDER BY j.failed_at DESC
        LIMIT 50;
    " | column -t -s '|'

    printf "\n"
}

# Show help
show_help() {
    printf "Usage: nself plugin jobs retry [OPTIONS]\n\n"
    printf "Retry failed jobs that haven't exceeded max retry attempts.\n\n"
    printf "Options:\n"
    printf "  -i, --id JOB_ID      Retry specific job by ID\n"
    printf "  -q, --queue QUEUE    Filter by queue name\n"
    printf "  -t, --type TYPE      Filter by job type\n"
    printf "  -l, --limit N        Limit number of jobs to retry (default: 10)\n"
    printf "  -s, --show           Show retryable jobs without retrying\n"
    printf "  -h, --help           Show this help\n\n"
    printf "Examples:\n"
    printf "  nself plugin jobs retry                      # Retry up to 10 failed jobs\n"
    printf "  nself plugin jobs retry -q default -l 20     # Retry 20 jobs from 'default' queue\n"
    printf "  nself plugin jobs retry -t send-email        # Retry failed email jobs\n"
    printf "  nself plugin jobs retry -i <uuid>            # Retry specific job\n"
    printf "  nself plugin jobs retry -s                   # Show retryable jobs\n"
}

# Parse arguments
JOB_ID=""
QUEUE=""
TYPE=""
LIMIT=10
SHOW_ONLY=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        -i|--id)
            JOB_ID="$2"
            shift 2
            ;;
        -q|--queue)
            QUEUE="$2"
            shift 2
            ;;
        -t|--type)
            TYPE="$2"
            shift 2
            ;;
        -l|--limit)
            LIMIT="$2"
            shift 2
            ;;
        -s|--show)
            SHOW_ONLY=true
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
if [[ "$SHOW_ONLY" == "true" ]]; then
    show_retryable "$QUEUE"
elif [[ -n "$JOB_ID" ]]; then
    retry_job "$JOB_ID"
else
    retry_failed "$QUEUE" "$TYPE" "$LIMIT"
fi
