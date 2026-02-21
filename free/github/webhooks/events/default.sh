#!/usr/bin/env bash
# GitHub Webhook Handler - Default Handler
# Catches any events without specific handlers

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

handle_default() {
    local event_id="$1"
    local event_type="$2"
    local action="$3"
    local payload="$4"

    plugin_log "debug" "Default handler for event: $event_type ($action)"

    # Just log the event - no special processing
    # The event is already stored in github_webhook_events by the main handler

    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && handle_default "$@"
