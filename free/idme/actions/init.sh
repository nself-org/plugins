#!/bin/bash
# =============================================================================
# ID.me Init Action
# Initialize OAuth configuration and display authorization URL
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Configuration
# =============================================================================

IDME_CLIENT_ID="${IDME_CLIENT_ID:-}"
IDME_REDIRECT_URI="${IDME_REDIRECT_URI:-}"
IDME_SCOPES="${IDME_SCOPES:-openid,email,profile}"
IDME_SANDBOX="${IDME_SANDBOX:-false}"

# Determine base URL
if [[ "$IDME_SANDBOX" == "true" ]]; then
    IDME_BASE_URL="https://api.idmelabs.com"
else
    IDME_BASE_URL="https://api.id.me"
fi

# =============================================================================
# Functions
# =============================================================================

show_config() {
    plugin_info "ID.me OAuth Configuration"
    printf "\n"
    printf "Environment:\n"
    printf "  Mode: %s\n" "$([ "$IDME_SANDBOX" == "true" ] && echo "Sandbox" || echo "Production")"
    printf "  Base URL: %s\n" "$IDME_BASE_URL"
    printf "  Client ID: %s\n" "${IDME_CLIENT_ID:0:8}..."
    printf "  Redirect URI: %s\n" "$IDME_REDIRECT_URI"
    printf "  Scopes: %s\n" "$IDME_SCOPES"
    printf "\n"
}

generate_auth_url() {
    # Generate random state for CSRF protection
    local state
    state=$(openssl rand -hex 16)

    # URL encode scopes
    local encoded_scopes
    encoded_scopes=$(echo -n "$IDME_SCOPES" | sed 's/ /%20/g' | sed 's/,/%2C/g')

    # Build authorization URL
    local auth_url
    auth_url="${IDME_BASE_URL}/oauth/authorize"
    auth_url="${auth_url}?client_id=${IDME_CLIENT_ID}"
    auth_url="${auth_url}&redirect_uri=${IDME_REDIRECT_URI}"
    auth_url="${auth_url}&response_type=code"
    auth_url="${auth_url}&scope=${encoded_scopes}"
    auth_url="${auth_url}&state=${state}"

    printf "Authorization URL:\n\n"
    printf "%s\n\n" "$auth_url"
    printf "State (save for CSRF validation): %s\n\n" "$state"
}

show_groups() {
    plugin_info "Available Verification Groups"
    printf "\n"
    printf "You can request verification for the following groups by adding them to IDME_SCOPES:\n\n"
    printf "  military         - Active duty military personnel\n"
    printf "  veteran          - Military veterans\n"
    printf "  first_responder  - First responders (police, fire, EMT)\n"
    printf "  government       - Government employees\n"
    printf "  teacher          - Teachers and educators\n"
    printf "  student          - Students\n"
    printf "  nurse            - Nurses and healthcare workers\n"
    printf "\n"
    printf "Example: IDME_SCOPES=openid,email,profile,military,veteran\n"
    printf "\n"
}

# =============================================================================
# Main
# =============================================================================

main() {
    # Check required variables
    if [[ -z "$IDME_CLIENT_ID" ]] || [[ -z "$IDME_REDIRECT_URI" ]]; then
        plugin_error "Missing required environment variables"
        printf "\n"
        printf "Please set the following in your .env file:\n"
        printf "  IDME_CLIENT_ID=your_client_id\n"
        printf "  IDME_CLIENT_SECRET=your_client_secret\n"
        printf "  IDME_REDIRECT_URI=https://your-domain.com/callback/idme\n"
        printf "\n"
        return 1
    fi

    case "${1:-}" in
        -h|--help|help)
            printf "Usage: nself plugin idme init [command]\n\n"
            printf "Commands:\n"
            printf "  config    Show current configuration\n"
            printf "  auth      Generate authorization URL\n"
            printf "  groups    Show available verification groups\n"
            printf "\n"
            ;;
        config)
            show_config
            ;;
        auth)
            show_config
            generate_auth_url
            ;;
        groups)
            show_groups
            ;;
        *)
            show_config
            generate_auth_url
            show_groups
            ;;
    esac
}

main "$@"
