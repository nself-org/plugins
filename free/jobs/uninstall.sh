#!/bin/bash
# =============================================================================
# Jobs Plugin Uninstaller
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Uninstallation
# =============================================================================

uninstall_jobs_plugin() {
    plugin_info "Uninstalling Jobs plugin..."

    # Confirm uninstallation
    printf "\n"
    printf "This will:\n"
    printf "  - Drop all job tables (jobs, job_results, job_failures, job_schedules)\n"
    printf "  - Remove all job data from the database\n"
    printf "  - Remove cache and log files\n"
    printf "  - Clear Redis queues (if JOBS_CLEAR_REDIS=true)\n"
    printf "\n"
    printf "Continue? (yes/no): "
    read -r confirm
    printf "\n"

    if [[ "$confirm" != "yes" ]]; then
        plugin_warn "Uninstallation cancelled"
        return 0
    fi

    # Drop tables and types
    plugin_info "Dropping database tables..."

    plugin_db_query "
        DROP VIEW IF EXISTS scheduled_jobs_overview CASCADE;
        DROP VIEW IF EXISTS recent_failures CASCADE;
        DROP VIEW IF EXISTS job_type_stats CASCADE;
        DROP VIEW IF EXISTS queue_stats CASCADE;
        DROP VIEW IF EXISTS jobs_failed_details CASCADE;
        DROP VIEW IF EXISTS jobs_active CASCADE;

        DROP FUNCTION IF EXISTS get_job_stats(VARCHAR, INTEGER) CASCADE;
        DROP FUNCTION IF EXISTS cleanup_old_failed_jobs(INTEGER) CASCADE;
        DROP FUNCTION IF EXISTS cleanup_old_jobs(INTEGER) CASCADE;
        DROP FUNCTION IF EXISTS update_job_status() CASCADE;

        DROP TABLE IF EXISTS job_schedules CASCADE;
        DROP TABLE IF EXISTS job_failures CASCADE;
        DROP TABLE IF EXISTS job_results CASCADE;
        DROP TABLE IF EXISTS jobs CASCADE;

        DROP TYPE IF EXISTS job_priority CASCADE;
        DROP TYPE IF EXISTS job_status CASCADE;
    " || plugin_warn "Failed to drop some database objects"

    # Clear Redis queues if requested
    if [[ "${JOBS_CLEAR_REDIS:-false}" == "true" ]] && command -v redis-cli &> /dev/null; then
        plugin_info "Clearing Redis queues..."

        local redis_url="${JOBS_REDIS_URL:-redis://localhost:6379}"
        local redis_host=$(echo "$redis_url" | sed -E 's|redis://([^:]+).*|\1|')
        local redis_port=$(echo "$redis_url" | sed -E 's|redis://[^:]+:([0-9]+).*|\1|')

        # Clear BullMQ queues
        redis-cli -h "${redis_host:-localhost}" -p "${redis_port:-6379}" \
            --scan --pattern "bull:*" | xargs -r redis-cli -h "${redis_host:-localhost}" -p "${redis_port:-6379}" DEL \
            2>/dev/null || plugin_warn "Failed to clear Redis queues"
    fi

    # Remove cache and log directories
    plugin_info "Removing cache and log files..."
    rm -rf "${HOME}/.nself/cache/plugins/jobs"
    rm -rf "${HOME}/.nself/logs/plugins/jobs"

    # Remove migration records
    plugin_db_query "
        DELETE FROM plugin_migrations WHERE plugin_name = 'jobs';
    " 2>/dev/null || true

    plugin_success "Jobs plugin uninstalled successfully!"

    printf "\n"
    printf "Note: Redis data was %s\n" "${JOBS_CLEAR_REDIS:-false}"
    printf "To clear Redis manually, run:\n"
    printf "  redis-cli --scan --pattern 'bull:*' | xargs redis-cli DEL\n"
    printf "\n"
}

# Run uninstallation
uninstall_jobs_plugin
