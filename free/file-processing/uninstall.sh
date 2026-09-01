#!/bin/bash
# =============================================================================
# File Processing Plugin Uninstaller
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"
source "${SHARED_DIR}/schema-sync.sh"

# =============================================================================
# Uninstallation
# =============================================================================

uninstall_file_processing_plugin() {
    plugin_info "Uninstalling File Processing plugin..."

    # Confirmation prompt
    plugin_warn "This will remove all file processing data, thumbnails, and scan results."
    printf "Are you sure? (y/N) "
    read -r REPLY
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        plugin_info "Uninstall cancelled."
        exit 0
    fi

    # Drop tables in reverse dependency order
    plugin_info "Removing database tables..."

    plugin_db_exec "DROP VIEW IF EXISTS thumbnail_generation_stats CASCADE;"
    plugin_db_exec "DROP VIEW IF EXISTS file_processing_stats CASCADE;"
    plugin_db_exec "DROP VIEW IF EXISTS file_security_alerts CASCADE;"
    plugin_db_exec "DROP VIEW IF EXISTS file_processing_failures CASCADE;"
    plugin_db_exec "DROP VIEW IF EXISTS file_processing_queue CASCADE;"

    plugin_db_exec "DROP TABLE IF EXISTS file_metadata CASCADE;"
    plugin_db_exec "DROP TABLE IF EXISTS file_scans CASCADE;"
    plugin_db_exec "DROP TABLE IF EXISTS file_thumbnails CASCADE;"
    plugin_db_exec "DROP TABLE IF EXISTS file_processing_jobs CASCADE;"

    plugin_db_exec "DROP FUNCTION IF EXISTS get_next_job(VARCHAR) CASCADE;"
    plugin_db_exec "DROP FUNCTION IF EXISTS cleanup_old_jobs(INTEGER) CASCADE;"
    plugin_db_exec "DROP FUNCTION IF EXISTS update_job_status() CASCADE;"

    # Remove migration records
    schema_remove_plugin_migrations "file-processing"

    # Clean up directories
    plugin_info "Cleaning up directories..."
    rm -rf "${HOME}/.nself/cache/plugins/file-processing"
    rm -rf "${HOME}/.nself/logs/plugins/file-processing"
    rm -rf "${HOME}/.nself/temp/plugins/file-processing"

    plugin_success "File Processing plugin uninstalled successfully!"
}

# Run uninstallation
uninstall_file_processing_plugin
