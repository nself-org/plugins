#!/bin/bash
# =============================================================================
# ID.me Verify Action
# Check verification status for a user
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Functions
# =============================================================================

verify_user() {
    local user_email="$1"

    plugin_info "Checking verification status for: $user_email"
    printf "\n"

    # Query verification status
    local result
    result=$(plugin_db_query "
        SELECT
            v.verified,
            v.verified_at,
            v.first_name,
            v.last_name,
            COUNT(g.id) as groups_count
        FROM idme_verifications v
        LEFT JOIN idme_groups g ON v.id = g.verification_id AND g.verified = TRUE
        WHERE v.email = '$user_email'
        GROUP BY v.id, v.verified, v.verified_at, v.first_name, v.last_name
    " || echo "")

    if [[ -z "$result" ]]; then
        plugin_warn "No verification found for: $user_email"
        return 1
    fi

    # Parse result (simplified - in production use proper JSON parsing)
    local verified=$(echo "$result" | grep -o 't\|f' | head -1)
    local groups_count=$(echo "$result" | grep -o '[0-9]*$')

    if [[ "$verified" == "t" ]]; then
        plugin_success "User is verified"
        printf "  Groups verified: %s\n" "$groups_count"
    else
        plugin_warn "User is not verified"
    fi

    # Show groups
    if [[ "$groups_count" -gt 0 ]]; then
        printf "\nVerified groups:\n"
        plugin_db_query "
            SELECT group_type, group_name, verified_at
            FROM idme_groups g
            JOIN idme_verifications v ON g.verification_id = v.id
            WHERE v.email = '$user_email' AND g.verified = TRUE
            ORDER BY g.verified_at DESC
        " || true
    fi
}

list_verifications() {
    local limit="${1:-10}"

    plugin_info "Recent verifications (last $limit)"
    printf "\n"

    plugin_db_query "
        SELECT
            v.email,
            v.first_name || ' ' || v.last_name as name,
            v.verified_at,
            COUNT(g.id) as groups
        FROM idme_verifications v
        LEFT JOIN idme_groups g ON v.id = g.verification_id AND g.verified = TRUE
        WHERE v.verified = TRUE
        GROUP BY v.id, v.email, v.first_name, v.last_name, v.verified_at
        ORDER BY v.verified_at DESC
        LIMIT $limit
    " || plugin_error "Failed to fetch verifications"
}

show_stats() {
    plugin_info "Verification Statistics"
    printf "\n"

    # Total verifications
    local total
    total=$(plugin_db_query "SELECT COUNT(*) FROM idme_verifications WHERE verified = TRUE" | tr -d '[:space:]')
    printf "Total verified users: %s\n" "$total"

    # Groups breakdown
    printf "\nGroups breakdown:\n"
    plugin_db_query "
        SELECT
            group_type,
            COUNT(*) as count
        FROM idme_groups
        WHERE verified = TRUE
        GROUP BY group_type
        ORDER BY count DESC
    " || true
}

# =============================================================================
# Main
# =============================================================================

main() {
    case "${1:-}" in
        -h|--help|help)
            printf "Usage: nself plugin idme verify [command] [args]\n\n"
            printf "Commands:\n"
            printf "  user <email>   Check verification status for a user\n"
            printf "  list [limit]   List recent verifications (default: 10)\n"
            printf "  stats          Show verification statistics\n"
            printf "\n"
            ;;
        user)
            if [[ -z "${2:-}" ]]; then
                plugin_error "Email required"
                printf "Usage: nself plugin idme verify user <email>\n"
                return 1
            fi
            verify_user "$2"
            ;;
        list)
            list_verifications "${2:-10}"
            ;;
        stats)
            show_stats
            ;;
        *)
            show_stats
            printf "\n"
            list_verifications 5
            ;;
    esac
}

main "$@"
