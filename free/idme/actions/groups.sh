#!/bin/bash
# =============================================================================
# ID.me Groups Action
# List and manage verified groups
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Functions
# =============================================================================

list_all_groups() {
    plugin_info "All Verification Groups"
    printf "\n"

    plugin_db_query "
        SELECT * FROM idme_group_summary
    " || plugin_error "Failed to fetch groups"
}

list_user_groups() {
    local user_email="$1"

    plugin_info "Groups for: $user_email"
    printf "\n"

    plugin_db_query "
        SELECT
            g.group_type,
            g.group_name,
            g.verified,
            g.verified_at,
            g.affiliation,
            g.rank,
            g.status
        FROM idme_groups g
        JOIN idme_verifications v ON g.verification_id = v.id
        WHERE v.email = '$user_email'
        ORDER BY g.verified_at DESC
    " || plugin_error "Failed to fetch user groups"
}

list_group_type() {
    local group_type="$1"

    plugin_info "Users verified as: $group_type"
    printf "\n"

    plugin_db_query "
        SELECT
            v.email,
            v.first_name || ' ' || v.last_name as name,
            g.verified_at,
            g.affiliation,
            g.rank
        FROM idme_groups g
        JOIN idme_verifications v ON g.verification_id = v.id
        WHERE g.group_type = '$group_type' AND g.verified = TRUE
        ORDER BY g.verified_at DESC
    " || plugin_error "Failed to fetch group members"
}

show_group_types() {
    plugin_info "Available Group Types"
    printf "\n"
    printf "  military         - Active duty military personnel\n"
    printf "  veteran          - Military veterans\n"
    printf "  first_responder  - First responders (police, fire, EMT)\n"
    printf "  government       - Government employees\n"
    printf "  teacher          - Teachers and educators\n"
    printf "  student          - Students\n"
    printf "  nurse            - Nurses and healthcare workers\n"
    printf "\n"
}

# =============================================================================
# Main
# =============================================================================

main() {
    case "${1:-}" in
        -h|--help|help)
            printf "Usage: nself plugin idme groups [command] [args]\n\n"
            printf "Commands:\n"
            printf "  list           List all groups with counts\n"
            printf "  user <email>   List groups for a specific user\n"
            printf "  type <type>    List all users in a group type\n"
            printf "  types          Show available group types\n"
            printf "\n"
            ;;
        list)
            list_all_groups
            ;;
        user)
            if [[ -z "${2:-}" ]]; then
                plugin_error "Email required"
                printf "Usage: nself plugin idme groups user <email>\n"
                return 1
            fi
            list_user_groups "$2"
            ;;
        type)
            if [[ -z "${2:-}" ]]; then
                plugin_error "Group type required"
                printf "Usage: nself plugin idme groups type <type>\n\n"
                show_group_types
                return 1
            fi
            list_group_type "$2"
            ;;
        types)
            show_group_types
            ;;
        *)
            list_all_groups
            ;;
    esac
}

main "$@"
