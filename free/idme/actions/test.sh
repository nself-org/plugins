#!/bin/bash
# =============================================================================
# ID.me Test Action
# Test OAuth connection and API access
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Configuration
# =============================================================================

IDME_CLIENT_ID="${IDME_CLIENT_ID:-}"
IDME_CLIENT_SECRET="${IDME_CLIENT_SECRET:-}"
IDME_SANDBOX="${IDME_SANDBOX:-false}"

# Determine base URL
if [[ "$IDME_SANDBOX" == "true" ]]; then
    IDME_BASE_URL="https://api.idmelabs.com"
else
    IDME_BASE_URL="https://api.id.me"
fi

# =============================================================================
# Tests
# =============================================================================

test_config() {
    plugin_info "Testing configuration..."

    local errors=0

    # Check client ID
    if [[ -z "$IDME_CLIENT_ID" ]]; then
        plugin_error "IDME_CLIENT_ID not set"
        errors=$((errors + 1))
    else
        plugin_success "IDME_CLIENT_ID is set"
    fi

    # Check client secret
    if [[ -z "$IDME_CLIENT_SECRET" ]]; then
        plugin_error "IDME_CLIENT_SECRET not set"
        errors=$((errors + 1))
    else
        plugin_success "IDME_CLIENT_SECRET is set"
    fi

    # Check redirect URI
    if [[ -z "${IDME_REDIRECT_URI:-}" ]]; then
        plugin_warn "IDME_REDIRECT_URI not set"
    else
        plugin_success "IDME_REDIRECT_URI is set"
    fi

    printf "\n"

    if [[ $errors -gt 0 ]]; then
        plugin_error "Configuration test failed"
        return 1
    fi

    plugin_success "Configuration test passed"
    return 0
}

test_database() {
    plugin_info "Testing database connection..."

    # Check if tables exist
    local tables=("idme_verifications" "idme_groups" "idme_badges" "idme_attributes")
    local errors=0

    for table in "${tables[@]}"; do
        local exists
        exists=$(plugin_db_query "
            SELECT EXISTS (
                SELECT FROM information_schema.tables
                WHERE table_name = '$table'
            )
        " | grep -o 't\|f' || echo "f")

        if [[ "$exists" == "t" ]]; then
            plugin_success "Table $table exists"
        else
            plugin_error "Table $table does not exist"
            errors=$((errors + 1))
        fi
    done

    printf "\n"

    if [[ $errors -gt 0 ]]; then
        plugin_error "Database test failed"
        return 1
    fi

    plugin_success "Database test passed"
    return 0
}

test_api() {
    plugin_info "Testing API connectivity..."

    # Try to reach the ID.me API
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" "${IDME_BASE_URL}/" || echo "000")

    if [[ "$response" == "000" ]]; then
        plugin_error "Cannot reach ID.me API at: $IDME_BASE_URL"
        return 1
    fi

    plugin_success "API is reachable (HTTP $response)"
    printf "  Base URL: %s\n" "$IDME_BASE_URL"
    printf "  Mode: %s\n" "$([ "$IDME_SANDBOX" == "true" ] && echo "Sandbox" || echo "Production")"
    printf "\n"

    plugin_success "API connectivity test passed"
    return 0
}

run_all_tests() {
    plugin_info "Running all tests..."
    printf "\n"

    local failed=0

    test_config || failed=$((failed + 1))
    printf "\n"

    test_database || failed=$((failed + 1))
    printf "\n"

    test_api || failed=$((failed + 1))
    printf "\n"

    if [[ $failed -eq 0 ]]; then
        plugin_success "All tests passed!"
        return 0
    else
        plugin_error "$failed test(s) failed"
        return 1
    fi
}

# =============================================================================
# Main
# =============================================================================

main() {
    case "${1:-}" in
        -h|--help|help)
            printf "Usage: nself plugin idme test [command]\n\n"
            printf "Commands:\n"
            printf "  config     Test configuration\n"
            printf "  database   Test database setup\n"
            printf "  api        Test API connectivity\n"
            printf "  all        Run all tests (default)\n"
            printf "\n"
            ;;
        config)
            test_config
            ;;
        database)
            test_database
            ;;
        api)
            test_api
            ;;
        all|*)
            run_all_tests
            ;;
    esac
}

main "$@"
