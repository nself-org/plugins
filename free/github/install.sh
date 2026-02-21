#!/usr/bin/env bash
# GitHub Plugin - Installation Script

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_NAME="github"

# Source shared utilities
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

# ============================================================================
# Pre-flight Checks
# ============================================================================

preflight_checks() {
    plugin_log "info" "Running pre-flight checks..."

    # Check required environment variables
    if [[ -z "${GITHUB_TOKEN:-}" ]]; then
        plugin_log "error" "GITHUB_TOKEN is required"
        plugin_log "info" "Generate a token at: https://github.com/settings/tokens"
        plugin_log "info" "Required scopes: repo, read:org (for org repos)"
        return 1
    fi

    # Validate token format
    if [[ ! "$GITHUB_TOKEN" =~ ^(ghp_|github_pat_|gho_|ghu_|ghs_|ghr_) ]]; then
        plugin_log "warning" "Token format doesn't match known GitHub token patterns"
        plugin_log "info" "Expected: ghp_*, github_pat_*, gho_*, ghu_*, ghs_*, or ghr_*"
    fi

    # Test API access
    plugin_log "info" "Testing GitHub API access..."
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" \
        -H "Authorization: Bearer $GITHUB_TOKEN" \
        -H "Accept: application/vnd.github+json" \
        -H "X-GitHub-Api-Version: 2022-11-28" \
        "https://api.github.com/user")

    if [[ "$response" != "200" ]]; then
        plugin_log "error" "GitHub API returned status $response"
        plugin_log "info" "Check your GITHUB_TOKEN is valid and not expired"
        return 1
    fi

    plugin_log "success" "GitHub API access verified"
    return 0
}

# ============================================================================
# Database Setup
# ============================================================================

setup_database() {
    plugin_log "info" "Setting up database schema..."

    # Check if schema exists
    local tables_exist
    tables_exist=$(plugin_db_query "SELECT COUNT(*) FROM information_schema.tables WHERE table_name LIKE 'github_%'" 2>/dev/null || echo "0")

    if [[ "$tables_exist" -gt 0 ]]; then
        plugin_log "info" "GitHub tables already exist, checking for updates..."

        # Run migration checks here if needed
        return 0
    fi

    # Apply schema
    if [[ -f "${PLUGIN_DIR}/schema/tables.sql" ]]; then
        plugin_log "info" "Applying database schema..."
        plugin_db_exec_file "${PLUGIN_DIR}/schema/tables.sql"
        plugin_log "success" "Database schema applied"
    else
        plugin_log "error" "Schema file not found: ${PLUGIN_DIR}/schema/tables.sql"
        return 1
    fi

    return 0
}

# ============================================================================
# Webhook Setup
# ============================================================================

setup_webhooks() {
    plugin_log "info" "Configuring webhook handlers..."

    # Register webhook endpoint with nself
    local webhook_path="${GITHUB_WEBHOOK_PATH:-/webhooks/github}"

    plugin_log "info" "Webhook endpoint: $webhook_path"

    if [[ -n "${GITHUB_WEBHOOK_SECRET:-}" ]]; then
        plugin_log "success" "Webhook signature verification enabled"
    else
        plugin_log "warning" "GITHUB_WEBHOOK_SECRET not set - webhook signatures won't be verified"
        plugin_log "info" "Set GITHUB_WEBHOOK_SECRET for production use"
    fi

    # Copy webhook handlers to runtime location if needed
    local webhook_runtime_dir="${HOME}/.nself/plugins/${PLUGIN_NAME}/webhooks"
    mkdir -p "$webhook_runtime_dir"

    if [[ -d "${PLUGIN_DIR}/webhooks" ]]; then
        cp -r "${PLUGIN_DIR}/webhooks/"* "$webhook_runtime_dir/" 2>/dev/null || true
        chmod +x "$webhook_runtime_dir"/*.sh 2>/dev/null || true
        chmod +x "$webhook_runtime_dir/events/"*.sh 2>/dev/null || true
    fi

    return 0
}

# ============================================================================
# Initial Sync
# ============================================================================

initial_sync() {
    plugin_log "info" "Running initial data sync..."

    # Determine repositories to sync
    local repos_to_sync=""

    if [[ -n "${GITHUB_REPOS:-}" ]]; then
        # Specific repos configured
        repos_to_sync="$GITHUB_REPOS"
        plugin_log "info" "Syncing specified repos: $repos_to_sync"
    elif [[ -n "${GITHUB_ORG:-}" ]]; then
        # Sync org repos
        plugin_log "info" "Syncing repos for org: $GITHUB_ORG"
    else
        # Sync user repos
        plugin_log "info" "Syncing authenticated user's repos"
    fi

    # Run sync action
    if [[ -x "${PLUGIN_DIR}/actions/sync.sh" ]]; then
        bash "${PLUGIN_DIR}/actions/sync.sh" --initial
    else
        plugin_log "warning" "Sync action not found, skipping initial sync"
    fi

    return 0
}

# ============================================================================
# Main Installation
# ============================================================================

main() {
    plugin_log "info" "Installing GitHub plugin..."

    # Run pre-flight checks
    if ! preflight_checks; then
        plugin_log "error" "Pre-flight checks failed"
        return 1
    fi

    # Setup database
    if ! setup_database; then
        plugin_log "error" "Database setup failed"
        return 1
    fi

    # Setup webhooks
    if ! setup_webhooks; then
        plugin_log "error" "Webhook setup failed"
        return 1
    fi

    # Initial sync (optional, can fail)
    if [[ "${SKIP_INITIAL_SYNC:-}" != "true" ]]; then
        initial_sync || plugin_log "warning" "Initial sync had issues, continuing..."
    fi

    # Mark as installed
    plugin_mark_installed "$PLUGIN_NAME"

    plugin_log "success" "GitHub plugin installed successfully"

    # Show usage
    echo ""
    echo "Usage:"
    echo "  nself plugin github sync         - Sync repository data"
    echo "  nself plugin github repos        - List repositories"
    echo "  nself plugin github issues       - View issues"
    echo "  nself plugin github prs          - View pull requests"
    echo "  nself plugin github actions      - View workflow runs"
    echo ""

    return 0
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
