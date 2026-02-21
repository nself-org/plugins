#!/bin/bash
# =============================================================================
# Notifications Test Action
# Send test notifications to verify setup
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Test Functions
# =============================================================================

test_email() {
    local recipient="${1:-}"

    if [[ -z "$recipient" ]]; then
        plugin_error "Email recipient required"
        printf "Usage: nself plugin notifications test email user@example.com\n"
        return 1
    fi

    plugin_info "Sending test email to: $recipient"

    # Create test notification
    local result
    result=$(plugin_db_query "
        INSERT INTO notifications (
            user_id,
            channel,
            category,
            recipient_email,
            subject,
            body_text,
            body_html,
            status
        ) VALUES (
            uuid_generate_v4(),
            'email',
            'system',
            '$recipient',
            'Test Email from nself Notifications',
            'This is a test email to verify your notification system is working correctly.',
            '<h1>Test Email</h1><p>This is a test email to verify your notification system is working correctly.</p>',
            'pending'
        )
        RETURNING id;
    " 2>&1)

    if [[ $? -eq 0 ]]; then
        plugin_success "Test email queued"
        printf "\n"
        printf "Check the notification_queue table or run the worker to process:\n"
        printf "  nself plugin notifications worker\n"
        printf "\n"
    else
        plugin_error "Failed to queue test email"
        printf "%s\n" "$result"
        return 1
    fi
}

test_template() {
    local template_name="${1:-welcome_email}"
    local recipient="${2:-}"

    if [[ -z "$recipient" ]]; then
        plugin_error "Recipient required"
        printf "Usage: nself plugin notifications test template <template_name> <email>\n"
        return 1
    fi

    plugin_info "Sending test using template: $template_name"

    # Check template exists
    local template_exists
    template_exists=$(plugin_db_query "SELECT COUNT(*) FROM notification_templates WHERE name = '$template_name';" 2>/dev/null | grep -o '[0-9]*' | head -1)

    if [[ "$template_exists" -eq 0 ]]; then
        plugin_error "Template not found: $template_name"
        printf "\nAvailable templates:\n"
        plugin_db_query "SELECT name FROM notification_templates;" 2>/dev/null || true
        return 1
    fi

    # Create notification from template
    local result
    result=$(plugin_db_query "
        INSERT INTO notifications (
            user_id,
            template_name,
            channel,
            category,
            recipient_email,
            subject,
            body_text,
            body_html,
            status,
            metadata
        )
        SELECT
            uuid_generate_v4(),
            '$template_name',
            'email',
            category,
            '$recipient',
            REPLACE(subject, '{{app_name}}', 'nself'),
            REPLACE(REPLACE(body_text, '{{app_name}}', 'nself'), '{{user_name}}', 'Test User'),
            REPLACE(REPLACE(body_html, '{{app_name}}', 'nself'), '{{user_name}}', 'Test User'),
            'pending',
            '{\"test\": true}'::jsonb
        FROM notification_templates
        WHERE name = '$template_name'
        RETURNING id;
    " 2>&1)

    if [[ $? -eq 0 ]]; then
        plugin_success "Test notification created from template"
        printf "\n"
        printf "Run worker to process:\n"
        printf "  nself plugin notifications worker\n"
        printf "\n"
    else
        plugin_error "Failed to create notification"
        printf "%s\n" "$result"
        return 1
    fi
}

test_providers() {
    plugin_info "Testing provider connectivity..."
    printf "\n"

    # List enabled providers
    local providers
    providers=$(plugin_db_query "SELECT name, type, health_status FROM notification_providers WHERE enabled = true;" 2>/dev/null)

    if [[ -z "$providers" ]]; then
        plugin_warn "No providers enabled"
        printf "\nConfigure a provider first:\n"
        printf "  NOTIFICATIONS_EMAIL_PROVIDER=resend\n"
        printf "  NOTIFICATIONS_EMAIL_API_KEY=re_xxx\n"
        printf "\n"
        return 1
    fi

    printf "Enabled providers:\n"
    printf "%s\n" "$providers"
    printf "\n"

    plugin_success "Provider check complete"
}

# =============================================================================
# Main
# =============================================================================

run_tests() {
    local test_type="${1:-}"
    shift || true

    case "$test_type" in
        email)
            test_email "$@"
            ;;
        template)
            test_template "$@"
            ;;
        providers)
            test_providers
            ;;
        *)
            show_help
            return 1
            ;;
    esac
}

# Show help
show_help() {
    printf "Usage: nself plugin notifications test <type> [args]\n\n"
    printf "Send test notifications to verify system setup.\n\n"
    printf "Test types:\n"
    printf "  email <recipient>              Send basic test email\n"
    printf "  template <name> <recipient>    Test notification template\n"
    printf "  providers                      Check provider status\n\n"
    printf "Examples:\n"
    printf "  nself plugin notifications test email user@example.com\n"
    printf "  nself plugin notifications test template welcome_email user@example.com\n"
    printf "  nself plugin notifications test providers\n"
}

# Parse arguments
if [[ $# -eq 0 ]]; then
    show_help
    exit 1
fi

case "${1:-}" in
    -h|--help|help)
        show_help
        ;;
    *)
        run_tests "$@"
        ;;
esac
