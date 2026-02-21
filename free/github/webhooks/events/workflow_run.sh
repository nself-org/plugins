#!/usr/bin/env bash
# GitHub Webhook Handler - Workflow Run Events

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

handle_workflow_run() {
    local event_id="$1"
    local action="$2"
    local payload="$3"

    plugin_log "info" "Handling workflow_run event: $action"

    local run repo_id
    run=$(echo "$payload" | jq '.workflow_run')
    repo_id=$(echo "$payload" | jq -r '.repository.id')

    local run_id workflow_name conclusion
    run_id=$(echo "$run" | jq -r '.id')
    workflow_name=$(echo "$run" | jq -r '.name')
    conclusion=$(echo "$run" | jq -r '.conclusion // "in_progress"')

    plugin_log "debug" "Workflow: $workflow_name ($conclusion)"

    # Upsert workflow run
    plugin_db_query "INSERT INTO github_workflow_runs (
        id, node_id, repo_id, workflow_id, workflow_name, name,
        head_branch, head_sha, run_number, run_attempt, event,
        status, conclusion, actor_login, triggering_actor_login,
        html_url, jobs_url, logs_url, run_started_at, created_at, updated_at, synced_at
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
        $(echo "$run" | jq '.updated_at'),
        NOW()
    ) ON CONFLICT (id) DO UPDATE SET
        status = EXCLUDED.status,
        conclusion = EXCLUDED.conclusion,
        updated_at = EXCLUDED.updated_at,
        synced_at = NOW()"

    case "$action" in
        completed)
            case "$conclusion" in
                success)
                    plugin_log "success" "Workflow completed successfully: $workflow_name"
                    ;;
                failure)
                    plugin_log "warning" "Workflow failed: $workflow_name"
                    ;;
                cancelled)
                    plugin_log "info" "Workflow cancelled: $workflow_name"
                    ;;
            esac
            ;;
        requested)
            plugin_log "info" "Workflow requested: $workflow_name"
            ;;
    esac

    plugin_log "success" "Workflow run event processed"
    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && handle_workflow_run "$@"
