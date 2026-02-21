#!/usr/bin/env bash
# GitHub Plugin - Pull Requests Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin github prs - Pull request management"
    echo ""
    echo "Usage: nself plugin github prs [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List pull requests"
    echo "  show <id>         Show PR details"
    echo "  open              List open PRs"
    echo "  merged            List merged PRs"
    echo "  stats             PR statistics"
    echo ""
    echo "Options:"
    echo "  --repo <name>     Filter by repository"
    echo "  --author <user>   Filter by author"
    echo "  --draft           Include drafts only"
    echo "  --limit <n>       Limit results (default: 50)"
    echo ""
}

list_prs() {
    local state=""
    local repo=""
    local author=""
    local draft_only=false
    local limit=50

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --state) state="$2"; shift 2 ;;
            --repo) repo="$2"; shift 2 ;;
            --author) author="$2"; shift 2 ;;
            --draft) draft_only=true; shift ;;
            --limit) limit="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local where_clause="WHERE 1=1"
    [[ -n "$state" ]] && where_clause+=" AND p.state = '$state'"
    [[ -n "$repo" ]] && where_clause+=" AND r.full_name = '$repo'"
    [[ -n "$author" ]] && where_clause+=" AND p.user_login = '$author'"
    [[ "$draft_only" == "true" ]] && where_clause+=" AND p.draft = true"

    printf "%-8s %-40s %-10s %-6s %-20s\n" "#" "Title" "State" "Merged" "Repo"
    printf "%-8s %-40s %-10s %-6s %-20s\n" "--------" "----------------------------------------" "----------" "------" "--------------------"

    plugin_db_query "SELECT p.number, SUBSTRING(p.title, 1, 40), p.state, p.merged, r.full_name
                     FROM github_pull_requests p
                     JOIN github_repositories r ON p.repo_id = r.id
                     $where_clause
                     ORDER BY p.updated_at DESC
                     LIMIT $limit" | while IFS='|' read -r num title state merged repo_name; do
        local merged_str="No"
        [[ "$merged" == "t" ]] && merged_str="Yes"
        printf "%-8s %-40s %-10s %-6s %-20s\n" "#$num" "$title" "$state" "$merged_str" "$repo_name"
    done
}

show_pr() {
    local pr_id="$1"

    if [[ -z "$pr_id" ]]; then
        plugin_log "error" "PR number or ID required"
        return 1
    fi

    plugin_db_query "SELECT
        'PR #' || p.number || ': ' || p.title,
        'Repository: ' || r.full_name,
        'State: ' || p.state || CASE WHEN p.draft THEN ' (Draft)' ELSE '' END,
        'Author: ' || p.user_login,
        'Branch: ' || p.head_ref || ' → ' || p.base_ref,
        'Merged: ' || CASE WHEN p.merged THEN 'Yes' ELSE 'No' END,
        'Changes: +' || p.additions || ' -' || p.deletions || ' (' || p.changed_files || ' files)',
        'Comments: ' || p.comments || ' | Reviews: ' || p.review_comments,
        'Created: ' || p.created_at,
        'URL: ' || p.html_url
    FROM github_pull_requests p
    JOIN github_repositories r ON p.repo_id = r.id
    WHERE p.id = $pr_id OR p.number = $pr_id
    LIMIT 1"
}

show_stats() {
    echo "Pull Request Statistics"
    echo "======================="
    echo ""

    echo "Summary:"
    plugin_db_query "SELECT
        'Total PRs: ' || COUNT(*),
        'Open: ' || COUNT(*) FILTER (WHERE state = 'open'),
        'Merged: ' || COUNT(*) FILTER (WHERE merged = true),
        'Closed (not merged): ' || COUNT(*) FILTER (WHERE state = 'closed' AND merged = false)
    FROM github_pull_requests"

    echo ""
    echo "Top Contributors (by PRs merged):"
    plugin_db_query "SELECT user_login, COUNT(*)
                     FROM github_pull_requests
                     WHERE merged = true
                     GROUP BY user_login
                     ORDER BY COUNT(*) DESC
                     LIMIT 10" | while IFS='|' read -r author count; do
        printf "  %-20s %s\n" "$author" "$count"
    done

    echo ""
    echo "Avg Changes per PR:"
    plugin_db_query "SELECT
        'Additions: ' || ROUND(AVG(additions)),
        'Deletions: ' || ROUND(AVG(deletions)),
        'Files Changed: ' || ROUND(AVG(changed_files))
    FROM github_pull_requests WHERE merged = true"
}

main() {
    local subcommand="${1:-list}"
    shift 2>/dev/null || true

    case "$subcommand" in
        list) list_prs "$@" ;;
        show) show_pr "$@" ;;
        open) list_prs --state open "$@" ;;
        merged) list_prs --state closed --merged true "$@" ;;
        stats) show_stats ;;
        -h|--help) show_help ;;
        *) show_help; return 1 ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
