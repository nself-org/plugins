#!/usr/bin/env bash
# GitHub Plugin - Issues Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin github issues - Issue management"
    echo ""
    echo "Usage: nself plugin github issues [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List issues"
    echo "  show <id>         Show issue details"
    echo "  open              List open issues"
    echo "  closed            List closed issues"
    echo "  stats             Issue statistics"
    echo ""
    echo "Options:"
    echo "  --repo <name>     Filter by repository"
    echo "  --author <user>   Filter by author"
    echo "  --label <name>    Filter by label"
    echo "  --limit <n>       Limit results (default: 50)"
    echo ""
}

list_issues() {
    local state="all"
    local repo=""
    local author=""
    local label=""
    local limit=50

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --state) state="$2"; shift 2 ;;
            --repo) repo="$2"; shift 2 ;;
            --author) author="$2"; shift 2 ;;
            --label) label="$2"; shift 2 ;;
            --limit) limit="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local where_clause="WHERE 1=1"
    [[ "$state" != "all" ]] && where_clause+=" AND i.state = '$state'"
    [[ -n "$repo" ]] && where_clause+=" AND r.full_name = '$repo'"
    [[ -n "$author" ]] && where_clause+=" AND i.user_login = '$author'"
    [[ -n "$label" ]] && where_clause+=" AND i.labels::text LIKE '%$label%'"

    printf "%-8s %-40s %-10s %-20s %-12s\n" "#" "Title" "State" "Repo" "Author"
    printf "%-8s %-40s %-10s %-20s %-12s\n" "--------" "----------------------------------------" "----------" "--------------------" "------------"

    plugin_db_query "SELECT i.number, SUBSTRING(i.title, 1, 40), i.state, r.full_name, i.user_login
                     FROM github_issues i
                     JOIN github_repositories r ON i.repo_id = r.id
                     $where_clause
                     ORDER BY i.updated_at DESC
                     LIMIT $limit" | while IFS='|' read -r num title state repo_name author; do
        printf "%-8s %-40s %-10s %-20s %-12s\n" "#$num" "$title" "$state" "$repo_name" "$author"
    done
}

show_issue() {
    local issue_id="$1"

    if [[ -z "$issue_id" ]]; then
        plugin_log "error" "Issue number or ID required"
        return 1
    fi

    plugin_db_query "SELECT
        'Issue #' || i.number || ': ' || i.title,
        'Repository: ' || r.full_name,
        'State: ' || i.state,
        'Author: ' || i.user_login,
        'Labels: ' || COALESCE(i.labels::text, '[]'),
        'Comments: ' || i.comments,
        'Created: ' || i.created_at,
        'Updated: ' || i.updated_at,
        'URL: ' || i.html_url
    FROM github_issues i
    JOIN github_repositories r ON i.repo_id = r.id
    WHERE i.id = $issue_id OR i.number = $issue_id
    LIMIT 1"
}

show_stats() {
    echo "Issue Statistics"
    echo "================"
    echo ""

    echo "By State:"
    plugin_db_query "SELECT state, COUNT(*) FROM github_issues GROUP BY state"

    echo ""
    echo "Open Issues by Repo (Top 10):"
    plugin_db_query "SELECT r.full_name, COUNT(*)
                     FROM github_issues i
                     JOIN github_repositories r ON i.repo_id = r.id
                     WHERE i.state = 'open'
                     GROUP BY r.full_name
                     ORDER BY COUNT(*) DESC
                     LIMIT 10" | while IFS='|' read -r repo count; do
        printf "  %-35s %s\n" "$repo" "$count"
    done

    echo ""
    echo "Top Issue Authors:"
    plugin_db_query "SELECT user_login, COUNT(*)
                     FROM github_issues
                     GROUP BY user_login
                     ORDER BY COUNT(*) DESC
                     LIMIT 10" | while IFS='|' read -r author count; do
        printf "  %-20s %s\n" "$author" "$count"
    done
}

main() {
    local subcommand="${1:-list}"
    shift 2>/dev/null || true

    case "$subcommand" in
        list) list_issues "$@" ;;
        show) show_issue "$@" ;;
        open) list_issues --state open "$@" ;;
        closed) list_issues --state closed "$@" ;;
        stats) show_stats ;;
        -h|--help) show_help ;;
        *) show_help; return 1 ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
