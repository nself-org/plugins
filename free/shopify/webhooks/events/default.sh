#!/usr/bin/env bash
# Shopify Webhook Handler - Default Handler

set -e

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${PLUGIN_DIR}/../../shared/plugin-utils.sh"

handle_default() {
    local event_id="$1"
    local topic="$2"
    local payload="$3"

    plugin_log "debug" "Default handler for topic: $topic"
    return 0
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && handle_default "$@"
