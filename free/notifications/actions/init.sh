#!/bin/bash
# =============================================================================
# Notifications Init Action
# Initialize notification system and verify setup
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Initialization
# =============================================================================

init_notifications() {
    plugin_info "Initializing Notifications system..."
    printf "\n"

    # Check database connection
    plugin_info "Checking database connection..."
    if ! plugin_db_query "SELECT 1;" >/dev/null 2>&1; then
        plugin_error "Database connection failed"
        return 1
    fi
    plugin_success "Database connected"

    # Check tables exist
    plugin_info "Verifying database schema..."
    local tables=(
        "notification_templates"
        "notification_preferences"
        "notifications"
        "notification_queue"
        "notification_providers"
        "notification_batches"
    )

    local missing=0
    for table in "${tables[@]}"; do
        if ! plugin_db_query "SELECT 1 FROM $table LIMIT 1;" >/dev/null 2>&1; then
            plugin_error "Table missing: $table"
            missing=1
        fi
    done

    if [[ $missing -eq 1 ]]; then
        plugin_error "Schema verification failed. Run install.sh first."
        return 1
    fi
    plugin_success "All tables present"

    # Check for configured providers
    plugin_info "Checking configured providers..."

    local enabled_count
    enabled_count=$(plugin_db_query "SELECT COUNT(*) FROM notification_providers WHERE enabled = true;" 2>/dev/null | grep -o '[0-9]*' | head -1)

    if [[ -z "$enabled_count" || "$enabled_count" -eq 0 ]]; then
        plugin_warn "No providers enabled yet"
        printf "\n"
        printf "Configure at least one provider:\n"
        printf "  Email: NOTIFICATIONS_EMAIL_PROVIDER=resend\n"
        printf "         NOTIFICATIONS_EMAIL_API_KEY=re_xxx\n"
        printf "         NOTIFICATIONS_EMAIL_FROM=noreply@example.com\n"
        printf "\n"
        printf "  Push:  NOTIFICATIONS_PUSH_PROVIDER=fcm\n"
        printf "         NOTIFICATIONS_PUSH_API_KEY=xxx\n"
        printf "\n"
        printf "  SMS:   NOTIFICATIONS_SMS_PROVIDER=twilio\n"
        printf "         NOTIFICATIONS_SMS_ACCOUNT_SID=xxx\n"
        printf "         NOTIFICATIONS_SMS_AUTH_TOKEN=xxx\n"
        printf "         NOTIFICATIONS_SMS_FROM=+1234567890\n"
        printf "\n"
    else
        plugin_success "$enabled_count provider(s) enabled"
    fi

    # Check templates
    local template_count
    template_count=$(plugin_db_query "SELECT COUNT(*) FROM notification_templates;" 2>/dev/null | grep -o '[0-9]*' | head -1)
    plugin_info "Templates available: $template_count"

    # Show statistics
    printf "\n"
    plugin_info "System Statistics:"
    printf "\n"

    local total_sent
    total_sent=$(plugin_db_query "SELECT COUNT(*) FROM notifications;" 2>/dev/null | grep -o '[0-9]*' | head -1)
    printf "  Total notifications sent: %s\n" "$total_sent"

    local queued
    queued=$(plugin_db_query "SELECT COUNT(*) FROM notification_queue WHERE status = 'pending';" 2>/dev/null | grep -o '[0-9]*' | head -1)
    printf "  Queued notifications: %s\n" "$queued"

    local failed
    failed=$(plugin_db_query "SELECT COUNT(*) FROM notifications WHERE status = 'failed';" 2>/dev/null | grep -o '[0-9]*' | head -1)
    printf "  Failed notifications: %s\n" "$failed"

    printf "\n"
    plugin_success "Initialization complete!"

    printf "\n"
    printf "Next commands:\n"
    printf "  nself plugin notifications template list   # List templates\n"
    printf "  nself plugin notifications test            # Send test notification\n"
    printf "  nself plugin notifications server          # Start API server\n"
    printf "  nself plugin notifications worker          # Start queue worker\n"
    printf "\n"
}

# Show help
show_help() {
    printf "Usage: nself plugin notifications init\n\n"
    printf "Initialize and verify notification system setup.\n\n"
    printf "Checks:\n"
    printf "  - Database connection\n"
    printf "  - Schema installation\n"
    printf "  - Provider configuration\n"
    printf "  - System statistics\n"
}

# Parse arguments
case "${1:-}" in
    -h|--help|help)
        show_help
        ;;
    *)
        init_notifications
        ;;
esac
