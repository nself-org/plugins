#!/usr/bin/env bash
# GitHub Plugin - Sync Action
# Syncs repository data from GitHub API

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLUGIN_NAME="github"

# Source shared utilities
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

# ============================================================================
# Configuration
# ============================================================================

GITHUB_API="https://api.github.com"
API_VERSION="2022-11-28"
PER_PAGE=100

# ============================================================================
# API Helpers
# ============================================================================

github_api() {
    local endpoint="$1"
    local method="${2:-GET}"

    curl -s \
        -X "$method" \
        -H "Authorization: Bearer $GITHUB_TOKEN" \
        -H "Accept: application/vnd.github+json" \
        -H "X-GitHub-Api-Version: $API_VERSION" \
        "${GITHUB_API}${endpoint}"
}

github_api_paginated() {
    local endpoint="$1"
    local page=1
    local results="[]"

    while true; do
        local response
        response=$(github_api "${endpoint}?per_page=${PER_PAGE}&page=${page}")

        # Check if empty or error
        if [[ -z "$response" ]] || [[ "$response" == "[]" ]]; then
            break
        fi

        # Check for error response
        if echo "$response" | grep -q '"message"'; then
            plugin_log "error" "API error: $(echo "$response" | grep -o '"message":"[^"]*"')"
            break
        fi

        # Merge results
        results=$(echo "$results $response" | jq -s 'add')

        # Check if we got less than a full page
        local count
        count=$(echo "$response" | jq 'length')
        if [[ "$count" -lt "$PER_PAGE" ]]; then
            break
        fi

        ((page++))

        # Rate limit protection
        sleep 0.1
    done

    echo "$results"
}

# ============================================================================
# Sync Functions
# ============================================================================

sync_repositories() {
    plugin_log "info" "Syncing repositories..."

    local repos

    if [[ -n "${GITHUB_ORG:-}" ]]; then
        repos=$(github_api_paginated "/orgs/${GITHUB_ORG}/repos")
    elif [[ -n "${GITHUB_REPOS:-}" ]]; then
        # Sync specific repos
        repos="[]"
        IFS=',' read -ra REPO_LIST <<< "$GITHUB_REPOS"
        for repo in "${REPO_LIST[@]}"; do
            repo=$(echo "$repo" | xargs)  # Trim whitespace
            local repo_data
            repo_data=$(github_api "/repos/${repo}")
            repos=$(echo "$repos [$repo_data]" | jq -s 'add')
        done
    else
        repos=$(github_api_paginated "/user/repos")
    fi

    local count
    count=$(echo "$repos" | jq 'length')
    plugin_log "info" "Found $count repositories"

    # Insert/update repos
    echo "$repos" | jq -c '.[]' | while read -r repo; do
        local id name full_name owner_login
        id=$(echo "$repo" | jq -r '.id')
        name=$(echo "$repo" | jq -r '.name')
        full_name=$(echo "$repo" | jq -r '.full_name')
        owner_login=$(echo "$repo" | jq -r '.owner.login')

        plugin_log "debug" "Syncing repo: $full_name"

        # Upsert repository
        plugin_db_query "INSERT INTO github_repositories (
            id, node_id, name, full_name, owner_login, owner_type,
            private, description, fork, url, html_url, clone_url, ssh_url,
            homepage, language, default_branch, size, stargazers_count,
            watchers_count, forks_count, open_issues_count, topics,
            visibility, archived, disabled, pushed_at, created_at, updated_at
        ) VALUES (
            $(echo "$repo" | jq '.id'),
            $(echo "$repo" | jq '.node_id'),
            $(echo "$repo" | jq '.name'),
            $(echo "$repo" | jq '.full_name'),
            $(echo "$repo" | jq '.owner.login'),
            $(echo "$repo" | jq '.owner.type'),
            $(echo "$repo" | jq '.private'),
            $(echo "$repo" | jq '.description'),
            $(echo "$repo" | jq '.fork'),
            $(echo "$repo" | jq '.url'),
            $(echo "$repo" | jq '.html_url'),
            $(echo "$repo" | jq '.clone_url'),
            $(echo "$repo" | jq '.ssh_url'),
            $(echo "$repo" | jq '.homepage'),
            $(echo "$repo" | jq '.language'),
            $(echo "$repo" | jq '.default_branch'),
            $(echo "$repo" | jq '.size'),
            $(echo "$repo" | jq '.stargazers_count'),
            $(echo "$repo" | jq '.watchers_count'),
            $(echo "$repo" | jq '.forks_count'),
            $(echo "$repo" | jq '.open_issues_count'),
            $(echo "$repo" | jq '.topics'),
            $(echo "$repo" | jq '.visibility'),
            $(echo "$repo" | jq '.archived'),
            $(echo "$repo" | jq '.disabled'),
            $(echo "$repo" | jq '.pushed_at'),
            $(echo "$repo" | jq '.created_at'),
            $(echo "$repo" | jq '.updated_at')
        ) ON CONFLICT (id) DO UPDATE SET
            name = EXCLUDED.name,
            full_name = EXCLUDED.full_name,
            description = EXCLUDED.description,
            language = EXCLUDED.language,
            stargazers_count = EXCLUDED.stargazers_count,
            watchers_count = EXCLUDED.watchers_count,
            forks_count = EXCLUDED.forks_count,
            open_issues_count = EXCLUDED.open_issues_count,
            topics = EXCLUDED.topics,
            visibility = EXCLUDED.visibility,
            archived = EXCLUDED.archived,
            pushed_at = EXCLUDED.pushed_at,
            updated_at = EXCLUDED.updated_at,
            synced_at = NOW()"
    done

    plugin_log "success" "Repositories synced"
}

sync_issues() {
    local repo_id="$1"
    local full_name="$2"

    plugin_log "info" "Syncing issues for $full_name..."

    local issues
    issues=$(github_api_paginated "/repos/${full_name}/issues?state=all")

    local count
    count=$(echo "$issues" | jq '[.[] | select(.pull_request == null)] | length')
    plugin_log "debug" "Found $count issues"

    # Filter out PRs (they come in issues endpoint)
    echo "$issues" | jq -c '.[] | select(.pull_request == null)' | while read -r issue; do
        local id number title state
        id=$(echo "$issue" | jq -r '.id')
        number=$(echo "$issue" | jq -r '.number')

        plugin_db_query "INSERT INTO github_issues (
            id, node_id, repo_id, number, title, body, state, state_reason,
            locked, user_login, user_id, labels, assignees, milestone,
            comments, reactions, html_url, closed_at, closed_by_login,
            created_at, updated_at
        ) VALUES (
            $(echo "$issue" | jq '.id'),
            $(echo "$issue" | jq '.node_id'),
            $repo_id,
            $(echo "$issue" | jq '.number'),
            $(echo "$issue" | jq '.title'),
            $(echo "$issue" | jq '.body'),
            $(echo "$issue" | jq '.state'),
            $(echo "$issue" | jq '.state_reason'),
            $(echo "$issue" | jq '.locked'),
            $(echo "$issue" | jq '.user.login'),
            $(echo "$issue" | jq '.user.id'),
            $(echo "$issue" | jq '.labels'),
            $(echo "$issue" | jq '.assignees'),
            $(echo "$issue" | jq '.milestone'),
            $(echo "$issue" | jq '.comments'),
            $(echo "$issue" | jq '.reactions'),
            $(echo "$issue" | jq '.html_url'),
            $(echo "$issue" | jq '.closed_at'),
            $(echo "$issue" | jq '.closed_by.login // null'),
            $(echo "$issue" | jq '.created_at'),
            $(echo "$issue" | jq '.updated_at')
        ) ON CONFLICT (id) DO UPDATE SET
            title = EXCLUDED.title,
            body = EXCLUDED.body,
            state = EXCLUDED.state,
            state_reason = EXCLUDED.state_reason,
            labels = EXCLUDED.labels,
            assignees = EXCLUDED.assignees,
            comments = EXCLUDED.comments,
            closed_at = EXCLUDED.closed_at,
            updated_at = EXCLUDED.updated_at,
            synced_at = NOW()"
    done
}

sync_pull_requests() {
    local repo_id="$1"
    local full_name="$2"

    plugin_log "info" "Syncing pull requests for $full_name..."

    local prs
    prs=$(github_api_paginated "/repos/${full_name}/pulls?state=all")

    local count
    count=$(echo "$prs" | jq 'length')
    plugin_log "debug" "Found $count pull requests"

    echo "$prs" | jq -c '.[]' | while read -r pr; do
        plugin_db_query "INSERT INTO github_pull_requests (
            id, node_id, repo_id, number, title, body, state, draft, locked,
            user_login, user_id, head_ref, head_sha, base_ref, base_sha,
            merged, mergeable, mergeable_state, merged_by_login, merged_at,
            merge_commit_sha, labels, assignees, reviewers, milestone,
            comments, review_comments, commits, additions, deletions,
            changed_files, html_url, diff_url, closed_at, created_at, updated_at
        ) VALUES (
            $(echo "$pr" | jq '.id'),
            $(echo "$pr" | jq '.node_id'),
            $repo_id,
            $(echo "$pr" | jq '.number'),
            $(echo "$pr" | jq '.title'),
            $(echo "$pr" | jq '.body'),
            $(echo "$pr" | jq '.state'),
            $(echo "$pr" | jq '.draft'),
            $(echo "$pr" | jq '.locked'),
            $(echo "$pr" | jq '.user.login'),
            $(echo "$pr" | jq '.user.id'),
            $(echo "$pr" | jq '.head.ref'),
            $(echo "$pr" | jq '.head.sha'),
            $(echo "$pr" | jq '.base.ref'),
            $(echo "$pr" | jq '.base.sha'),
            $(echo "$pr" | jq '.merged'),
            $(echo "$pr" | jq '.mergeable'),
            $(echo "$pr" | jq '.mergeable_state'),
            $(echo "$pr" | jq '.merged_by.login // null'),
            $(echo "$pr" | jq '.merged_at'),
            $(echo "$pr" | jq '.merge_commit_sha'),
            $(echo "$pr" | jq '.labels'),
            $(echo "$pr" | jq '.assignees'),
            $(echo "$pr" | jq '[.requested_reviewers[].login]'),
            $(echo "$pr" | jq '.milestone'),
            $(echo "$pr" | jq '.comments // 0'),
            $(echo "$pr" | jq '.review_comments // 0'),
            $(echo "$pr" | jq '.commits // 0'),
            $(echo "$pr" | jq '.additions // 0'),
            $(echo "$pr" | jq '.deletions // 0'),
            $(echo "$pr" | jq '.changed_files // 0'),
            $(echo "$pr" | jq '.html_url'),
            $(echo "$pr" | jq '.diff_url'),
            $(echo "$pr" | jq '.closed_at'),
            $(echo "$pr" | jq '.created_at'),
            $(echo "$pr" | jq '.updated_at')
        ) ON CONFLICT (id) DO UPDATE SET
            title = EXCLUDED.title,
            body = EXCLUDED.body,
            state = EXCLUDED.state,
            draft = EXCLUDED.draft,
            merged = EXCLUDED.merged,
            merged_at = EXCLUDED.merged_at,
            labels = EXCLUDED.labels,
            assignees = EXCLUDED.assignees,
            closed_at = EXCLUDED.closed_at,
            updated_at = EXCLUDED.updated_at,
            synced_at = NOW()"
    done
}

sync_workflow_runs() {
    local repo_id="$1"
    local full_name="$2"

    plugin_log "info" "Syncing workflow runs for $full_name..."

    local runs
    runs=$(github_api "/repos/${full_name}/actions/runs?per_page=50")

    if echo "$runs" | jq -e '.workflow_runs' > /dev/null 2>&1; then
        echo "$runs" | jq -c '.workflow_runs[]' | while read -r run; do
            plugin_db_query "INSERT INTO github_workflow_runs (
                id, node_id, repo_id, workflow_id, workflow_name, name,
                head_branch, head_sha, run_number, run_attempt, event,
                status, conclusion, actor_login, triggering_actor_login,
                html_url, jobs_url, logs_url, run_started_at, created_at, updated_at
            ) VALUES (
                $(echo "$run" | jq '.id'),
                $(echo "$run" | jq '.node_id'),
                $repo_id,
                $(echo "$run" | jq '.workflow_id'),
                $(echo "$run" | jq '.name'),
                $(echo "$run" | jq '.display_title'),
                $(echo "$run" | jq '.head_branch'),
                $(echo "$run" | jq '.head_sha'),
                $(echo "$run" | jq '.run_number'),
                $(echo "$run" | jq '.run_attempt'),
                $(echo "$run" | jq '.event'),
                $(echo "$run" | jq '.status'),
                $(echo "$run" | jq '.conclusion'),
                $(echo "$run" | jq '.actor.login'),
                $(echo "$run" | jq '.triggering_actor.login'),
                $(echo "$run" | jq '.html_url'),
                $(echo "$run" | jq '.jobs_url'),
                $(echo "$run" | jq '.logs_url'),
                $(echo "$run" | jq '.run_started_at'),
                $(echo "$run" | jq '.created_at'),
                $(echo "$run" | jq '.updated_at')
            ) ON CONFLICT (id) DO UPDATE SET
                status = EXCLUDED.status,
                conclusion = EXCLUDED.conclusion,
                updated_at = EXCLUDED.updated_at,
                synced_at = NOW()"
        done
    fi
}

# ============================================================================
# Main Sync
# ============================================================================

main() {
    local initial=false
    local full=false
    local repos_only=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --initial)
                initial=true
                shift
                ;;
            --full)
                full=true
                shift
                ;;
            --repos-only)
                repos_only=true
                shift
                ;;
            *)
                shift
                ;;
        esac
    done

    plugin_log "info" "Starting GitHub sync..."

    # Check token
    if [[ -z "${GITHUB_TOKEN:-}" ]]; then
        plugin_log "error" "GITHUB_TOKEN not set"
        return 1
    fi

    # Sync repositories first
    sync_repositories

    if [[ "$repos_only" == "true" ]]; then
        plugin_log "success" "Sync complete (repos only)"
        return 0
    fi

    # Sync details for each repository
    plugin_db_query "SELECT id, full_name FROM github_repositories" | while read -r line; do
        local repo_id full_name
        repo_id=$(echo "$line" | cut -d'|' -f1 | xargs)
        full_name=$(echo "$line" | cut -d'|' -f2 | xargs)

        if [[ -n "$repo_id" ]] && [[ -n "$full_name" ]]; then
            sync_issues "$repo_id" "$full_name"
            sync_pull_requests "$repo_id" "$full_name"
            sync_workflow_runs "$repo_id" "$full_name"

            # Rate limit protection
            sleep 0.5
        fi
    done

    # Update sync timestamp
    plugin_set_meta "$PLUGIN_NAME" "last_sync" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

    plugin_log "success" "GitHub sync complete"
    return 0
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
