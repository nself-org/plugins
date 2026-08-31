#!/bin/bash
# =============================================================================
# File Processing Plugin Installer
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

# Source utilities
source "${SHARED_DIR}/plugin-utils.sh"
source "${SHARED_DIR}/schema-sync.sh"

# =============================================================================
# Installation
# =============================================================================

install_file_processing_plugin() {
    plugin_info "Installing File Processing plugin..."

    # Check required environment variables
    if ! plugin_check_env "file-processing" "FILE_STORAGE_PROVIDER" "FILE_STORAGE_BUCKET"; then
        plugin_warn "Required environment variables not set."
        plugin_info "Add to your .env:"
        plugin_info "  FILE_STORAGE_PROVIDER=minio|s3|gcs|r2|azure|b2"
        plugin_info "  FILE_STORAGE_BUCKET=your-bucket-name"
        plugin_info "  FILE_STORAGE_ENDPOINT=http://localhost:9000 (for MinIO/S3-compatible)"
        plugin_info "  FILE_STORAGE_ACCESS_KEY=your-access-key"
        plugin_info "  FILE_STORAGE_SECRET_KEY=your-secret-key"
    fi

    # Check for optional dependencies
    plugin_info "Checking dependencies..."

    # Check for Sharp (image processing)
    if command -v node >/dev/null 2>&1; then
        plugin_info "✓ Node.js found"
        if node -e "require('sharp')" 2>/dev/null; then
            plugin_info "✓ Sharp (image processing) available"
        else
            plugin_warn "Sharp not installed. Install with: npm install sharp"
        fi
    else
        plugin_warn "Node.js not found. Required for image processing."
    fi

    # Check for ffmpeg (video processing)
    if command -v ffmpeg >/dev/null 2>&1; then
        plugin_info "✓ ffmpeg found (video thumbnail support enabled)"
    else
        plugin_warn "ffmpeg not found. Video thumbnail generation will be disabled."
        plugin_info "Install: brew install ffmpeg (macOS) or apt-get install ffmpeg (Linux)"
    fi

    # Check for ClamAV (virus scanning)
    if [[ "${FILE_ENABLE_VIRUS_SCAN:-false}" == "true" ]]; then
        if command -v clamdscan >/dev/null 2>&1 || command -v clamd >/dev/null 2>&1; then
            plugin_info "✓ ClamAV found (virus scanning enabled)"
        else
            plugin_warn "ClamAV not found but virus scanning is enabled."
            plugin_info "Install: brew install clamav (macOS) or apt-get install clamav-daemon (Linux)"
        fi
    fi

    # Apply database schema
    plugin_info "Applying database schema..."

    # Ensure migrations table exists
    schema_ensure_migrations_table

    # Apply main schema
    if [[ -f "${PLUGIN_DIR}/schema/tables.sql" ]]; then
        plugin_db_exec_file "${PLUGIN_DIR}/schema/tables.sql"
    fi

    # Apply migrations
    if [[ -d "${PLUGIN_DIR}/schema/migrations" ]]; then
        for migration in "${PLUGIN_DIR}/schema/migrations"/*.sql; do
            [[ ! -f "$migration" ]] && continue

            local migration_name
            migration_name=$(basename "$migration" .sql)

            if ! schema_migration_applied "file-processing" "$migration_name"; then
                plugin_info "Applying migration: $migration_name"
                plugin_db_exec_file "$migration"
                schema_record_migration "file-processing" "$migration_name"
            fi
        done
    fi

    # Create directories
    mkdir -p "${HOME}/.nself/cache/plugins/file-processing"
    mkdir -p "${HOME}/.nself/logs/plugins/file-processing"
    mkdir -p "${HOME}/.nself/temp/plugins/file-processing"

    # Install TypeScript dependencies
    if [[ -d "${PLUGIN_DIR}/ts" ]]; then
        plugin_info "Installing TypeScript dependencies..."
        cd "${PLUGIN_DIR}/ts"
        npm install --silent 2>/dev/null || plugin_warn "Failed to install npm dependencies"
        npm run build --silent 2>/dev/null || plugin_warn "Failed to build TypeScript"
        cd - > /dev/null
    fi

    plugin_success "File Processing plugin installed successfully!"

    printf "\n"
    printf "Next steps:\n"
    printf "  1. Configure storage provider in .env\n"
    printf "  2. Run 'nself plugin file-processing server' to start the processing server\n"
    printf "  3. Run 'nself plugin file-processing worker' to start the background worker\n"
    printf "  4. Optional: Configure virus scanning with ClamAV\n"
    printf "\n"
    printf "Default thumbnail sizes: 100x100, 400x400, 1200x1200\n"
    printf "Customize with: FILE_THUMBNAIL_SIZES=100,400,1200\n"
    printf "\n"
}

# Run installation
install_file_processing_plugin
