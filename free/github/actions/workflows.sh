#!/usr/bin/env bash
# GitHub Plugin - Workflow Runs Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin github actions - GitHub Actions workflow management"
    echo ""
    echo "Usage: nself plugin github actions [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List workflow runs"
    echo "  show <id>         Show run details"
    echo "  failed            List failed runs"
    echo "  stats             Workflow statistics"
    echo ""
    echo "Options:"
    echo "  --repo <name>     Filter by repository"
    echo "  --workflow <name> Filter by workflow name"
    echo "  --status <status> Filter by status"
    echo "  --limit <n>       Limit results (default: 50)"
    echo ""
}

list_runs() {
    local repo=""
    local workflow=""
    local status=""
    local limit=50

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --repo) repo="$2"; shift 2 ;;
            --workflow) workflow="$2"; shift 2 ;;
            --status) status="$2"; shift 2 ;;
            --limit) limit="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local where_clause="WHERE 1=1"
    [[ -n "$repo" ]] && where_clause+=" AND r.full_name = '$repo'"
    [[ -n "$workflow" ]] && where_clause+=" AND w.workflow_name LIKE '%$workflow%'"
    [[ -n "$status" ]] && where_clause+=" AND w.conclusion = '$status'"

    printf "%-10s %-25s %-10s %-12s %-20s\n" "Run#" "Workflow" "Status" "Conclusion" "Repo"
    printf "%-10s %-25s %-10s %-12s %-20s\n" "----------" "-------------------------" "----------" "------------" "--------------------"

    plugin_db_query "SELECT w.run_number, SUBSTRING(w.workflow_name, 1, 25), w.status, w.conclusion, r.full_name
                     FROM github_workflow_runs w
                     JOIN github_repositories r ON w.repo_id = r.id
                     $where_clause
                     ORDER BY w.created_at DESC
                     LIMIT $limit" | while IFS='|' read -r num name status conclusion repo_name; do
        local status_icon=""
        case "$conclusion" in
            success) status_icon="✓" ;;
            failure) status_icon="✗" ;;
            cancelled) status_icon="○" ;;
            *) status_icon="?" ;;
        esac
        printf "%-10s %-25s %-10s %-12s %-20s\n" "#$num" "$name" "$status" "$status_icon $conclusion" "$repo_name"
    done
}

show_run() {
    local run_id="$1"

    if [[ -z "$run_id" ]]; then
        plugin_log "error" "Run ID required"
        return 1
    fi

    plugin_db_query "SELECT
        'Run #' || w.run_number || ' - ' || w.workflow_name,
        'Repository: ' || r.full_name,
        'Status: ' || w.status,
        'Conclusion: ' || COALESCE(w.conclusion, 'in_progress'),
        'Event: ' || w.event,
        'Branch: ' || w.head_branch,
        'Actor: ' || w.actor_login,
        'Started: ' || w.run_started_at,
        'URL: ' || w.html_url
    FROM github_workflow_runs w
    JOIN github_repositories r ON w.repo_id = r.id
    WHERE w.id = $run_id OR w.run_number = $run_id
    LIMIT 1"
}

show_stats() {
    echo "Workflow Statistics (Last 30 Days)"
    echo "==================================="
    echo ""

    # Use view if exists, otherwise direct query
    plugin_db_query "SELECT repo, workflow_name, total_runs, success, failure, success_rate
                     FROM github_workflow_stats
                     ORDER BY total_runs DESC
                     LIMIT 15" 2>/dev/null || \
    plugin_db_query "SELECT r.full_name, w.workflow_name, COUNT(*),
                     COUNT(*) FILTER (WHERE w.conclusion = 'success'),
                     COUNT(*) FILTER (WHERE w.conclusion = 'failure'),
                     ROUND(COUNT(*) FILTER (WHERE w.conclusion = 'success')::numeric / NULLIF(COUNT(*), 0) * 100, 2)
                     FROM github_workflow_runs w
                     JOIN github_repositories r ON w.repo_id = r.id
                     WHERE w.created_at > NOW() - INTERVAL '30 days'
                     GROUP BY r.full_name, w.workflow_name
                     ORDER BY COUNT(*) DESC
                     LIMIT 15" | while IFS='|' read -r repo workflow total success failure rate; do
        printf "%-25s %-20s %5s runs (%s%% success)\n" "$repo" "$workflow" "$total" "$rate"
    done
}

list_failed() {
    list_runs --status failure "$@"
}

main() {
    local subcommand="${1:-list}"
    shift 2>/dev/null || true

    case "$subcommand" in
        list) list_runs "$@" ;;
        show) show_run "$@" ;;
        failed) list_failed "$@" ;;
        stats) show_stats ;;
        -h|--help) show_help ;;
        *) show_help; return 1 ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
