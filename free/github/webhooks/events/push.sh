#!/usr/bin/env bash
# GitHub Webhook Handler - Push Events

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

handle_push() {
    local event_id="$1"
    local action="$2"
    local payload="$3"

    plugin_log "info" "Handling push event"

    # Extract push details
    local ref repo_id commits_count
    ref=$(echo "$payload" | jq -r '.ref')
    repo_id=$(echo "$payload" | jq -r '.repository.id')
    commits_count=$(echo "$payload" | jq '.commits | length')

    plugin_log "debug" "Push to $ref with $commits_count commits"

    # Store commits
    echo "$payload" | jq -c '.commits[]' 2>/dev/null | while read -r commit; do
        local sha message author_name author_email timestamp
        sha=$(echo "$commit" | jq -r '.id')
        message=$(echo "$commit" | jq -r '.message' | head -1)
        author_name=$(echo "$commit" | jq -r '.author.name')
        author_email=$(echo "$commit" | jq -r '.author.email')
        timestamp=$(echo "$commit" | jq -r '.timestamp')

        plugin_db_query "INSERT INTO github_commits (
            sha, repo_id, message, author_name, author_email, author_date, synced_at
        ) VALUES (
            '$sha',
            $repo_id,
            $(echo "$message" | jq -Rs .),
            $(echo "$author_name" | jq -Rs .),
            '$author_email',
            '$timestamp',
            NOW()
        ) ON CONFLICT (sha) DO UPDATE SET
            message = EXCLUDED.message,
            synced_at = NOW()"
    done

    # Update repository pushed_at
    local pushed_at
    pushed_at=$(echo "$payload" | jq -r '.head_commit.timestamp // empty')
    if [[ -n "$pushed_at" ]]; then
        plugin_db_query "UPDATE github_repositories SET pushed_at = '$pushed_at', synced_at = NOW() WHERE id = $repo_id"
    fi

    plugin_log "success" "Push event processed: $commits_count commits"
    return 0
}

# Run if executed directly
[[ "${BASH_SOURCE[0]}" == "${0}" ]] && handle_push "$@"
