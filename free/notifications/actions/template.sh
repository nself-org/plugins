#!/bin/bash
# =============================================================================
# Notifications Template Action
# Manage notification templates
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Template Management
# =============================================================================

list_templates() {
    plugin_info "Notification Templates"
    printf "\n"

    local format="${1:-table}"

    if [[ "$format" == "json" ]]; then
        plugin_db_query "
            SELECT json_agg(
                json_build_object(
                    'id', id::text,
                    'name', name,
                    'category', category,
                    'channels', channels,
                    'active', active,
                    'created_at', created_at
                )
            )
            FROM notification_templates
            ORDER BY category, name;
        " 2>/dev/null
    else
        plugin_db_query "
            SELECT
                name,
                category,
                array_to_string(array(SELECT jsonb_array_elements_text(channels)), ', ') AS channels,
                active,
                to_char(created_at, 'YYYY-MM-DD') AS created
            FROM notification_templates
            ORDER BY category, name;
        " 2>/dev/null | column -t -s '|'
    fi
}

show_template() {
    local template_name="$1"

    plugin_info "Template: $template_name"
    printf "\n"

    # Get template details
    local template
    template=$(plugin_db_query "
        SELECT
            id::text,
            name,
            category,
            channels::text,
            subject,
            body_text,
            body_html,
            push_title,
            push_body,
            sms_body,
            variables::text,
            active,
            created_at,
            updated_at
        FROM notification_templates
        WHERE name = '$template_name';
    " 2>/dev/null)

    if [[ -z "$template" ]]; then
        plugin_error "Template not found: $template_name"
        return 1
    fi

    printf "%s\n" "$template"
    printf "\n"

    # Show usage stats
    plugin_info "Usage Statistics (last 30 days)"
    printf "\n"

    plugin_db_query "
        SELECT
            COUNT(*) AS total_sent,
            COUNT(*) FILTER (WHERE status = 'delivered') AS delivered,
            COUNT(*) FILTER (WHERE status = 'failed') AS failed,
            ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'delivered') / NULLIF(COUNT(*), 0), 2) AS delivery_rate
        FROM notifications
        WHERE template_name = '$template_name'
          AND created_at >= NOW() - INTERVAL '30 days';
    " 2>/dev/null
}

create_template() {
    plugin_info "Creating new template..."
    printf "\n"

    # Interactive creation
    read -p "Template name (e.g., welcome_email): " name
    read -p "Category (transactional/marketing/system/alert): " category
    read -p "Channels (comma-separated, e.g., email,push): " channels
    read -p "Subject (for email): " subject
    read -p "Body text: " body_text

    # Escape single quotes for SQL
    name=$(printf '%s' "$name" | sed "s/'/''/g")
    category=$(printf '%s' "$category" | sed "s/'/''/g")
    subject=$(printf '%s' "$subject" | sed "s/'/''/g")
    body_text=$(printf '%s' "$body_text" | sed "s/'/''/g")

    # Convert channels to JSON array
    local channels_json
    channels_json="[$(echo "$channels" | sed "s/,/\",\"/g" | sed 's/^/\"/' | sed 's/$/\"/')]"

    local result
    result=$(plugin_db_query "
        INSERT INTO notification_templates (name, category, channels, subject, body_text)
        VALUES ('$name', '$category', '$channels_json'::jsonb, '$subject', '$body_text')
        RETURNING id::text;
    " 2>&1)

    if [[ $? -eq 0 ]]; then
        plugin_success "Template created: $name"
        printf "ID: %s\n" "$result"
    else
        plugin_error "Failed to create template"
        printf "%s\n" "$result"
        return 1
    fi
}

update_template() {
    local template_name="$1"

    plugin_info "Updating template: $template_name"
    printf "\n"

    # Check exists
    local exists
    exists=$(plugin_db_query "SELECT COUNT(*) FROM notification_templates WHERE name = '$template_name';" 2>/dev/null | grep -o '[0-9]*' | head -1)

    if [[ "$exists" -eq 0 ]]; then
        plugin_error "Template not found: $template_name"
        return 1
    fi

    printf "Leave fields blank to keep current value\n\n"

    read -p "Subject: " subject
    read -p "Body text: " body_text
    read -p "Active (true/false): " active

    local updates=()

    if [[ -n "$subject" ]]; then
        subject=$(printf '%s' "$subject" | sed "s/'/''/g")
        updates+=("subject = '$subject'")
    fi

    if [[ -n "$body_text" ]]; then
        body_text=$(printf '%s' "$body_text" | sed "s/'/''/g")
        updates+=("body_text = '$body_text'")
    fi

    if [[ -n "$active" ]]; then
        updates+=("active = $active")
    fi

    if [[ ${#updates[@]} -eq 0 ]]; then
        plugin_warn "No updates provided"
        return 0
    fi

    local update_clause
    update_clause=$(IFS=,; echo "${updates[*]}")

    plugin_db_query "
        UPDATE notification_templates
        SET $update_clause
        WHERE name = '$template_name';
    " >/dev/null 2>&1

    if [[ $? -eq 0 ]]; then
        plugin_success "Template updated"
    else
        plugin_error "Update failed"
        return 1
    fi
}

delete_template() {
    local template_name="$1"

    plugin_warn "Delete template: $template_name"
    printf "\n"

    read -p "Are you sure? [y/N] " -n 1 -r
    printf "\n"

    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        plugin_info "Cancelled"
        return 0
    fi

    plugin_db_query "DELETE FROM notification_templates WHERE name = '$template_name';" >/dev/null 2>&1

    if [[ $? -eq 0 ]]; then
        plugin_success "Template deleted"
    else
        plugin_error "Delete failed"
        return 1
    fi
}

# =============================================================================
# Main
# =============================================================================

manage_templates() {
    local action="${1:-list}"
    shift || true

    case "$action" in
        list|ls)
            list_templates "$@"
            ;;
        show|get|view)
            if [[ $# -eq 0 ]]; then
                plugin_error "Template name required"
                return 1
            fi
            show_template "$1"
            ;;
        create|new|add)
            create_template
            ;;
        update|edit)
            if [[ $# -eq 0 ]]; then
                plugin_error "Template name required"
                return 1
            fi
            update_template "$1"
            ;;
        delete|remove|rm)
            if [[ $# -eq 0 ]]; then
                plugin_error "Template name required"
                return 1
            fi
            delete_template "$1"
            ;;
        *)
            show_help
            return 1
            ;;
    esac
}

# Show help
show_help() {
    printf "Usage: nself plugin notifications template <action> [args]\n\n"
    printf "Manage notification templates.\n\n"
    printf "Actions:\n"
    printf "  list [json]          List all templates (default: table format)\n"
    printf "  show <name>          Show template details and stats\n"
    printf "  create               Create new template (interactive)\n"
    printf "  update <name>        Update template (interactive)\n"
    printf "  delete <name>        Delete template\n\n"
    printf "Examples:\n"
    printf "  nself plugin notifications template list\n"
    printf "  nself plugin notifications template show welcome_email\n"
    printf "  nself plugin notifications template create\n"
}

# Parse arguments
if [[ $# -eq 0 ]]; then
    list_templates
    exit 0
fi

case "${1:-}" in
    -h|--help|help)
        show_help
        ;;
    *)
        manage_templates "$@"
        ;;
esac
