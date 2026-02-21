#!/bin/bash
# =============================================================================
# Jobs Schedule Action
# Manage scheduled (cron) jobs
# =============================================================================

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

# =============================================================================
# Scheduled Jobs Management
# =============================================================================

# List scheduled jobs
list_schedules() {
    plugin_info "Scheduled Jobs"
    printf "\n"

    plugin_db_query "
        SELECT
            name,
            job_type,
            cron_expression,
            CASE WHEN enabled THEN 'Yes' ELSE 'No' END AS enabled,
            total_runs,
            COALESCE(success_rate, 0) AS success_rate,
            TO_CHAR(next_run_at, 'YYYY-MM-DD HH24:MI:SS') AS next_run,
            TO_CHAR(last_run_at, 'YYYY-MM-DD HH24:MI:SS') AS last_run
        FROM scheduled_jobs_overview
        ORDER BY next_run_at;
    " | column -t -s '|'

    printf "\n"
}

# Show schedule details
show_schedule() {
    local name="$1"

    plugin_info "Schedule Details: $name"
    printf "\n"

    local details
    details=$(plugin_db_query "
        SELECT
            id,
            name,
            description,
            job_type,
            queue_name,
            cron_expression,
            timezone,
            enabled,
            total_runs,
            successful_runs,
            failed_runs,
            max_runs,
            TO_CHAR(last_run_at, 'YYYY-MM-DD HH24:MI:SS') AS last_run,
            TO_CHAR(next_run_at, 'YYYY-MM-DD HH24:MI:SS') AS next_run,
            TO_CHAR(end_date, 'YYYY-MM-DD HH24:MI:SS') AS end_date,
            TO_CHAR(created_at, 'YYYY-MM-DD HH24:MI:SS') AS created
        FROM job_schedules
        WHERE name = '$name';
    ")

    if [[ -z "$details" ]]; then
        plugin_error "Schedule not found: $name"
        return 1
    fi

    echo "$details" | tr '|' '\n' | awk '{print "  " $0}'

    printf "\n"

    # Show payload
    plugin_info "Payload:"
    plugin_db_query "SELECT payload FROM job_schedules WHERE name = '$name';" | \
        python3 -m json.tool 2>/dev/null || cat

    printf "\n"

    # Show recent runs
    plugin_info "Recent Runs (last 10):"
    plugin_db_query "
        SELECT
            TO_CHAR(j.created_at, 'YYYY-MM-DD HH24:MI:SS') AS created,
            j.status,
            ROUND(EXTRACT(EPOCH FROM (j.completed_at - j.started_at)), 2) AS duration_sec,
            CASE WHEN j.status = 'failed' THEN f.error_message ELSE '' END AS error
        FROM jobs j
        LEFT JOIN job_schedules s ON j.id = s.last_job_id OR j.metadata->>'schedule_name' = s.name
        LEFT JOIN LATERAL (
            SELECT error_message
            FROM job_failures
            WHERE job_id = j.id
            ORDER BY failed_at DESC
            LIMIT 1
        ) f ON TRUE
        WHERE s.name = '$name'
        ORDER BY j.created_at DESC
        LIMIT 10;
    " | column -t -s '|'

    printf "\n"
}

# Enable schedule
enable_schedule() {
    local name="$1"

    plugin_db_query "
        UPDATE job_schedules
        SET enabled = TRUE, updated_at = NOW()
        WHERE name = '$name';
    "

    plugin_success "Schedule '$name' enabled"
}

# Disable schedule
disable_schedule() {
    local name="$1"

    plugin_db_query "
        UPDATE job_schedules
        SET enabled = FALSE, updated_at = NOW()
        WHERE name = '$name';
    "

    plugin_success "Schedule '$name' disabled"
}

# Delete schedule
delete_schedule() {
    local name="$1"

    read -p "Delete schedule '$name'? (yes/no): " confirm
    if [[ "$confirm" != "yes" ]]; then
        plugin_warn "Delete cancelled"
        return 0
    fi

    plugin_db_query "DELETE FROM job_schedules WHERE name = '$name';"

    plugin_success "Schedule '$name' deleted"
}

# Create schedule
create_schedule() {
    local name="$1"
    local job_type="$2"
    local cron="$3"
    local payload="${4:-{}}"
    local queue="${5:-default}"
    local description="${6:-}"

    # Validate cron expression (basic check)
    if ! echo "$cron" | grep -qE '^[0-9*/,-]+ [0-9*/,-]+ [0-9*/,-]+ [0-9*/,-]+ [0-9*/,-]+'; then
        plugin_error "Invalid cron expression: $cron"
        return 1
    fi

    plugin_db_query "
        INSERT INTO job_schedules (name, description, job_type, queue_name, cron_expression, payload)
        VALUES (
            '$name',
            $([ -n "$description" ] && echo "'$description'" || echo "NULL"),
            '$job_type',
            '$queue',
            '$cron',
            '$payload'::JSONB
        );
    "

    plugin_success "Schedule '$name' created"
}

# Show help
show_help() {
    printf "Usage: nself plugin jobs schedule [COMMAND] [OPTIONS]\n\n"
    printf "Manage scheduled (cron) jobs.\n\n"
    printf "Commands:\n"
    printf "  list                 List all schedules (default)\n"
    printf "  show NAME            Show schedule details\n"
    printf "  create               Create a new schedule\n"
    printf "  enable NAME          Enable a schedule\n"
    printf "  disable NAME         Disable a schedule\n"
    printf "  delete NAME          Delete a schedule\n\n"
    printf "Create Options:\n"
    printf "  -n, --name NAME      Schedule name (required)\n"
    printf "  -t, --type TYPE      Job type (required)\n"
    printf "  -c, --cron EXPR      Cron expression (required)\n"
    printf "  -p, --payload JSON   Job payload (default: {})\n"
    printf "  -q, --queue QUEUE    Queue name (default: default)\n"
    printf "  -d, --desc TEXT      Description\n\n"
    printf "Examples:\n"
    printf "  nself plugin jobs schedule list\n"
    printf "  nself plugin jobs schedule show cleanup-jobs\n"
    printf "  nself plugin jobs schedule create \\\\\n"
    printf "    -n daily-backup \\\\\n"
    printf "    -t database-backup \\\\\n"
    printf "    -c '0 2 * * *' \\\\\n"
    printf "    -p '{\"database\": \"production\"}' \\\\\n"
    printf "    -d 'Daily production database backup'\n"
    printf "  nself plugin jobs schedule enable daily-backup\n"
    printf "  nself plugin jobs schedule disable daily-backup\n"
    printf "  nself plugin jobs schedule delete daily-backup\n"
}

# Parse command
COMMAND="${1:-list}"
shift || true

case "$COMMAND" in
    list)
        list_schedules
        ;;
    show)
        if [[ -z "${1:-}" ]]; then
            plugin_error "Schedule name required"
            show_help
            exit 1
        fi
        show_schedule "$1"
        ;;
    enable)
        if [[ -z "${1:-}" ]]; then
            plugin_error "Schedule name required"
            exit 1
        fi
        enable_schedule "$1"
        ;;
    disable)
        if [[ -z "${1:-}" ]]; then
            plugin_error "Schedule name required"
            exit 1
        fi
        disable_schedule "$1"
        ;;
    delete)
        if [[ -z "${1:-}" ]]; then
            plugin_error "Schedule name required"
            exit 1
        fi
        delete_schedule "$1"
        ;;
    create)
        # Parse create options
        NAME=""
        TYPE=""
        CRON=""
        PAYLOAD="{}"
        QUEUE="default"
        DESC=""

        while [[ $# -gt 0 ]]; do
            case "$1" in
                -n|--name)
                    NAME="$2"
                    shift 2
                    ;;
                -t|--type)
                    TYPE="$2"
                    shift 2
                    ;;
                -c|--cron)
                    CRON="$2"
                    shift 2
                    ;;
                -p|--payload)
                    PAYLOAD="$2"
                    shift 2
                    ;;
                -q|--queue)
                    QUEUE="$2"
                    shift 2
                    ;;
                -d|--desc)
                    DESC="$2"
                    shift 2
                    ;;
                *)
                    plugin_error "Unknown option: $1"
                    show_help
                    exit 1
                    ;;
            esac
        done

        if [[ -z "$NAME" ]] || [[ -z "$TYPE" ]] || [[ -z "$CRON" ]]; then
            plugin_error "Name, type, and cron expression are required"
            show_help
            exit 1
        fi

        create_schedule "$NAME" "$TYPE" "$CRON" "$PAYLOAD" "$QUEUE" "$DESC"
        ;;
    -h|--help|help)
        show_help
        ;;
    *)
        plugin_error "Unknown command: $COMMAND"
        show_help
        exit 1
        ;;
esac
