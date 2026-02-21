#!/bin/bash
# =============================================================================
# Jobs Init Action
# Initialize jobs infrastructure (verify Redis, database, create queues)
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Initialization
# =============================================================================

init_jobs() {
    plugin_info "Initializing Jobs infrastructure..."

    # Check Redis connection
    plugin_info "Checking Redis connection..."
    if ! command -v redis-cli &> /dev/null; then
        plugin_error "redis-cli not found. Please install Redis."
        return 1
    fi

    local redis_url="${JOBS_REDIS_URL:-redis://localhost:6379}"
    local redis_host=$(echo "$redis_url" | sed -E 's|redis://([^:]+).*|\1|')
    local redis_port=$(echo "$redis_url" | sed -E 's|redis://[^:]+:([0-9]+).*|\1|')

    if ! redis-cli -h "${redis_host:-localhost}" -p "${redis_port:-6379}" ping &>/dev/null; then
        plugin_error "Cannot connect to Redis at ${redis_url}"
        return 1
    fi

    plugin_success "Redis connection OK"

    # Check database connection
    plugin_info "Checking database connection..."
    if ! plugin_db_query "SELECT 1;" &>/dev/null; then
        plugin_error "Cannot connect to database"
        return 1
    fi

    plugin_success "Database connection OK"

    # Verify tables exist
    plugin_info "Verifying database schema..."
    local tables=("jobs" "job_results" "job_failures" "job_schedules")
    for table in "${tables[@]}"; do
        if ! plugin_db_query "SELECT 1 FROM information_schema.tables WHERE table_name = '$table';" | grep -q "1"; then
            plugin_error "Table '$table' not found. Run 'nself plugin jobs install' first."
            return 1
        fi
    done

    plugin_success "Database schema OK"

    # Show queue configuration
    printf "\n"
    printf "Queue Configuration:\n"
    printf "  Redis URL: %s\n" "${JOBS_REDIS_URL}"
    printf "  Default concurrency: %s\n" "${JOBS_DEFAULT_CONCURRENCY:-5}"
    printf "  Max retry attempts: %s\n" "${JOBS_RETRY_ATTEMPTS:-3}"
    printf "  Retry delay: %s ms\n" "${JOBS_RETRY_DELAY:-5000}"
    printf "  Job timeout: %s ms\n" "${JOBS_JOB_TIMEOUT:-60000}"
    printf "\n"

    # Show queues
    plugin_info "Available queues:"
    printf "  - default (concurrency: %s)\n" "${JOBS_DEFAULT_CONCURRENCY:-5}"
    printf "  - high-priority (concurrency: 3)\n"
    printf "  - low-priority (concurrency: 10)\n"
    printf "\n"

    # Show stats
    plugin_info "Current statistics:"
    plugin_db_query "
        SELECT
            COALESCE(queue_name, 'TOTAL') AS queue,
            SUM(waiting) AS waiting,
            SUM(active) AS active,
            SUM(completed) AS completed,
            SUM(failed) AS failed
        FROM queue_stats
        GROUP BY ROLLUP(queue_name)
        ORDER BY queue_name NULLS LAST;
    " | column -t -s '|'

    printf "\n"
    plugin_success "Jobs infrastructure ready!"
    printf "\n"
    printf "Next steps:\n"
    printf "  - Start dashboard: nself plugin jobs server\n"
    printf "  - Start worker: nself plugin jobs worker\n"
    printf "  - View stats: nself plugin jobs stats\n"
    printf "\n"
}

# Show help
show_help() {
    printf "Usage: nself plugin jobs init\n\n"
    printf "Initialize jobs infrastructure and verify configuration.\n\n"
    printf "Checks:\n"
    printf "  - Redis connectivity\n"
    printf "  - Database connectivity\n"
    printf "  - Database schema\n"
    printf "  - Queue configuration\n\n"
    printf "Environment:\n"
    printf "  JOBS_REDIS_URL            Redis connection URL (required)\n"
    printf "  JOBS_DEFAULT_CONCURRENCY  Default queue concurrency (default: 5)\n"
    printf "  JOBS_RETRY_ATTEMPTS       Max retry attempts (default: 3)\n"
}

# Parse arguments
case "${1:-}" in
    -h|--help|help)
        show_help
        ;;
    *)
        init_jobs
        ;;
esac
