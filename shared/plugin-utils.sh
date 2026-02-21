#!/bin/bash
# =============================================================================
# nself Plugin Utilities
# Shared functions for all nself plugins
# =============================================================================

# Ensure we have the required nself functions
if ! command -v nself >/dev/null 2>&1; then
    printf "\033[31mError: nself is not installed or not in PATH\033[0m\n" >&2
    exit 1
fi

# =============================================================================
# Configuration
# =============================================================================

PLUGIN_DIR="${PLUGIN_DIR:-$HOME/.nself/plugins}"
PLUGIN_CACHE_DIR="${PLUGIN_CACHE_DIR:-$HOME/.nself/cache/plugins}"
PLUGIN_LOG_DIR="${PLUGIN_LOG_DIR:-$HOME/.nself/logs/plugins}"

# =============================================================================
# Logging Functions
# =============================================================================

plugin_log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    case "$level" in
        debug)
            [[ "${NSELF_DEBUG:-false}" == "true" ]] && printf "[%s] DEBUG: %s\n" "$timestamp" "$message"
            ;;
        info)
            printf "[%s] INFO: %s\n" "$timestamp" "$message"
            ;;
        warn)
            printf "\033[33m[%s] WARN: %s\033[0m\n" "$timestamp" "$message"
            ;;
        error)
            printf "\033[31m[%s] ERROR: %s\033[0m\n" "$timestamp" "$message" >&2
            ;;
        success)
            printf "\033[32m[%s] %s\033[0m\n" "$timestamp" "$message"
            ;;
    esac
}

plugin_debug() { plugin_log debug "$@"; }
plugin_info() { plugin_log info "$@"; }
plugin_warn() { plugin_log warn "$@"; }
plugin_error() { plugin_log error "$@"; }
plugin_success() { plugin_log success "$@"; }

# =============================================================================
# Environment Functions
# =============================================================================

# Load plugin environment variables from .env
plugin_load_env() {
    local plugin_name="$1"
    local env_file="${NSELF_PROJECT_DIR:-.}/.env"

    if [[ -f "$env_file" ]]; then
        # Source only the plugin-specific variables
        # Use tr for Bash 3.2 compatibility (no ${var^^})
        local plugin_upper
        plugin_upper=$(printf '%s' "$plugin_name" | tr '[:lower:]' '[:upper:]')

        while IFS='=' read -r key value; do
            # Skip comments and empty lines
            [[ -z "$key" || "$key" =~ ^# ]] && continue
            # Export the variable
            export "$key=$value"
        done < <(grep -E "^${plugin_upper}_" "$env_file" 2>/dev/null || true)
    fi
}

# Check if required environment variables are set
plugin_check_env() {
    local plugin_name="$1"
    shift
    local required_vars=("$@")
    local missing=()

    for var in "${required_vars[@]}"; do
        if [[ -z "${!var:-}" ]]; then
            missing+=("$var")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        plugin_error "Missing required environment variables:"
        for var in "${missing[@]}"; do
            printf "  - %s\n" "$var" >&2
        done
        return 1
    fi

    return 0
}

# =============================================================================
# Database Functions
# =============================================================================

# Get database connection string
plugin_get_db_url() {
    local db_host="${POSTGRES_HOST:-localhost}"
    local db_port="${POSTGRES_PORT:-5432}"
    local db_name="${POSTGRES_DB:-nself}"
    local db_user="${POSTGRES_USER:-postgres}"
    local db_pass="${POSTGRES_PASSWORD:-}"

    printf "postgresql://%s:%s@%s:%s/%s" "$db_user" "$db_pass" "$db_host" "$db_port" "$db_name"
}

# Execute SQL query
plugin_db_query() {
    local query="$1"
    local db_url
    db_url=$(plugin_get_db_url)

    if command -v psql >/dev/null 2>&1; then
        psql "$db_url" -t -c "$query" 2>/dev/null
    elif command -v docker >/dev/null 2>&1; then
        docker exec -i "${PROJECT_NAME:-nself}_postgres" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself}" -t -c "$query" 2>/dev/null
    else
        plugin_error "No PostgreSQL client available"
        return 1
    fi
}

# Execute SQL file
plugin_db_exec_file() {
    local file="$1"

    if [[ ! -f "$file" ]]; then
        plugin_error "SQL file not found: $file"
        return 1
    fi

    if command -v psql >/dev/null 2>&1; then
        psql "$(plugin_get_db_url)" -f "$file" 2>/dev/null
    elif command -v docker >/dev/null 2>&1; then
        docker exec -i "${PROJECT_NAME:-nself}_postgres" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself}" < "$file" 2>/dev/null
    else
        plugin_error "No PostgreSQL client available"
        return 1
    fi
}

# Execute SQL statement without returning results
plugin_db_exec() {
    local query="$1"
    local db_url
    db_url=$(plugin_get_db_url)

    if command -v psql >/dev/null 2>&1; then
        psql "$db_url" -c "$query" >/dev/null 2>&1
    elif command -v docker >/dev/null 2>&1; then
        docker exec -i "${PROJECT_NAME:-nself}_postgres" psql -U "${POSTGRES_USER:-postgres}" -d "${POSTGRES_DB:-nself}" -c "$query" >/dev/null 2>&1
    else
        plugin_error "No PostgreSQL client available"
        return 1
    fi
}

# Check if table exists
plugin_table_exists() {
    local table_name="$1"
    local result

    result=$(plugin_db_query "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = '$table_name');")
    [[ "$result" =~ t ]]
}

# =============================================================================
# HTTP Functions
# =============================================================================

# Make HTTP request with curl
plugin_http_request() {
    local method="$1"
    local url="$2"
    local data="${3:-}"
    local headers=()
    shift 3 || true

    # Remaining args are headers
    while [[ $# -gt 0 ]]; do
        headers+=("-H" "$1")
        shift
    done

    local curl_args=("-s" "-X" "$method")

    if [[ -n "$data" ]]; then
        curl_args+=("-d" "$data")
    fi

    if [[ ${#headers[@]} -gt 0 ]]; then
        curl_args+=("${headers[@]}")
    fi

    curl "${curl_args[@]}" "$url"
}

plugin_http_get() {
    plugin_http_request "GET" "$@"
}

plugin_http_post() {
    plugin_http_request "POST" "$@"
}

# =============================================================================
# Webhook Functions
# =============================================================================

# Verify webhook signature (generic HMAC-SHA256)
plugin_verify_webhook_signature() {
    local payload="$1"
    local signature="$2"
    local secret="$3"

    local expected
    expected=$(printf '%s' "$payload" | openssl dgst -sha256 -hmac "$secret" | sed 's/^.* //')

    [[ "$signature" == "$expected" ]]
}

# Log webhook event
plugin_log_webhook() {
    local plugin_name="$1"
    local event_type="$2"
    local event_id="$3"
    local status="$4"

    local log_file="${PLUGIN_LOG_DIR}/${plugin_name}/webhooks.log"
    mkdir -p "$(dirname "$log_file")"

    printf "%s | %s | %s | %s\n" "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$event_type" "$event_id" "$status" >> "$log_file"
}

# =============================================================================
# Cache Functions
# =============================================================================

# Get cached value
plugin_cache_get() {
    local plugin_name="$1"
    local key="$2"
    local ttl="${3:-3600}"  # Default 1 hour TTL

    local cache_file="${PLUGIN_CACHE_DIR}/${plugin_name}/${key}"

    if [[ -f "$cache_file" ]]; then
        local file_age
        local current_time
        current_time=$(date +%s)

        if [[ "$OSTYPE" == "darwin"* ]]; then
            file_age=$(stat -f %m "$cache_file")
        else
            file_age=$(stat -c %Y "$cache_file")
        fi

        if (( current_time - file_age < ttl )); then
            cat "$cache_file"
            return 0
        fi
    fi

    return 1
}

# Set cached value
plugin_cache_set() {
    local plugin_name="$1"
    local key="$2"
    local value="$3"

    local cache_dir="${PLUGIN_CACHE_DIR}/${plugin_name}"
    mkdir -p "$cache_dir"

    printf '%s' "$value" > "${cache_dir}/${key}"
}

# Clear plugin cache
plugin_cache_clear() {
    local plugin_name="$1"
    local cache_dir="${PLUGIN_CACHE_DIR}/${plugin_name}"

    if [[ -d "$cache_dir" ]]; then
        rm -rf "$cache_dir"
        plugin_info "Cleared cache for $plugin_name"
    fi
}

# =============================================================================
# Validation Functions
# =============================================================================

# Validate plugin.json
plugin_validate_manifest() {
    local plugin_dir="$1"
    local manifest="${plugin_dir}/plugin.json"

    if [[ ! -f "$manifest" ]]; then
        plugin_error "plugin.json not found in $plugin_dir"
        return 1
    fi

    # Check required fields
    local required_fields=("name" "version" "description" "minNselfVersion")
    for field in "${required_fields[@]}"; do
        if ! grep -q "\"$field\"" "$manifest"; then
            plugin_error "Missing required field in plugin.json: $field"
            return 1
        fi
    done

    return 0
}

# =============================================================================
# Version Comparison
# =============================================================================

# Compare semantic versions
# Returns: 0 if v1 >= v2, 1 if v1 < v2
plugin_version_gte() {
    local v1="$1"
    local v2="$2"

    # Remove 'v' prefix if present
    v1="${v1#v}"
    v2="${v2#v}"

    # Split into parts
    IFS='.' read -ra V1_PARTS <<< "$v1"
    IFS='.' read -ra V2_PARTS <<< "$v2"

    for i in 0 1 2; do
        local p1="${V1_PARTS[$i]:-0}"
        local p2="${V2_PARTS[$i]:-0}"

        if (( p1 > p2 )); then
            return 0
        elif (( p1 < p2 )); then
            return 1
        fi
    done

    return 0
}

# =============================================================================
# JSON Helpers (using grep/sed for Bash 3.2 compatibility)
# =============================================================================

# Extract value from JSON (simple single-level)
plugin_json_get() {
    local json="$1"
    local key="$2"

    printf '%s' "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | sed 's/.*: *"\([^"]*\)".*/\1/' | head -1
}

# Extract array from JSON
plugin_json_get_array() {
    local json="$1"
    local key="$2"

    printf '%s' "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*\[[^]]*\]" | sed 's/.*\[\([^]]*\)\].*/\1/' | tr ',' '\n' | sed 's/[" ]//g'
}

# =============================================================================
# Plugin Registry Functions
# =============================================================================

# Ensure plugin registry table exists
plugin_ensure_registry_table() {
    plugin_db_exec "
        CREATE TABLE IF NOT EXISTS _nself_plugin_registry (
            plugin_name VARCHAR(255) PRIMARY KEY,
            version VARCHAR(50),
            installed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            status VARCHAR(50) DEFAULT 'active'
        );
    " 2>/dev/null || true
}

# Mark plugin as installed
plugin_mark_installed() {
    local plugin_name="$1"
    local version="${2:-1.0.0}"

    plugin_ensure_registry_table

    plugin_db_exec "
        INSERT INTO _nself_plugin_registry (plugin_name, version, installed_at, status)
        VALUES ('$plugin_name', '$version', CURRENT_TIMESTAMP, 'active')
        ON CONFLICT (plugin_name) DO UPDATE SET
            version = EXCLUDED.version,
            updated_at = CURRENT_TIMESTAMP,
            status = 'active';
    "

    plugin_debug "Marked $plugin_name as installed (version: $version)"
}

# Mark plugin as uninstalled
plugin_mark_uninstalled() {
    local plugin_name="$1"

    plugin_ensure_registry_table

    plugin_db_exec "
        UPDATE _nself_plugin_registry
        SET status = 'uninstalled', updated_at = CURRENT_TIMESTAMP
        WHERE plugin_name = '$plugin_name';
    "

    plugin_debug "Marked $plugin_name as uninstalled"
}

# Check if plugin is installed
plugin_is_installed() {
    local plugin_name="$1"

    plugin_ensure_registry_table

    local result
    result=$(plugin_db_query "SELECT COUNT(*) FROM _nself_plugin_registry WHERE plugin_name = '$plugin_name' AND status = 'active';")
    [[ "$result" =~ [1-9] ]]
}
