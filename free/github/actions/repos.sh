#!/usr/bin/env bash
# GitHub Plugin - Repository Management Action

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

show_help() {
    echo "nself plugin github repos - Repository management"
    echo ""
    echo "Usage: nself plugin github repos [subcommand] [options]"
    echo ""
    echo "Subcommands:"
    echo "  list              List synced repositories"
    echo "  show <repo>       Show repository details"
    echo "  stats             Show repository statistics"
    echo "  sync              Sync repository list"
    echo ""
    echo "Options:"
    echo "  --org <name>      Filter by organization"
    echo "  --language <lang> Filter by language"
    echo "  --archived        Include archived repos"
    echo "  --format <fmt>    Output format (table, json, csv)"
    echo ""
    echo "Examples:"
    echo "  nself plugin github repos list"
    echo "  nself plugin github repos show owner/repo"
    echo "  nself plugin github repos stats --org myorg"
}

list_repos() {
    local org=""
    local language=""
    local include_archived=false
    local format="table"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --org) org="$2"; shift 2 ;;
            --language) language="$2"; shift 2 ;;
            --archived) include_archived=true; shift ;;
            --format) format="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local where_clause="WHERE 1=1"
    [[ -n "$org" ]] && where_clause+=" AND owner_login = '$org'"
    [[ -n "$language" ]] && where_clause+=" AND language = '$language'"
    [[ "$include_archived" != "true" ]] && where_clause+=" AND archived = false"

    local query="SELECT full_name, language, stargazers_count, forks_count, open_issues_count, updated_at
                 FROM github_repositories
                 $where_clause
                 ORDER BY stargazers_count DESC"

    if [[ "$format" == "json" ]]; then
        plugin_db_query_json "$query"
    else
        printf "%-40s %-12s %6s %6s %6s %-20s\n" "Repository" "Language" "Stars" "Forks" "Issues" "Updated"
        printf "%-40s %-12s %6s %6s %6s %-20s\n" "----------------------------------------" "------------" "------" "------" "------" "--------------------"
        plugin_db_query "$query" | while IFS='|' read -r name lang stars forks issues updated; do
            printf "%-40s %-12s %6s %6s %6s %-20s\n" "$name" "${lang:-n/a}" "$stars" "$forks" "$issues" "${updated:0:10}"
        done
    fi
}

show_repo() {
    local repo_name="$1"

    if [[ -z "$repo_name" ]]; then
        plugin_log "error" "Repository name required"
        return 1
    fi

    local data
    data=$(plugin_db_query "SELECT * FROM github_repositories WHERE full_name = '$repo_name'" 2>/dev/null)

    if [[ -z "$data" ]]; then
        plugin_log "error" "Repository not found: $repo_name"
        return 1
    fi

    echo "Repository: $repo_name"
    echo "========================"
    plugin_db_query "SELECT
        'Owner: ' || owner_login,
        'Language: ' || COALESCE(language, 'n/a'),
        'Stars: ' || stargazers_count,
        'Forks: ' || forks_count,
        'Open Issues: ' || open_issues_count,
        'Default Branch: ' || default_branch,
        'Visibility: ' || visibility,
        'Archived: ' || archived,
        'Created: ' || created_at,
        'Updated: ' || updated_at
    FROM github_repositories WHERE full_name = '$repo_name'"
}

show_stats() {
    echo "Repository Statistics"
    echo "====================="
    echo ""

    echo "By Language:"
    plugin_db_query "SELECT COALESCE(language, 'Unknown') as lang, COUNT(*) as count
                     FROM github_repositories
                     WHERE archived = false
                     GROUP BY language
                     ORDER BY count DESC
                     LIMIT 10" | while IFS='|' read -r lang count; do
        printf "  %-20s %s\n" "$lang" "$count"
    done

    echo ""
    echo "By Owner:"
    plugin_db_query "SELECT owner_login, COUNT(*) as count
                     FROM github_repositories
                     GROUP BY owner_login
                     ORDER BY count DESC
                     LIMIT 10" | while IFS='|' read -r owner count; do
        printf "  %-20s %s\n" "$owner" "$count"
    done

    echo ""
    echo "Totals:"
    plugin_db_query "SELECT
        'Total Repos: ' || COUNT(*),
        'Total Stars: ' || SUM(stargazers_count),
        'Total Forks: ' || SUM(forks_count),
        'Total Issues: ' || SUM(open_issues_count)
    FROM github_repositories WHERE archived = false"
}

main() {
    local subcommand="${1:-list}"
    shift 2>/dev/null || true

    case "$subcommand" in
        list) list_repos "$@" ;;
        show) show_repo "$@" ;;
        stats) show_stats "$@" ;;
        sync) bash "${PLUGIN_DIR}/actions/sync.sh" --repos-only ;;
        -h|--help) show_help ;;
        *) show_help; return 1 ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
