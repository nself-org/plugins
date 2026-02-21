#!/bin/bash
# =============================================================================
# nself Plugin Schema Sync
# Database schema management for nself plugins
# =============================================================================

set -euo pipefail

# Source plugin utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/plugin-utils.sh"

# =============================================================================
# Schema Management
# =============================================================================

# Apply plugin schema
schema_apply() {
    local plugin_name="$1"
    local plugin_dir="${PLUGIN_DIR}/${plugin_name}"
    local schema_dir="${plugin_dir}/schema"

    if [[ ! -d "$schema_dir" ]]; then
        plugin_warn "No schema directory found for $plugin_name"
        return 0
    fi

    plugin_info "Applying schema for $plugin_name..."

    # Apply tables.sql if exists
    if [[ -f "${schema_dir}/tables.sql" ]]; then
        plugin_debug "Applying tables.sql"
        plugin_db_exec_file "${schema_dir}/tables.sql"
    fi

    # Apply migrations in order
    if [[ -d "${schema_dir}/migrations" ]]; then
        for migration in "${schema_dir}/migrations"/*.sql; do
            [[ ! -f "$migration" ]] && continue

            local migration_name
            migration_name=$(basename "$migration" .sql)

            # Check if migration already applied
            if schema_migration_applied "$plugin_name" "$migration_name"; then
                plugin_debug "Migration already applied: $migration_name"
                continue
            fi

            plugin_info "Applying migration: $migration_name"
            plugin_db_exec_file "$migration"
            schema_record_migration "$plugin_name" "$migration_name"
        done
    fi

    plugin_success "Schema applied for $plugin_name"
}

# Rollback plugin schema
schema_rollback() {
    local plugin_name="$1"
    local steps="${2:-1}"
    local plugin_dir="${PLUGIN_DIR}/${plugin_name}"
    local schema_dir="${plugin_dir}/schema"

    if [[ ! -d "${schema_dir}/rollbacks" ]]; then
        plugin_error "No rollback scripts found for $plugin_name"
        return 1
    fi

    plugin_info "Rolling back $steps migration(s) for $plugin_name..."

    # Get applied migrations in reverse order
    local applied_migrations
    applied_migrations=$(plugin_db_query "SELECT migration_name FROM _nself_plugin_migrations WHERE plugin_name = '$plugin_name' ORDER BY applied_at DESC LIMIT $steps;")

    while IFS= read -r migration_name; do
        [[ -z "$migration_name" ]] && continue
        migration_name=$(echo "$migration_name" | xargs)

        local rollback_file="${schema_dir}/rollbacks/${migration_name}.sql"

        if [[ -f "$rollback_file" ]]; then
            plugin_info "Rolling back: $migration_name"
            plugin_db_exec_file "$rollback_file"
            schema_remove_migration "$plugin_name" "$migration_name"
        else
            plugin_warn "No rollback script for: $migration_name"
        fi
    done <<< "$applied_migrations"

    plugin_success "Rollback complete"
}

# Check migration status
schema_status() {
    local plugin_name="$1"
    local plugin_dir="${PLUGIN_DIR}/${plugin_name}"
    local schema_dir="${plugin_dir}/schema"

    printf "\n=== Schema Status: %s ===\n\n" "$plugin_name"

    # List applied migrations
    printf "Applied migrations:\n"
    plugin_db_query "SELECT migration_name, applied_at FROM _nself_plugin_migrations WHERE plugin_name = '$plugin_name' ORDER BY applied_at;"

    # List pending migrations
    if [[ -d "${schema_dir}/migrations" ]]; then
        printf "\nPending migrations:\n"
        for migration in "${schema_dir}/migrations"/*.sql; do
            [[ ! -f "$migration" ]] && continue

            local migration_name
            migration_name=$(basename "$migration" .sql)

            if ! schema_migration_applied "$plugin_name" "$migration_name"; then
                printf "  - %s\n" "$migration_name"
            fi
        done
    fi

    # List tables
    printf "\nPlugin tables:\n"
    plugin_db_query "SELECT table_name FROM information_schema.tables WHERE table_name LIKE '${plugin_name}_%' ORDER BY table_name;"
}

# =============================================================================
# Migration Tracking
# =============================================================================

# Ensure migrations table exists
schema_ensure_migrations_table() {
    plugin_db_exec "
        CREATE TABLE IF NOT EXISTS _nself_plugin_migrations (
            id SERIAL PRIMARY KEY,
            plugin_name VARCHAR(255) NOT NULL,
            migration_name VARCHAR(255) NOT NULL,
            applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(plugin_name, migration_name)
        );
    " 2>/dev/null || true
}

# Check if migration is applied
schema_migration_applied() {
    local plugin_name="$1"
    local migration_name="$2"

    schema_ensure_migrations_table

    local result
    result=$(plugin_db_query "SELECT COUNT(*) FROM _nself_plugin_migrations WHERE plugin_name = '$plugin_name' AND migration_name = '$migration_name';")
    [[ "$result" =~ [1-9] ]]
}

# Record applied migration
schema_record_migration() {
    local plugin_name="$1"
    local migration_name="$2"

    schema_ensure_migrations_table

    plugin_db_exec "INSERT INTO _nself_plugin_migrations (plugin_name, migration_name) VALUES ('$plugin_name', '$migration_name');"
}

# Remove migration record
schema_remove_migration() {
    local plugin_name="$1"
    local migration_name="$2"

    plugin_db_exec "DELETE FROM _nself_plugin_migrations WHERE plugin_name = '$plugin_name' AND migration_name = '$migration_name';"
}

# Remove all migration records for a plugin
schema_remove_plugin_migrations() {
    local plugin_name="$1"

    schema_ensure_migrations_table

    plugin_db_exec "DELETE FROM _nself_plugin_migrations WHERE plugin_name = '$plugin_name';"
    plugin_debug "Removed all migration records for $plugin_name"
}

# =============================================================================
# Schema Generation
# =============================================================================

# Generate schema from API response (for data sync plugins)
schema_generate_from_api() {
    local plugin_name="$1"
    local api_response="$2"
    local table_name="$3"

    # This is a placeholder - each plugin should implement its own logic
    plugin_warn "schema_generate_from_api should be implemented per plugin"
}

# =============================================================================
# Main
# =============================================================================

schema_main() {
    local action="${1:-}"
    local plugin_name="${2:-}"

    case "$action" in
        apply)
            schema_apply "$plugin_name"
            ;;
        rollback)
            schema_rollback "$plugin_name" "${3:-1}"
            ;;
        status)
            schema_status "$plugin_name"
            ;;
        *)
            printf "Usage: schema-sync.sh <action> <plugin_name> [args...]\n"
            printf "Actions: apply, rollback, status\n"
            return 1
            ;;
    esac
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    schema_main "$@"
fi
