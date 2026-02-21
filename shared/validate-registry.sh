#!/bin/bash
# =============================================================================
# nself Plugin Registry Validator
# Validates every plugin entry in registry.json against the required schema
# =============================================================================
#
# Usage:
#   validate-registry.sh [OPTIONS]
#
# Options:
#   --fix         Auto-fix simple issues (category normalization suggestions)
#   --json        Output results as machine-readable JSON
#   --plugin <n>  Validate a single plugin by name
#   --help        Show this help message
#
# Exit codes:
#   0 = All plugins valid (warnings do not fail)
#   1 = One or more validation errors found
#
# Bash 3.2+ compatible (macOS default shell)
# =============================================================================

set -uo pipefail

# ---------------------------------------------------------------------------
# Script location — resolve registry.json relative to this script
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REGISTRY_FILE="${REGISTRY_ROOT}/registry.json"
FREE_DIR="${REGISTRY_ROOT}/free"
COMMUNITY_DIR="${REGISTRY_ROOT}/community"

# ---------------------------------------------------------------------------
# Color constants (disabled when piped or --json is active)
# ---------------------------------------------------------------------------
RED=""
GREEN=""
YELLOW=""
CYAN=""
BOLD=""
DIM=""
RESET=""

if [ -t 1 ] && [ "${NO_COLOR:-}" = "" ]; then
    RED="\033[0;31m"
    GREEN="\033[0;32m"
    YELLOW="\033[0;33m"
    CYAN="\033[0;36m"
    BOLD="\033[1m"
    DIM="\033[2m"
    RESET="\033[0m"
fi

# ---------------------------------------------------------------------------
# Valid categories (13 official — never add more without team approval)
# ---------------------------------------------------------------------------
VALID_CATEGORIES="authentication automation commerce communication content data development infrastructure integrations media streaming sports compliance"

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
OPT_FIX=false
OPT_JSON=false
OPT_PLUGIN=""

while [ $# -gt 0 ]; do
    case "$1" in
        --fix)
            OPT_FIX=true
            shift
            ;;
        --json)
            OPT_JSON=true
            RED=""; GREEN=""; YELLOW=""; CYAN=""; BOLD=""; DIM=""; RESET=""
            shift
            ;;
        --plugin)
            shift
            OPT_PLUGIN="${1:-}"
            if [ -z "$OPT_PLUGIN" ]; then
                printf "ERROR: --plugin requires a plugin name\n" >&2
                exit 1
            fi
            shift
            ;;
        --help|-h)
            sed -n '2,20p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            printf "Unknown option: %s\n" "$1" >&2
            exit 1
            ;;
    esac
done

# ---------------------------------------------------------------------------
# Locate python3
# ---------------------------------------------------------------------------
PYTHON3=""
for _py in python3 /usr/bin/python3 /opt/homebrew/bin/python3; do
    if command -v "$_py" >/dev/null 2>&1; then
        PYTHON3="$_py"
        break
    fi
done

if [ -z "$PYTHON3" ]; then
    printf "${RED}ERROR:${RESET} python3 is required for JSON parsing.\n" >&2
    printf "Install it with: brew install python3\n" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Sanity: registry file must exist and be valid JSON
# ---------------------------------------------------------------------------
if [ ! -f "$REGISTRY_FILE" ]; then
    printf "${RED}ERROR:${RESET} registry.json not found at: %s\n" "$REGISTRY_FILE" >&2
    exit 1
fi

if ! "$PYTHON3" -c "import json, sys; json.load(open(sys.argv[1]))" "$REGISTRY_FILE" 2>/dev/null; then
    printf "${RED}ERROR:${RESET} registry.json is not valid JSON. Fix syntax errors first.\n" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Temp dir for inter-call data (cleaned on exit)
# ---------------------------------------------------------------------------
TMPDIR_VAL=$(mktemp -d 2>/dev/null || mktemp -d -t 'vr')
trap 'rm -rf "$TMPDIR_VAL"' EXIT

# ---------------------------------------------------------------------------
# Global counters (written to temp files to survive subshell boundaries)
# ---------------------------------------------------------------------------
printf '0' > "${TMPDIR_VAL}/total"
printf '0' > "${TMPDIR_VAL}/valid"
printf '0' > "${TMPDIR_VAL}/errors"
printf '0' > "${TMPDIR_VAL}/warnings"
printf '' > "${TMPDIR_VAL}/json_entries"

_inc() {
    local file="${TMPDIR_VAL}/$1"
    local val
    val=$(cat "$file")
    printf '%d' $((val + 1)) > "$file"
}

_get() {
    cat "${TMPDIR_VAL}/$1"
}

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------
_print_error() {
    local plugin="$1" field="$2" message="$3"
    _inc errors
    if [ "$OPT_JSON" = "false" ]; then
        printf "  ${RED}[ERROR]${RESET} ${BOLD}%s${RESET} — %s: %s\n" "$plugin" "$field" "$message"
    fi
}

_print_warning() {
    local plugin="$1" field="$2" message="$3"
    _inc warnings
    if [ "$OPT_JSON" = "false" ]; then
        printf "  ${YELLOW}[WARN]${RESET}  ${BOLD}%s${RESET} — %s: %s\n" "$plugin" "$field" "$message"
    fi
}

_print_fixed() {
    local plugin="$1" field="$2" message="$3"
    if [ "$OPT_JSON" = "false" ]; then
        printf "  ${CYAN}[FIXED]${RESET} ${BOLD}%s${RESET} — %s: %s\n" "$plugin" "$field" "$message"
    fi
}

# ---------------------------------------------------------------------------
# Semver validation — x.y.z (digits only)
# ---------------------------------------------------------------------------
_is_valid_semver() {
    local v="${1#v}"    # strip optional leading v
    case "$v" in
        *.*.*) ;;
        *) return 1 ;;
    esac
    local major="${v%%.*}"
    local rest="${v#*.}"
    local minor="${rest%%.*}"
    local patch="${rest#*.}"
    # All three parts must be non-empty digits only
    case "$major" in *[!0-9]*|"") return 1 ;; esac
    case "$minor" in *[!0-9]*|"") return 1 ;; esac
    case "$patch" in *[!0-9]*|"") return 1 ;; esac
    return 0
}

# ---------------------------------------------------------------------------
# Category validation
# ---------------------------------------------------------------------------
_is_valid_category() {
    local cat="$1"
    local c
    for c in $VALID_CATEGORIES; do
        [ "$c" = "$cat" ] && return 0
    done
    return 1
}

# ---------------------------------------------------------------------------
# Find plugin directory on disk (free/ or community/)
# Prints path to stdout; returns 1 if not found
# ---------------------------------------------------------------------------
_find_plugin_dir() {
    local name="$1"
    local dir
    for dir in "${FREE_DIR}/${name}" "${COMMUNITY_DIR}/${name}"; do
        if [ -d "$dir" ]; then
            printf '%s' "$dir"
            return 0
        fi
    done
    return 1
}

# ---------------------------------------------------------------------------
# Extract a single-line field from the per-plugin temp data file
# ---------------------------------------------------------------------------
_field() {
    local datafile="$1"
    local key="$2"
    grep "^${key}=" "$datafile" 2>/dev/null | head -1 | cut -d'=' -f2-
}

# ---------------------------------------------------------------------------
# Write plugin data to a temp file via Python (avoids eval of JSON arrays)
# Each value is written as a separate line: KEY=value
# Multiline-safe: one key per line, value is everything after first '='
# ---------------------------------------------------------------------------
_extract_plugin_data() {
    local plugin_name="$1"
    local outfile="$2"

    "$PYTHON3" - "$REGISTRY_FILE" "$plugin_name" "$outfile" <<'PYEOF'
import json, sys, os

registry_file = sys.argv[1]
plugin_name   = sys.argv[2]
outfile       = sys.argv[3]

with open(registry_file) as f:
    data = json.load(f)

p = data["plugins"].get(plugin_name)
if p is None:
    with open(outfile, 'w') as out:
        out.write("NOT_FOUND=1\n")
    sys.exit(0)

lines = []

def w(key, val):
    """Write key=val, converting non-strings to their JSON representation."""
    if isinstance(val, (list, dict)):
        lines.append(key + "=" + json.dumps(val, separators=(',',':')))
    elif val is None:
        lines.append(key + "=")
    else:
        lines.append(key + "=" + str(val))

w("NAME",        p.get("name", ""))
w("VERSION",     p.get("version", ""))
w("DESCRIPTION", p.get("description", ""))
w("AUTHOR",      p.get("author", ""))
w("LICENSE",     p.get("license", ""))
w("CATEGORY",    p.get("category", ""))
w("PATH_REG",    p.get("path", ""))
w("MIN_NSELF",   p.get("minNselfVersion", ""))

tags = p.get("tags")
lines.append("TAGS_PRESENT=" + ("1" if tags is not None else "0"))
lines.append("TAGS_COUNT="   + str(len(tags) if isinstance(tags, list) else 0))

tables = p.get("tables")
lines.append("TABLES_PRESENT=" + ("1" if tables is not None else "0"))
lines.append("TABLES_COUNT="   + str(len(tables) if isinstance(tables, list) else 0))
if isinstance(tables, list):
    w("TABLES_JSON", tables)
else:
    lines.append("TABLES_JSON=[]")

impl = p.get("implementation")
lines.append("IMPL_PRESENT=" + ("1" if isinstance(impl, dict) else "0"))
if isinstance(impl, dict):
    w("IMPL_LANGUAGE", impl.get("language", ""))
    w("IMPL_RUNTIME",  impl.get("runtime",  ""))
    w("IMPL_ENTRY",    impl.get("entryPoint",""))
    w("IMPL_CLI",      impl.get("cli",       ""))
else:
    # Some entries (observability, admin-api) have flat "language" key
    w("IMPL_LANGUAGE", p.get("language", ""))
    lines.append("IMPL_RUNTIME=")
    lines.append("IMPL_ENTRY=")
    lines.append("IMPL_CLI=")

multi = p.get("multiApp")
lines.append("MULTIAPP_PRESENT=" + ("1" if isinstance(multi, dict) else "0"))
if isinstance(multi, dict):
    w("MULTIAPP_ISOLATION", multi.get("isolationColumn", ""))
else:
    lines.append("MULTIAPP_ISOLATION=")

with open(outfile, 'w') as out:
    out.write("\n".join(lines) + "\n")
PYEOF
}

# ---------------------------------------------------------------------------
# Validate one plugin
# Uses a temp data file — no eval, no subshells for counters
# ---------------------------------------------------------------------------
_validate_plugin() {
    local plugin_name="$1"
    local plugin_errors=0
    local plugin_warnings=0
    local datafile="${TMPDIR_VAL}/plugin_${plugin_name}.dat"

    # Extract plugin fields to temp file
    _extract_plugin_data "$plugin_name" "$datafile"

    # NOT_FOUND guard
    if grep -q "^NOT_FOUND=1" "$datafile" 2>/dev/null; then
        _print_error "$plugin_name" "registry" "Plugin not found in registry.json"
        return
    fi

    # Read fields
    local NAME VERSION DESCRIPTION AUTHOR LICENSE CATEGORY PATH_REG MIN_NSELF
    local TAGS_PRESENT TAGS_COUNT TABLES_PRESENT TABLES_COUNT TABLES_JSON
    local IMPL_PRESENT IMPL_LANGUAGE IMPL_RUNTIME IMPL_ENTRY IMPL_CLI
    local MULTIAPP_PRESENT MULTIAPP_ISOLATION

    NAME=$(_field "$datafile" "NAME")
    VERSION=$(_field "$datafile" "VERSION")
    DESCRIPTION=$(_field "$datafile" "DESCRIPTION")
    AUTHOR=$(_field "$datafile" "AUTHOR")
    LICENSE=$(_field "$datafile" "LICENSE")
    CATEGORY=$(_field "$datafile" "CATEGORY")
    PATH_REG=$(_field "$datafile" "PATH_REG")
    MIN_NSELF=$(_field "$datafile" "MIN_NSELF")
    TAGS_PRESENT=$(_field "$datafile" "TAGS_PRESENT")
    TAGS_COUNT=$(_field "$datafile" "TAGS_COUNT")
    TABLES_PRESENT=$(_field "$datafile" "TABLES_PRESENT")
    TABLES_COUNT=$(_field "$datafile" "TABLES_COUNT")
    TABLES_JSON=$(_field "$datafile" "TABLES_JSON")
    IMPL_PRESENT=$(_field "$datafile" "IMPL_PRESENT")
    IMPL_LANGUAGE=$(_field "$datafile" "IMPL_LANGUAGE")
    IMPL_RUNTIME=$(_field "$datafile" "IMPL_RUNTIME")
    IMPL_ENTRY=$(_field "$datafile" "IMPL_ENTRY")
    IMPL_CLI=$(_field "$datafile" "IMPL_CLI")
    MULTIAPP_PRESENT=$(_field "$datafile" "MULTIAPP_PRESENT")
    MULTIAPP_ISOLATION=$(_field "$datafile" "MULTIAPP_ISOLATION")

    # ------------------------------------------------------------------
    # 1. Required string fields
    # ------------------------------------------------------------------
    local _check_field _check_val
    for _check_field in NAME VERSION DESCRIPTION AUTHOR LICENSE CATEGORY; do
        eval "_check_val=\"\${${_check_field}}\""
        if [ -z "$_check_val" ]; then
            local _fname
            _fname=$(printf '%s' "$_check_field" | tr '[:upper:]' '[:lower:]')
            _print_error "$plugin_name" "$_fname" "Required field is missing or empty"
            plugin_errors=$((plugin_errors + 1))
        fi
    done

    # ------------------------------------------------------------------
    # 2. name field must match the registry key
    # ------------------------------------------------------------------
    if [ -n "$NAME" ] && [ "$NAME" != "$plugin_name" ]; then
        _print_error "$plugin_name" "name" "name field ('$NAME') does not match registry key ('$plugin_name')"
        plugin_errors=$((plugin_errors + 1))
    fi

    # ------------------------------------------------------------------
    # 3. version must be strict semver x.y.z
    # ------------------------------------------------------------------
    if [ -n "$VERSION" ]; then
        if ! _is_valid_semver "$VERSION"; then
            _print_error "$plugin_name" "version" "Invalid semver: '$VERSION' (expected x.y.z)"
            plugin_errors=$((plugin_errors + 1))
        fi
    fi

    # ------------------------------------------------------------------
    # 4. category must be one of the 13 valid categories
    # ------------------------------------------------------------------
    if [ -n "$CATEGORY" ]; then
        if ! _is_valid_category "$CATEGORY"; then
            _print_error "$plugin_name" "category" "Invalid category: '$CATEGORY'. Valid: $VALID_CATEGORIES"
            plugin_errors=$((plugin_errors + 1))

            if [ "$OPT_FIX" = "true" ]; then
                local _fixed_cat=""
                case "$CATEGORY" in
                    networking|network|dns) _fixed_cat="infrastructure" ;;
                    security|auth)          _fixed_cat="authentication" ;;
                    ecommerce|payments|payment) _fixed_cat="commerce" ;;
                    messaging|chat)         _fixed_cat="communication" ;;
                    analytics|metrics|monitoring) _fixed_cat="data" ;;
                    devops|dev|tools)       _fixed_cat="development" ;;
                    storage|cloud|hosting)  _fixed_cat="infrastructure" ;;
                    video|audio)            _fixed_cat="media" ;;
                    live|broadcast)         _fixed_cat="streaming" ;;
                esac
                if [ -n "$_fixed_cat" ]; then
                    _print_fixed "$plugin_name" "category" "Suggested mapping: '$CATEGORY' -> '$_fixed_cat' (apply manually in registry.json)"
                fi
            fi
        fi
    fi

    # ------------------------------------------------------------------
    # 5. tags — must be present and non-empty
    # ------------------------------------------------------------------
    if [ "${TAGS_PRESENT:-0}" = "0" ]; then
        _print_error "$plugin_name" "tags" "Required field 'tags' is missing"
        plugin_errors=$((plugin_errors + 1))
    elif [ "${TAGS_COUNT:-0}" = "0" ]; then
        _print_error "$plugin_name" "tags" "tags array is empty — at least one tag required"
        plugin_errors=$((plugin_errors + 1))
    fi

    # ------------------------------------------------------------------
    # 6. tables — must be present; all entries must have np_ prefix
    # ------------------------------------------------------------------
    if [ "${TABLES_PRESENT:-0}" = "0" ]; then
        _print_error "$plugin_name" "tables" "Required field 'tables' is missing"
        plugin_errors=$((plugin_errors + 1))
    else
        # Extract table names via Python (handles JSON array safely)
        local _table_names
        _table_names=$("$PYTHON3" -c "
import json, sys
tables = json.loads(sys.argv[1])
for t in tables:
    print(t)
" "$TABLES_JSON" 2>/dev/null)

        local _table
        while IFS= read -r _table; do
            [ -z "$_table" ] && continue
            case "$_table" in
                np_*) ;;  # valid
                *)
                    _print_error "$plugin_name" "tables" "Table '$_table' missing required np_ prefix"
                    plugin_errors=$((plugin_errors + 1))
                    ;;
            esac
        done <<EOF
$_table_names
EOF
    fi

    # ------------------------------------------------------------------
    # 7. implementation — must be a nested object with required sub-fields
    # ------------------------------------------------------------------
    if [ "${IMPL_PRESENT:-0}" = "0" ]; then
        _print_error "$plugin_name" "implementation" "Required 'implementation' object is missing (flat 'language' key found — must be nested)"
        plugin_errors=$((plugin_errors + 1))
    else
        if [ -z "$IMPL_LANGUAGE" ]; then
            _print_error "$plugin_name" "implementation.language" "Required sub-field is missing or empty"
            plugin_errors=$((plugin_errors + 1))
        fi
        if [ -z "$IMPL_RUNTIME" ]; then
            _print_error "$plugin_name" "implementation.runtime" "Required sub-field is missing or empty"
            plugin_errors=$((plugin_errors + 1))
        fi
        if [ -z "$IMPL_ENTRY" ]; then
            _print_error "$plugin_name" "implementation.entryPoint" "Required sub-field is missing or empty"
            plugin_errors=$((plugin_errors + 1))
        fi
    fi

    # ------------------------------------------------------------------
    # 8. File existence — only for plugins present on disk
    # ------------------------------------------------------------------
    local _plugin_dir=""
    _plugin_dir=$(_find_plugin_dir "$plugin_name") || true

    if [ -n "$_plugin_dir" ]; then
        # 8a. plugin.json must exist
        if [ ! -f "${_plugin_dir}/plugin.json" ]; then
            _print_error "$plugin_name" "files" "plugin.json not found at ${_plugin_dir}/plugin.json"
            plugin_errors=$((plugin_errors + 1))
        else
            # 8b. Version in plugin.json must match registry version
            local _disk_version
            _disk_version=$("$PYTHON3" -c "
import json, sys
try:
    with open(sys.argv[1]) as f:
        d = json.load(f)
    print(d.get('version',''))
except Exception:
    print('')
" "${_plugin_dir}/plugin.json" 2>/dev/null)

            if [ -n "$_disk_version" ] && [ -n "$VERSION" ] && [ "$_disk_version" != "$VERSION" ]; then
                _print_error "$plugin_name" "version" "Mismatch: registry='$VERSION', plugin.json='$_disk_version'"
                plugin_errors=$((plugin_errors + 1))
            fi
        fi

        # 8c. entryPoint file existence (warn — may not be built yet)
        if [ -n "$IMPL_ENTRY" ]; then
            if [ ! -f "${_plugin_dir}/${IMPL_ENTRY}" ]; then
                _print_warning "$plugin_name" "implementation.entryPoint" "File not found: ${_plugin_dir}/${IMPL_ENTRY} (run: pnpm build)"
                plugin_warnings=$((plugin_warnings + 1))
            fi
        fi

        # 8d. CLI file existence (warn — may not be built yet)
        if [ -n "$IMPL_CLI" ]; then
            if [ ! -f "${_plugin_dir}/${IMPL_CLI}" ]; then
                _print_warning "$plugin_name" "implementation.cli" "File not found: ${_plugin_dir}/${IMPL_CLI} (run: pnpm build)"
                plugin_warnings=$((plugin_warnings + 1))
            fi
        fi

        # 8e. SQL schema: source_account_id column required
        local _sql_file="${_plugin_dir}/schema/tables.sql"
        if [ -f "$_sql_file" ] && [ "${TABLES_COUNT:-0}" != "0" ]; then
            if ! grep -q "source_account_id" "$_sql_file" 2>/dev/null; then
                _print_error "$plugin_name" "schema/tables.sql" "source_account_id column missing (required for multi-app isolation)"
                plugin_errors=$((plugin_errors + 1))
            fi
        fi

    else
        # Not on disk — expected for Source-Available plugins (not yet open-sourced)
        # Only warn for MIT-licensed plugins (should be in free/)
        if [ "$LICENSE" = "MIT" ]; then
            _print_warning "$plugin_name" "files" "Plugin directory not found in free/ or community/ (expected for MIT plugins)"
            plugin_warnings=$((plugin_warnings + 1))
        fi
    fi

    # ------------------------------------------------------------------
    # 9. multiApp isolation column must be source_account_id
    # ------------------------------------------------------------------
    if [ "${MULTIAPP_PRESENT:-0}" = "1" ]; then
        if [ "$MULTIAPP_ISOLATION" != "source_account_id" ]; then
            _print_error "$plugin_name" "multiApp.isolationColumn" "Must be 'source_account_id', found: '$MULTIAPP_ISOLATION'"
            plugin_errors=$((plugin_errors + 1))
        fi
    fi

    # ------------------------------------------------------------------
    # 10. minNselfVersion — recommended
    # ------------------------------------------------------------------
    if [ -z "$MIN_NSELF" ]; then
        _print_warning "$plugin_name" "minNselfVersion" "Recommended field is missing"
        plugin_warnings=$((plugin_warnings + 1))
    fi

    # ------------------------------------------------------------------
    # Result for this plugin
    # ------------------------------------------------------------------
    if [ $plugin_errors -eq 0 ]; then
        _inc valid
        if [ "$OPT_JSON" = "false" ]; then
            if [ $plugin_warnings -eq 0 ]; then
                printf "  ${GREEN}[OK]${RESET}    ${BOLD}%s${RESET} v%s\n" "$plugin_name" "$VERSION"
            else
                printf "  ${YELLOW}[WARN]${RESET}  ${BOLD}%s${RESET} v%s (%d warning(s))\n" "$plugin_name" "$VERSION" "$plugin_warnings"
            fi
        fi
    fi

    # Accumulate JSON entry (appended to file)
    if [ "$OPT_JSON" = "true" ]; then
        local _jstatus="valid"
        [ $plugin_errors -gt 0 ] && _jstatus="invalid"
        [ $plugin_errors -eq 0 ] && [ $plugin_warnings -gt 0 ] && _jstatus="warning"
        printf '{"plugin":"%s","version":"%s","status":"%s","errors":%d,"warnings":%d},\n' \
            "$plugin_name" "$VERSION" "$_jstatus" "$plugin_errors" "$plugin_warnings" \
            >> "${TMPDIR_VAL}/json_entries"
    fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

if [ "$OPT_JSON" = "false" ]; then
    printf "\n${BOLD}nself Plugin Registry Validator${RESET}\n"
    printf "${DIM}Registry: %s${RESET}\n\n" "$REGISTRY_FILE"
fi

# Get plugin names from registry
PLUGIN_NAMES=$("$PYTHON3" -c "
import json, sys
with open(sys.argv[1]) as f:
    data = json.load(f)
for name in data['plugins']:
    print(name)
" "$REGISTRY_FILE" 2>/dev/null)

TOTAL_PLUGINS=$(printf '%s\n' "$PLUGIN_NAMES" | grep -c '.' 2>/dev/null || printf '0')
printf '%d' "$TOTAL_PLUGINS" > "${TMPDIR_VAL}/total"

if [ "$OPT_JSON" = "false" ]; then
    printf "${BOLD}Validating %d plugin(s)...${RESET}\n\n" "$TOTAL_PLUGINS"
fi

# ---------------------------------------------------------------------------
# Validate — single plugin or all
# ---------------------------------------------------------------------------
if [ -n "$OPT_PLUGIN" ]; then
    # Verify the plugin exists
    if ! printf '%s\n' "$PLUGIN_NAMES" | grep -qx "$OPT_PLUGIN"; then
        printf "${RED}ERROR:${RESET} Plugin '%s' not found in registry.\n" "$OPT_PLUGIN" >&2
        exit 1
    fi
    printf '1' > "${TMPDIR_VAL}/total"
    _validate_plugin "$OPT_PLUGIN"
else
    while IFS= read -r plugin_name; do
        [ -z "$plugin_name" ] && continue
        _validate_plugin "$plugin_name"
    done <<EOF
$PLUGIN_NAMES
EOF
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
FINAL_TOTAL=$(_get total)
FINAL_VALID=$(_get valid)
FINAL_ERRORS=$(_get errors)
FINAL_WARNINGS=$(_get warnings)
FINAL_INVALID=$((FINAL_TOTAL - FINAL_VALID))

if [ "$OPT_JSON" = "true" ]; then
    # Build JSON output from accumulated entries file
    local_status="pass"
    [ "$FINAL_ERRORS" -gt 0 ] && local_status="fail"

    printf '{\n'
    printf '  "status": "%s",\n' "$local_status"
    printf '  "summary": {\n'
    printf '    "total": %d,\n'    "$FINAL_TOTAL"
    printf '    "valid": %d,\n'    "$FINAL_VALID"
    printf '    "invalid": %d,\n'  "$FINAL_INVALID"
    printf '    "errors": %d,\n'   "$FINAL_ERRORS"
    printf '    "warnings": %d\n'  "$FINAL_WARNINGS"
    printf '  },\n'
    printf '  "plugins": [\n'

    # Strip trailing comma+newline from last entry and output the list
    if [ -s "${TMPDIR_VAL}/json_entries" ]; then
        # Write all but strip trailing comma from last line
        "$PYTHON3" -c "
import sys
lines = open(sys.argv[1]).read().rstrip().rstrip(',')
print(lines)
" "${TMPDIR_VAL}/json_entries"
    fi

    printf '\n  ]\n}\n'
else
    _sep="━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    printf "\n${BOLD}%s${RESET}\n" "$_sep"
    printf "${BOLD}Summary${RESET}\n"
    printf "  Total plugins:   %d\n" "$FINAL_TOTAL"
    printf "  ${GREEN}Valid:           %d${RESET}\n" "$FINAL_VALID"

    if [ "$FINAL_INVALID" -gt 0 ]; then
        printf "  ${RED}Invalid:         %d${RESET}\n" "$FINAL_INVALID"
    else
        printf "  Invalid:         %d\n" "$FINAL_INVALID"
    fi

    if [ "$FINAL_ERRORS" -gt 0 ]; then
        printf "  ${RED}Errors:          %d${RESET}\n" "$FINAL_ERRORS"
    else
        printf "  ${GREEN}Errors:          0${RESET}\n"
    fi

    if [ "$FINAL_WARNINGS" -gt 0 ]; then
        printf "  ${YELLOW}Warnings:        %d${RESET}\n" "$FINAL_WARNINGS"
    else
        printf "  Warnings:        0\n"
    fi

    printf "${BOLD}%s${RESET}\n\n" "$_sep"

    if [ "$FINAL_ERRORS" -gt 0 ]; then
        printf "${RED}${BOLD}FAILED${RESET} — %d error(s) found. Fix before pushing to registry.\n\n" "$FINAL_ERRORS"
    else
        printf "${GREEN}${BOLD}PASSED${RESET} — Registry is valid"
        if [ "$FINAL_WARNINGS" -gt 0 ]; then
            printf " (with %d warning(s))" "$FINAL_WARNINGS"
        fi
        printf ".\n\n"
    fi
fi

# ---------------------------------------------------------------------------
# Exit code: 0 = clean, 1 = errors
# ---------------------------------------------------------------------------
[ "$FINAL_ERRORS" -eq 0 ]
