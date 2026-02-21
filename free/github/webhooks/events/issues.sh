#!/usr/bin/env bash
# GitHub Webhook Handler - Issues Events

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

handle_issues() {
    local event_id="$1"
    local action="$2"
    local payload="$3"

    plugin_log "info" "Handling issues event: $action"

    local issue repo_id
    issue=$(echo "$payload" | jq '.issue')
    repo_id=$(echo "$payload" | jq -r '.repository.id')

    local issue_id number title state
    issue_id=$(echo "$issue" | jq -r '.id')
    number=$(echo "$issue" | jq -r '.number')
    title=$(echo "$issue" | jq -r '.title')
    state=$(echo "$issue" | jq -r '.state')

    plugin_log "debug" "Issue #$number: $title ($action)"

    # Upsert issue
    plugin_db_query "INSERT INTO github_issues (
        id, node_id, repo_id, number, title, body, state, state_reason,
        locked, user_login, user_id, labels, assignees, milestone,
        comments, html_url, closed_at, created_at, updated_at, synced_at
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
        $(echo "$issue" | jq '.html_url'),
        $(echo "$issue" | jq '.closed_at'),
        $(echo "$issue" | jq '.created_at'),
        $(echo "$issue" | jq '.updated_at'),
        NOW()
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

    case "$action" in
        opened)
            plugin_log "info" "New issue opened: #$number"
            ;;
        closed)
            plugin_log "info" "Issue closed: #$number"
            ;;
        reopened)
            plugin_log "info" "Issue reopened: #$number"
            ;;
    esac

    plugin_log "success" "Issues event processed"
    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && handle_issues "$@"
