#!/usr/bin/env bash
# GitHub Plugin - Uninstall Script

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME="github"

# Source shared utilities
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

# ============================================================================
# Uninstallation
# ============================================================================

remove_database() {
    local keep_data="${1:-false}"

    if [[ "$keep_data" == "true" ]]; then
        plugin_log "info" "Keeping database tables (--keep-data specified)"
        return 0
    fi

    plugin_log "warning" "Removing GitHub database tables..."

    # Drop views first (they depend on tables)
    plugin_db_query "DROP VIEW IF EXISTS github_workflow_stats CASCADE" 2>/dev/null || true
    plugin_db_query "DROP VIEW IF EXISTS github_recent_activity CASCADE" 2>/dev/null || true
    plugin_db_query "DROP VIEW IF EXISTS github_open_items CASCADE" 2>/dev/null || true

    # Drop tables in reverse dependency order
    plugin_db_query "DROP TABLE IF EXISTS github_webhook_events CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS github_deployments CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS github_workflow_runs CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS github_releases CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS github_commits CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS github_pull_requests CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS github_issues CASCADE" 2>/dev/null || true
    plugin_db_query "DROP TABLE IF EXISTS github_repositories CASCADE" 2>/dev/null || true

    plugin_log "success" "Database tables removed"
    return 0
}

remove_webhooks() {
    plugin_log "info" "Removing webhook handlers..."

    local webhook_runtime_dir="${HOME}/.nself/plugins/${PLUGIN_NAME}/webhooks"

    if [[ -d "$webhook_runtime_dir" ]]; then
        rm -rf "$webhook_runtime_dir"
        plugin_log "success" "Webhook handlers removed"
    fi

    return 0
}

remove_cache() {
    plugin_log "info" "Clearing plugin cache..."

    local cache_dir="${HOME}/.nself/cache/plugins/${PLUGIN_NAME}"

    if [[ -d "$cache_dir" ]]; then
        rm -rf "$cache_dir"
        plugin_log "success" "Cache cleared"
    fi

    return 0
}

main() {
    local keep_data=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --keep-data)
                keep_data=true
                shift
                ;;
            *)
                shift
                ;;
        esac
    done

    plugin_log "info" "Uninstalling GitHub plugin..."

    # Remove database (unless --keep-data)
    remove_database "$keep_data"

    # Remove webhooks
    remove_webhooks

    # Remove cache
    remove_cache

    # Mark as uninstalled
    plugin_mark_uninstalled "$PLUGIN_NAME"

    plugin_log "success" "GitHub plugin uninstalled"

    if [[ "$keep_data" == "true" ]]; then
        echo ""
        echo "Note: Database tables were preserved. To remove them later:"
        echo "  nself db query \"DROP TABLE IF EXISTS github_* CASCADE\""
    fi

    return 0
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
