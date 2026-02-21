#!/usr/bin/env bash
# GitHub Webhook Handler - Pull Request Events

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

handle_pull_request() {
    local event_id="$1"
    local action="$2"
    local payload="$3"

    plugin_log "info" "Handling pull_request event: $action"

    local pr repo_id
    pr=$(echo "$payload" | jq '.pull_request')
    repo_id=$(echo "$payload" | jq -r '.repository.id')

    local pr_id number title state
    pr_id=$(echo "$pr" | jq -r '.id')
    number=$(echo "$pr" | jq -r '.number')
    title=$(echo "$pr" | jq -r '.title')
    state=$(echo "$pr" | jq -r '.state')

    plugin_log "debug" "PR #$number: $title ($action)"

    # Upsert pull request
    plugin_db_query "INSERT INTO github_pull_requests (
        id, node_id, repo_id, number, title, body, state, draft, locked,
        user_login, user_id, head_ref, head_sha, base_ref, base_sha,
        merged, merged_at, merge_commit_sha, labels, assignees,
        html_url, created_at, updated_at, synced_at
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
        $(echo "$pr" | jq '.merged_at'),
        $(echo "$pr" | jq '.merge_commit_sha'),
        $(echo "$pr" | jq '.labels'),
        $(echo "$pr" | jq '.assignees'),
        $(echo "$pr" | jq '.html_url'),
        $(echo "$pr" | jq '.created_at'),
        $(echo "$pr" | jq '.updated_at'),
        NOW()
    ) ON CONFLICT (id) DO UPDATE SET
        title = EXCLUDED.title,
        body = EXCLUDED.body,
        state = EXCLUDED.state,
        draft = EXCLUDED.draft,
        merged = EXCLUDED.merged,
        merged_at = EXCLUDED.merged_at,
        labels = EXCLUDED.labels,
        assignees = EXCLUDED.assignees,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()"

    # Handle specific actions
    case "$action" in
        opened)
            plugin_log "info" "New PR opened: #$number"
            ;;
        closed)
            local merged
            merged=$(echo "$pr" | jq -r '.merged')
            if [[ "$merged" == "true" ]]; then
                plugin_log "info" "PR merged: #$number"
            else
                plugin_log "info" "PR closed without merge: #$number"
            fi
            ;;
        reopened)
            plugin_log "info" "PR reopened: #$number"
            ;;
        synchronize)
            plugin_log "debug" "PR updated with new commits: #$number"
            ;;
    esac

    plugin_log "success" "Pull request event processed"
    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && handle_pull_request "$@"
