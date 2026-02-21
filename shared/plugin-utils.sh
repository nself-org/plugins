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

# Validate plugin.json (basic manifest check — required fields present)
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
# Validate plugin.json — full schema validation
# =============================================================================

# validate_plugin_json <plugin_json_path> [--quiet]
#
# Validates a plugin.json (the per-plugin manifest) against the nself schema:
#   - Required fields present and non-empty: name, version, description,
#     author, license, category, tags, tables
#   - version follows semver (x.y.z)
#   - category is one of the 13 official categories
#   - all table names carry the np_ prefix
#   - multiApp.isolationColumn is source_account_id (if multiApp is declared)
#   - source_account_id column present in schema/tables.sql (if file exists)
#
# Note: plugin.json does NOT contain an 'implementation' field — that lives
# only in registry.json. Use validate-registry.sh for registry validation.
#
# Returns 0 if valid, 1 if any errors. Prints errors to stderr.
# With --quiet, suppresses all output (useful for CI gating).
#
# Bash 3.2+ compatible.
validate_plugin_json() {
    local manifest="$1"
    local quiet="${2:-}"

    # -------------------------------------------------------------------
    # File existence and JSON validity
    # -------------------------------------------------------------------
    if [[ ! -f "$manifest" ]]; then
        printf "\033[31m[ERROR]\033[0m plugin.json not found: %s\n" "$manifest" >&2
        return 1
    fi

    local _vp_py3=""
    local _vp_py_candidate
    for _vp_py_candidate in python3 /usr/bin/python3 /opt/homebrew/bin/python3; do
        if command -v "$_vp_py_candidate" >/dev/null 2>&1; then
            _vp_py3="$_vp_py_candidate"
            break
        fi
    done
    if [[ -z "$_vp_py3" ]]; then
        plugin_error "python3 is required for validate_plugin_json"
        return 1
    fi

    if ! "$_vp_py3" -c "import json, sys; json.load(open(sys.argv[1]))" "$manifest" 2>/dev/null; then
        printf "\033[31m[ERROR]\033[0m %s: Invalid JSON — fix syntax errors first.\n" "$manifest" >&2
        return 1
    fi

    # -------------------------------------------------------------------
    # Extract fields to a temp file (avoids eval of JSON arrays/special chars)
    # -------------------------------------------------------------------
    local _vp_tmpdir
    _vp_tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t 'vpj')
    local _vp_datafile="${_vp_tmpdir}/data"

    "$_vp_py3" - "$manifest" "$_vp_datafile" <<'PYEOF'
import json, sys

with open(sys.argv[1]) as f:
    p = json.load(f)

outfile = sys.argv[2]
lines = []

def w(key, val):
    if isinstance(val, (list, dict)):
        lines.append(key + "=" + json.dumps(val, separators=(',', ':')))
    elif val is None:
        lines.append(key + "=")
    else:
        lines.append(key + "=" + str(val))

w("VPJ_NAME",        p.get("name", ""))
w("VPJ_VERSION",     p.get("version", ""))
w("VPJ_DESCRIPTION", p.get("description", ""))
w("VPJ_AUTHOR",      p.get("author", ""))
w("VPJ_LICENSE",     p.get("license", ""))
w("VPJ_CATEGORY",    p.get("category", ""))
w("VPJ_MINNSELF",    p.get("minNselfVersion", ""))

tags = p.get("tags")
lines.append("VPJ_TAGS_PRESENT=" + ("1" if tags is not None else "0"))
lines.append("VPJ_TAGS_COUNT="   + str(len(tags) if isinstance(tags, list) else 0))

tables = p.get("tables")
lines.append("VPJ_TABLES_PRESENT=" + ("1" if tables is not None else "0"))
lines.append("VPJ_TABLES_COUNT="   + str(len(tables) if isinstance(tables, list) else 0))
w("VPJ_TABLES_JSON", tables if isinstance(tables, list) else [])

multi = p.get("multiApp")
lines.append("VPJ_MULTI_PRESENT=" + ("1" if isinstance(multi, dict) else "0"))
w("VPJ_MULTI_ISOLATION", multi.get("isolationColumn","") if isinstance(multi, dict) else "")

with open(outfile, 'w') as f:
    f.write("\n".join(lines) + "\n")
PYEOF

    # Read fields from temp file (one key=value per line)
    local _vp_field_reader
    _vp_field_reader() {
        grep "^${1}=" "$_vp_datafile" 2>/dev/null | head -1 | cut -d'=' -f2-
    }

    local _vp_name; _vp_name=$(_vp_field_reader VPJ_NAME)
    local _vp_version; _vp_version=$(_vp_field_reader VPJ_VERSION)
    local _vp_description; _vp_description=$(_vp_field_reader VPJ_DESCRIPTION)
    local _vp_author; _vp_author=$(_vp_field_reader VPJ_AUTHOR)
    local _vp_license; _vp_license=$(_vp_field_reader VPJ_LICENSE)
    local _vp_category; _vp_category=$(_vp_field_reader VPJ_CATEGORY)
    local _vp_minnself; _vp_minnself=$(_vp_field_reader VPJ_MINNSELF)
    local _vp_tags_present; _vp_tags_present=$(_vp_field_reader VPJ_TAGS_PRESENT)
    local _vp_tags_count; _vp_tags_count=$(_vp_field_reader VPJ_TAGS_COUNT)
    local _vp_tables_present; _vp_tables_present=$(_vp_field_reader VPJ_TABLES_PRESENT)
    local _vp_tables_count; _vp_tables_count=$(_vp_field_reader VPJ_TABLES_COUNT)
    local _vp_tables_json; _vp_tables_json=$(_vp_field_reader VPJ_TABLES_JSON)
    local _vp_multi_present; _vp_multi_present=$(_vp_field_reader VPJ_MULTI_PRESENT)
    local _vp_multi_isolation; _vp_multi_isolation=$(_vp_field_reader VPJ_MULTI_ISOLATION)

    rm -rf "$_vp_tmpdir"

    # Plugin directory (parent of manifest file)
    local _vp_dir
    _vp_dir="$(cd "$(dirname "$manifest")" && pwd)"

    # Error counter
    local _vp_errors=0

    # Valid categories
    local _vp_valid_cats="authentication automation commerce communication content data development infrastructure integrations media streaming sports compliance"

    # Output helpers (use stderr to not pollute caller's stdout)
    _vp_err_out() {
        [[ "$quiet" == "--quiet" ]] && return 0
        printf "\033[31m[ERROR]\033[0m %s — %s: %s\n" "${_vp_name:-unknown}" "$1" "$2" >&2
    }
    _vp_warn_out() {
        [[ "$quiet" == "--quiet" ]] && return 0
        printf "\033[33m[WARN]\033[0m  %s — %s: %s\n" "${_vp_name:-unknown}" "$1" "$2" >&2
    }

    # -------------------------------------------------------------------
    # 1. Required string fields
    # -------------------------------------------------------------------
    local _vp_check_val
    for _vp_check_val in \
        "name:$_vp_name" \
        "version:$_vp_version" \
        "description:$_vp_description" \
        "author:$_vp_author" \
        "license:$_vp_license" \
        "category:$_vp_category"
    do
        local _vp_fname="${_vp_check_val%%:*}"
        local _vp_fval="${_vp_check_val#*:}"
        if [[ -z "$_vp_fval" ]]; then
            _vp_err_out "$_vp_fname" "Required field is missing or empty"
            _vp_errors=$((_vp_errors + 1))
        fi
    done

    # -------------------------------------------------------------------
    # 2. Semver: x.y.z (digits only)
    # -------------------------------------------------------------------
    if [[ -n "$_vp_version" ]]; then
        local _vp_vc="${_vp_version#v}"
        local _vp_vmaj="${_vp_vc%%.*}"
        local _vp_vrest="${_vp_vc#*.}"
        local _vp_vmin="${_vp_vrest%%.*}"
        local _vp_vpat="${_vp_vrest#*.}"
        local _vp_ver_ok=true
        [[ "$_vp_vc" != *.*.* ]]         && _vp_ver_ok=false
        [[ "$_vp_vmaj" =~ [^0-9] ]]       && _vp_ver_ok=false
        [[ "$_vp_vmin" =~ [^0-9] ]]       && _vp_ver_ok=false
        [[ "$_vp_vpat" =~ [^0-9] ]]       && _vp_ver_ok=false
        if [[ "$_vp_ver_ok" == "false" ]]; then
            _vp_err_out "version" "Invalid semver: '$_vp_version' (expected x.y.z)"
            _vp_errors=$((_vp_errors + 1))
        fi
    fi

    # -------------------------------------------------------------------
    # 3. Category validation
    # -------------------------------------------------------------------
    if [[ -n "$_vp_category" ]]; then
        local _vp_cat_ok=false
        local _vp_c
        for _vp_c in $_vp_valid_cats; do
            [[ "$_vp_c" == "$_vp_category" ]] && _vp_cat_ok=true && break
        done
        if [[ "$_vp_cat_ok" == "false" ]]; then
            _vp_err_out "category" "Invalid: '$_vp_category'. Valid: $_vp_valid_cats"
            _vp_errors=$((_vp_errors + 1))
        fi
    fi

    # -------------------------------------------------------------------
    # 4. Tags — present and non-empty
    # -------------------------------------------------------------------
    if [[ "${_vp_tags_present:-0}" == "0" ]]; then
        _vp_err_out "tags" "Required field 'tags' is missing"
        _vp_errors=$((_vp_errors + 1))
    elif [[ "${_vp_tags_count:-0}" == "0" ]]; then
        _vp_err_out "tags" "tags array is empty — at least one tag required"
        _vp_errors=$((_vp_errors + 1))
    fi

    # -------------------------------------------------------------------
    # 5. Tables — present; all entries carry np_ prefix
    # -------------------------------------------------------------------
    if [[ "${_vp_tables_present:-0}" == "0" ]]; then
        _vp_err_out "tables" "Required field 'tables' is missing"
        _vp_errors=$((_vp_errors + 1))
    else
        local _vp_table_list
        _vp_table_list=$("$_vp_py3" -c "
import json, sys
tables = json.loads(sys.argv[1])
for t in tables:
    print(t)
" "$_vp_tables_json" 2>/dev/null)

        local _vp_table
        while IFS= read -r _vp_table; do
            [[ -z "$_vp_table" ]] && continue
            case "$_vp_table" in
                np_*) ;;
                *)
                    _vp_err_out "tables" "Table '$_vp_table' missing np_ prefix"
                    _vp_errors=$((_vp_errors + 1))
                    ;;
            esac
        done <<< "$_vp_table_list"
    fi

    # -------------------------------------------------------------------
    # 6. SQL schema: source_account_id column required
    # -------------------------------------------------------------------
    local _vp_sql="${_vp_dir}/schema/tables.sql"
    if [[ -f "$_vp_sql" ]]; then
        if ! grep -q "source_account_id" "$_vp_sql" 2>/dev/null; then
            _vp_err_out "schema/tables.sql" "Missing source_account_id column (required for multi-app isolation)"
            _vp_errors=$((_vp_errors + 1))
        fi
    fi

    # -------------------------------------------------------------------
    # 7. multiApp isolation column must be source_account_id
    # -------------------------------------------------------------------
    if [[ "${_vp_multi_present:-0}" == "1" ]]; then
        if [[ "$_vp_multi_isolation" != "source_account_id" ]]; then
            _vp_err_out "multiApp.isolationColumn" "Must be 'source_account_id', found: '$_vp_multi_isolation'"
            _vp_errors=$((_vp_errors + 1))
        fi
    fi

    # -------------------------------------------------------------------
    # 8. minNselfVersion — recommended
    # -------------------------------------------------------------------
    if [[ -z "$_vp_minnself" ]]; then
        _vp_warn_out "minNselfVersion" "Recommended field is missing"
    fi

    # -------------------------------------------------------------------
    # Result
    # -------------------------------------------------------------------
    if [[ $_vp_errors -eq 0 ]]; then
        [[ "$quiet" != "--quiet" ]] && plugin_success "plugin.json valid: ${_vp_name} v${_vp_version}"
        return 0
    else
        [[ "$quiet" != "--quiet" ]] && plugin_error "plugin.json has $_vp_errors error(s): ${manifest}"
        return 1
    fi
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
