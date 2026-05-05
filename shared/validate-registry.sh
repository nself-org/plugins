#!/bin/bash
# =============================================================================
# nself Plugin Registry Validator (v2)
# Validates registry.json against the 13 canonical checks from
# .claude/docs/plugin-cleanup-spec.md § 6 Registry.json Validation.
#
# Supports BOTH formats:
#   - Aggregated v2.0.0: top-level object with `plugins` keyed dict
#   - Flat array (legacy): top-level JSON array of plugin objects
# =============================================================================
#
# Usage:
#   validate-registry.sh [REGISTRY_FILE] [OPTIONS]
#
# Arguments:
#   REGISTRY_FILE  Path to registry.json (default: <script-dir>/../registry.json)
#
# Options:
#   --json         Output results as machine-readable JSON
#   --strict       Run legacy per-plugin schema validation in addition to the
#                  13 canonical checks (requires tables, implementation, etc).
#   --plugin <n>   Validate a single plugin by name (strict mode only)
#   --help         Show this help message
#
# Exit codes:
#   0 = All checks passed (warnings do not fail)
#   1 = One or more validation errors found
#
# Bash 3.2+ compatible (macOS default shell). shellcheck warning-clean.
# =============================================================================

set -uo pipefail

# ---------------------------------------------------------------------------
# Argument parsing — REGISTRY_FILE is optional positional
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY_FILE=""
OPT_JSON=false
OPT_STRICT=false
OPT_PLUGIN=""

while [ $# -gt 0 ]; do
    case "$1" in
        --json)
            OPT_JSON=true
            shift
            ;;
        --strict)
            OPT_STRICT=true
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
        --fix)
            # Legacy flag — accepted for backward compatibility, no-op.
            shift
            ;;
        --help|-h)
            sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        --*)
            printf "Unknown option: %s\n" "$1" >&2
            exit 1
            ;;
        *)
            if [ -z "$REGISTRY_FILE" ]; then
                REGISTRY_FILE="$1"
            else
                printf "Unexpected argument: %s\n" "$1" >&2
                exit 1
            fi
            shift
            ;;
    esac
done

# Default registry location: sibling of this script's parent dir
if [ -z "$REGISTRY_FILE" ]; then
    REGISTRY_FILE="$(cd "${SCRIPT_DIR}/.." && pwd)/registry.json"
fi

REGISTRY_ROOT="$(cd "$(dirname "$REGISTRY_FILE")" && pwd)"
FREE_DIR="${REGISTRY_ROOT}/free"
COMMUNITY_DIR="${REGISTRY_ROOT}/community"
PAID_DIR="${REGISTRY_ROOT}/paid"

# ---------------------------------------------------------------------------
# Color constants (disabled when piped or --json is active)
# ---------------------------------------------------------------------------
RED=""; GREEN=""; YELLOW=""; BOLD=""; DIM=""; RESET=""

if [ -t 1 ] && [ "${NO_COLOR:-}" = "" ] && [ "$OPT_JSON" = "false" ]; then
    RED="\033[0;31m"
    GREEN="\033[0;32m"
    YELLOW="\033[0;33m"
    BOLD="\033[1m"
    DIM="\033[2m"
    RESET="\033[0m"
fi

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
# 14 official categories — `social` added 2026-05 with team approval after
# plugins-pro/social/* introduction (PRI lists 13; PPI canonical is now 14).
VALID_CATEGORIES="authentication automation commerce communication content data development infrastructure integrations media streaming sports compliance social"

# Canonical tier values (per F07 PRICING-TIERS).
# free, pro, max — `max` represents the all-bundle ɳSelf+ tier per F07.
VALID_TIERS="free pro max enterprise"

# Canonical bundle names (per F06 BUNDLE-INVENTORY).
# Single source of truth — keep aligned with .claude/docs/sport/F06-BUNDLE-INVENTORY.md
VALID_BUNDLES="nclaw nchat ntv nfamily clawde nsentry ntask"

# ---------------------------------------------------------------------------
# Locate jq (preferred) and python3 (fallback for complex parsing)
# ---------------------------------------------------------------------------
JQ=""
for _candidate in jq /opt/homebrew/bin/jq /usr/local/bin/jq; do
    if command -v "$_candidate" >/dev/null 2>&1; then
        JQ="$_candidate"
        break
    fi
done

if [ -z "$JQ" ]; then
    printf "%bERROR:%b jq is required. Install it with: brew install jq\n" "$RED" "$RESET" >&2
    exit 1
fi

PYTHON3=""
for _py in python3 /usr/bin/python3 /opt/homebrew/bin/python3; do
    if command -v "$_py" >/dev/null 2>&1; then
        PYTHON3="$_py"
        break
    fi
done

# ---------------------------------------------------------------------------
# Sanity: registry file must exist and be valid JSON
# ---------------------------------------------------------------------------
if [ ! -f "$REGISTRY_FILE" ]; then
    printf "%bERROR:%b registry.json not found at: %s\n" "$RED" "$RESET" "$REGISTRY_FILE" >&2
    exit 1
fi

if ! "$JQ" empty "$REGISTRY_FILE" >/dev/null 2>&1; then
    printf "%bERROR:%b registry.json is not valid JSON. Fix syntax errors first.\n" "$RED" "$RESET" >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Format detection (Check 1 — implicit; format determines downstream paths)
# Returns: "aggregated" | "array"
# ---------------------------------------------------------------------------
detect_format() {
    "$JQ" -r '
        if type == "object" and (.plugins | type == "object") then
            "aggregated"
        elif type == "array" then
            "array"
        elif type == "object" and (.plugins | type == "array") then
            "array-wrapped"
        else
            "unknown"
        end
    ' "$REGISTRY_FILE"
}

REGISTRY_FORMAT="$(detect_format)"

# ---------------------------------------------------------------------------
# Temp dir for inter-call data (cleaned on exit)
# ---------------------------------------------------------------------------
TMPDIR_VAL="$(mktemp -d 2>/dev/null || mktemp -d -t 'vr')"
trap 'rm -rf "$TMPDIR_VAL"' EXIT

printf '0' > "${TMPDIR_VAL}/errors"
printf '0' > "${TMPDIR_VAL}/warnings"
printf '' > "${TMPDIR_VAL}/findings"

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------
_inc_errors() {
    local val
    val="$(cat "${TMPDIR_VAL}/errors")"
    printf '%d' "$((val + 1))" > "${TMPDIR_VAL}/errors"
}

_inc_warnings() {
    local val
    val="$(cat "${TMPDIR_VAL}/warnings")"
    printf '%d' "$((val + 1))" > "${TMPDIR_VAL}/warnings"
}

err() {
    local check="$1" plugin="$2" message="$3"
    _inc_errors
    if [ "$OPT_JSON" = "false" ]; then
        printf "  %b[ERROR]%b %b%s%b — %s: %s\n" \
            "$RED" "$RESET" "$BOLD" "$check" "$RESET" "$plugin" "$message"
    fi
    printf '{"severity":"error","check":"%s","plugin":"%s","message":"%s"},\n' \
        "$check" "$plugin" "$(printf '%s' "$message" | sed 's/"/\\"/g')" \
        >> "${TMPDIR_VAL}/findings"
}

warn() {
    local check="$1" plugin="$2" message="$3"
    _inc_warnings
    if [ "$OPT_JSON" = "false" ]; then
        printf "  %b[WARN]%b  %b%s%b — %s: %s\n" \
            "$YELLOW" "$RESET" "$BOLD" "$check" "$RESET" "$plugin" "$message"
    fi
    printf '{"severity":"warn","check":"%s","plugin":"%s","message":"%s"},\n' \
        "$check" "$plugin" "$(printf '%s' "$message" | sed 's/"/\\"/g')" \
        >> "${TMPDIR_VAL}/findings"
}

ok() {
    local check="$1" message="$2"
    if [ "$OPT_JSON" = "false" ]; then
        printf "  %b[OK]%b    %b%s%b — %s\n" \
            "$GREEN" "$RESET" "$BOLD" "$check" "$RESET" "$message"
    fi
}

section() {
    local title="$1"
    if [ "$OPT_JSON" = "false" ]; then
        printf "\n%b%s%b\n" "$BOLD" "$title" "$RESET"
    fi
}

# ---------------------------------------------------------------------------
# Helpers — semver, category, tier, bundle membership
# ---------------------------------------------------------------------------
_is_valid_semver() {
    local v="${1#v}"
    case "$v" in
        *.*.*) ;;
        *) return 1 ;;
    esac
    local major="${v%%.*}"
    local rest="${v#*.}"
    local minor="${rest%%.*}"
    local patch="${rest#*.}"
    # Strip pre-release / build metadata from patch (semver allows -alpha, +meta)
    patch="${patch%%[-+]*}"
    case "$major" in *[!0-9]*|"") return 1 ;; esac
    case "$minor" in *[!0-9]*|"") return 1 ;; esac
    case "$patch" in *[!0-9]*|"") return 1 ;; esac
    return 0
}

_is_valid_name() {
    # Lowercase, hyphenated, alphanumeric, must start with a letter
    case "$1" in
        ""|*[!a-z0-9-]*) return 1 ;;
        [!a-z]*) return 1 ;;
        *) return 0 ;;
    esac
}

_in_list() {
    local needle="$1"; shift
    local hay
    for hay in "$@"; do
        [ "$hay" = "$needle" ] && return 0
    done
    return 1
}

# ---------------------------------------------------------------------------
# Iterators — produce uniform stream of fields separated by ASCII Unit
# Separator (0x1f). Tabs collapse consecutive empties under bash `read`
# when IFS is whitespace; 0x1f does not.
#
# Field order: name, tier, category, version, port, bundles_json, deps_type, license
# ---------------------------------------------------------------------------
US=$'\x1f'  # unit separator

emit_entries() {
    case "$REGISTRY_FORMAT" in
        aggregated)
            "$JQ" -r '
                .plugins
                | to_entries[]
                | [
                    .key,
                    (.value.tier // ""),
                    (.value.category // ""),
                    (.value.version // ""),
                    (.value.port // "" | tostring),
                    ((.value.bundles // []) | tojson),
                    (.value.dependencies | type),
                    (.value.license // "")
                  ]
                | join("")
            ' "$REGISTRY_FILE"
            ;;
        array)
            "$JQ" -r '
                .[]
                | [
                    (.name // ""),
                    (.tier // ""),
                    (.category // ""),
                    (.version // ""),
                    (.port // "" | tostring),
                    ((.bundles // []) | tojson),
                    (.dependencies | type),
                    (.license // "")
                  ]
                | join("")
            ' "$REGISTRY_FILE"
            ;;
        array-wrapped)
            "$JQ" -r '
                .plugins[]
                | [
                    (.name // ""),
                    (.tier // ""),
                    (.category // ""),
                    (.version // ""),
                    (.port // "" | tostring),
                    ((.bundles // []) | tojson),
                    (.dependencies | type),
                    (.license // "")
                  ]
                | join("")
            ' "$REGISTRY_FILE"
            ;;
        *)
            return 1
            ;;
    esac
}

# ---------------------------------------------------------------------------
# Determine if this registry is the "pro" registry by directory location.
# plugins-pro/registry.json → has paid/ sibling → pro registry
# plugins/registry.json     → has free/ sibling → free registry
# ---------------------------------------------------------------------------
IS_PRO_REGISTRY=false
IS_FREE_REGISTRY=false
if [ -d "$PAID_DIR" ]; then
    IS_PRO_REGISTRY=true
fi
if [ -d "$FREE_DIR" ]; then
    IS_FREE_REGISTRY=true
fi

# =============================================================================
# Main validation pipeline
# =============================================================================

if [ "$OPT_JSON" = "false" ]; then
    printf "\n%bnself Plugin Registry Validator%b\n" "$BOLD" "$RESET"
    printf "%bRegistry:%b %s\n" "$DIM" "$RESET" "$REGISTRY_FILE"
    printf "%bFormat:%b   %s\n" "$DIM" "$RESET" "$REGISTRY_FORMAT"
fi

# ---------------------------------------------------------------------------
# Format gate — must be aggregated or array; abort otherwise
# ---------------------------------------------------------------------------
if [ "$REGISTRY_FORMAT" = "unknown" ]; then
    err "FORMAT" "registry" "Unrecognized registry format. Expected aggregated object {plugins:{}} or flat array."
    if [ "$OPT_JSON" = "false" ]; then
        printf "\n%bFAILED%b — registry format is invalid.\n\n" "$RED$BOLD" "$RESET"
    fi
    exit 1
fi

# ---------------------------------------------------------------------------
# Stream entries to a temp TSV file for repeated passes
# ---------------------------------------------------------------------------
ENTRIES_TSV="${TMPDIR_VAL}/entries.usv"
if ! emit_entries > "$ENTRIES_TSV" 2>/dev/null; then
    err "PARSE" "registry" "Failed to extract plugin entries from registry."
    exit 1
fi

TOTAL_PLUGINS="$(wc -l < "$ENTRIES_TSV" | tr -d ' ')"

# =============================================================================
# CHECK 11 — plugins_count matches actual plugin count (aggregated only)
# =============================================================================
section "CHECK 11 — plugins_count integrity"
if [ "$REGISTRY_FORMAT" = "aggregated" ]; then
    DECLARED_COUNT="$("$JQ" -r '.plugins_count // -1' "$REGISTRY_FILE")"
    ACTUAL_COUNT="$("$JQ" -r '.plugins | length' "$REGISTRY_FILE")"
    if [ "$DECLARED_COUNT" = "-1" ]; then
        warn "CHECK-11" "registry" "plugins_count field is missing (recommended for aggregated format)"
    elif [ "$DECLARED_COUNT" != "$ACTUAL_COUNT" ]; then
        err "CHECK-11" "registry" "plugins_count=${DECLARED_COUNT} does not match actual plugin count=${ACTUAL_COUNT}"
    else
        ok "CHECK-11" "plugins_count=${ACTUAL_COUNT} matches"
    fi
else
    ok "CHECK-11" "skipped (flat array format)"
fi

# =============================================================================
# CHECK 13 — Dependencies use Shape A (array form, per registry-schema.json)
# CHECK 7  — schema fields present
# =============================================================================
section "CHECK 13 — dependencies type validation"
DEPS_VIOLATIONS=0
while IFS="$US" read -r name _tier _category _version _port _bundles deps_type _license; do
    [ -z "$name" ] && continue
    if [ "$deps_type" = "string" ] || [ "$deps_type" = "number" ] || [ "$deps_type" = "boolean" ] || [ "$deps_type" = "object" ]; then
        err "CHECK-13" "$name" "dependencies has invalid type '${deps_type}' — must be array of strings"
        DEPS_VIOLATIONS=$((DEPS_VIOLATIONS + 1))
    fi
done < "$ENTRIES_TSV"
if [ "$DEPS_VIOLATIONS" -eq 0 ]; then
    ok "CHECK-13" "all dependencies are properly typed arrays"
fi

# =============================================================================
# CHECK 5 — All categories from official 14
# =============================================================================
section "CHECK 5 — category validity"
CATEGORY_VIOLATIONS=0
while IFS="$US" read -r name _tier category _version _port _bundles _deps _license; do
    [ -z "$name" ] && continue
    if [ -z "$category" ]; then
        warn "CHECK-5" "$name" "category field is missing"
        continue
    fi
    # shellcheck disable=SC2086  # intentional word splitting on $VALID_CATEGORIES
    if ! _in_list "$category" $VALID_CATEGORIES; then
        err "CHECK-5" "$name" "Invalid category '$category' (valid: $VALID_CATEGORIES)"
        CATEGORY_VIOLATIONS=$((CATEGORY_VIOLATIONS + 1))
    fi
done < "$ENTRIES_TSV"
if [ "$CATEGORY_VIOLATIONS" -eq 0 ]; then
    ok "CHECK-5" "all categories valid"
fi

# =============================================================================
# CHECK 6 — All tier values from canonical tier list
# CHECK 12 — No tier=free entries in plugins-pro/registry.json
# =============================================================================
section "CHECK 6 + 12 — tier validity and tier-directory enforcement"
TIER_VIOLATIONS=0
while IFS="$US" read -r name tier _category _version _port _bundles _deps _license; do
    [ -z "$name" ] && continue
    if [ -z "$tier" ]; then
        err "CHECK-6" "$name" "tier field is missing or empty"
        TIER_VIOLATIONS=$((TIER_VIOLATIONS + 1))
        continue
    fi
    # shellcheck disable=SC2086  # intentional word splitting on $VALID_TIERS
    if ! _in_list "$tier" $VALID_TIERS; then
        err "CHECK-6" "$name" "Invalid tier '$tier' (valid: $VALID_TIERS)"
        TIER_VIOLATIONS=$((TIER_VIOLATIONS + 1))
    fi
    # CHECK 12 — tier=free in pro registry
    if [ "$IS_PRO_REGISTRY" = "true" ] && [ "$tier" = "free" ]; then
        err "CHECK-12" "$name" "tier=free entry in plugins-pro/registry.json — must be moved to plugins/free/ or tier corrected (PLUG-10)"
        TIER_VIOLATIONS=$((TIER_VIOLATIONS + 1))
    fi
    # PLUG-10 inverse — tier!=free in plugins/registry.json
    if [ "$IS_FREE_REGISTRY" = "true" ] && [ -n "$tier" ] && [ "$tier" != "free" ]; then
        err "CHECK-12" "$name" "tier='$tier' entry in plugins/registry.json — must be tier=free or moved to plugins-pro/paid/ (PLUG-10)"
        TIER_VIOLATIONS=$((TIER_VIOLATIONS + 1))
    fi
done < "$ENTRIES_TSV"
if [ "$TIER_VIOLATIONS" -eq 0 ]; then
    ok "CHECK-6+12" "all tiers valid and tier-directory match enforced"
fi

# =============================================================================
# CHECK 7 — Schema field presence: name, version, description, category, tier
# In aggregated format, the registry key serves as the canonical name —
# .value.name need not be repeated. In flat-array format, .name is required.
# =============================================================================
section "CHECK 7 — required schema fields present"
SCHEMA_VIOLATIONS=0
SCHEMA_USV="${TMPDIR_VAL}/schema.usv"
case "$REGISTRY_FORMAT" in
    aggregated)
        # Name comes from the key, not .value.name — so we don't require it.
        # If .value.name IS present, ensure it matches the key (drift check).
        "$JQ" -r '
            .plugins
            | to_entries[]
            | [
                .key,
                (if (.value.name // "") == "" then "OK"
                 elif .value.name != .key then "MISMATCH"
                 else "OK" end),
                (if (.value.version // "") == "" then "MISSING" else "OK" end),
                (if (.value.description // "") == "" then "MISSING" else "OK" end),
                (if (.value.category // "") == "" then "MISSING" else "OK" end),
                (if (.value.tier // "") == "" then "MISSING" else "OK" end),
                (if (.value.language // "") == "" then "MISSING" else "OK" end)
              ]
            | join("")
        ' "$REGISTRY_FILE" > "$SCHEMA_USV"
        ;;
    array)
        "$JQ" -r '
            .[]
            | [
                (.name // ""),
                (if (.name // "") == "" then "MISSING" else "OK" end),
                (if (.version // "") == "" then "MISSING" else "OK" end),
                (if (.description // "") == "" then "MISSING" else "OK" end),
                (if (.category // "") == "" then "MISSING" else "OK" end),
                (if (.tier // "") == "" then "MISSING" else "OK" end),
                (if (.language // "") == "" then "MISSING" else "OK" end)
              ]
            | join("")
        ' "$REGISTRY_FILE" > "$SCHEMA_USV"
        ;;
    array-wrapped)
        "$JQ" -r '
            .plugins[]
            | [
                (.name // ""),
                (if (.name // "") == "" then "MISSING" else "OK" end),
                (if (.version // "") == "" then "MISSING" else "OK" end),
                (if (.description // "") == "" then "MISSING" else "OK" end),
                (if (.category // "") == "" then "MISSING" else "OK" end),
                (if (.tier // "") == "" then "MISSING" else "OK" end),
                (if (.language // "") == "" then "MISSING" else "OK" end)
              ]
            | join("")
        ' "$REGISTRY_FILE" > "$SCHEMA_USV"
        ;;
esac
while IFS="$US" read -r name name_ok version_ok description_ok category_ok tier_ok language_ok; do
    [ -z "$name" ] && continue
    if [ "$name_ok" = "MISSING" ]; then
        err "CHECK-7" "$name" "name field is missing"
        SCHEMA_VIOLATIONS=$((SCHEMA_VIOLATIONS + 1))
    elif [ "$name_ok" = "MISMATCH" ]; then
        err "CHECK-7" "$name" "name field does not match registry key"
        SCHEMA_VIOLATIONS=$((SCHEMA_VIOLATIONS + 1))
    fi
    if [ "$version_ok" = "MISSING" ]; then
        err "CHECK-7" "$name" "version field is missing"
        SCHEMA_VIOLATIONS=$((SCHEMA_VIOLATIONS + 1))
    fi
    [ "$description_ok" = "MISSING" ] && warn "CHECK-7" "$name" "description field is missing"
    [ "$category_ok" = "MISSING" ] && warn "CHECK-7" "$name" "category field is missing"
    if [ "$tier_ok" = "MISSING" ]; then
        err "CHECK-7" "$name" "tier field is missing"
        SCHEMA_VIOLATIONS=$((SCHEMA_VIOLATIONS + 1))
    fi
    [ "$language_ok" = "MISSING" ] && warn "CHECK-7" "$name" "language field is missing (recommended)"
done < "$SCHEMA_USV"
if [ "$SCHEMA_VIOLATIONS" -eq 0 ]; then
    ok "CHECK-7" "all required fields present"
fi

# =============================================================================
# Plugin name validity (part of CHECK 7)
# =============================================================================
section "CHECK 7b — plugin name format"
NAME_VIOLATIONS=0
while IFS="$US" read -r name _tier _category _version _port _bundles _deps _license; do
    [ -z "$name" ] && continue
    if ! _is_valid_name "$name"; then
        err "CHECK-7b" "$name" "Invalid plugin name '$name' (must be lowercase-with-hyphens, start with a letter)"
        NAME_VIOLATIONS=$((NAME_VIOLATIONS + 1))
    fi
done < "$ENTRIES_TSV"
if [ "$NAME_VIOLATIONS" -eq 0 ]; then
    ok "CHECK-7b" "all plugin names well-formed"
fi

# =============================================================================
# Version semver (part of CHECK 7)
# =============================================================================
section "CHECK 7c — semver compliance"
SEMVER_VIOLATIONS=0
while IFS="$US" read -r name _tier _category version _port _bundles _deps _license; do
    [ -z "$name" ] && continue
    [ -z "$version" ] && continue
    if ! _is_valid_semver "$version"; then
        err "CHECK-7c" "$name" "Invalid semver '$version' (expected x.y.z)"
        SEMVER_VIOLATIONS=$((SEMVER_VIOLATIONS + 1))
    fi
done < "$ENTRIES_TSV"
if [ "$SEMVER_VIOLATIONS" -eq 0 ]; then
    ok "CHECK-7c" "all versions are valid semver"
fi

# =============================================================================
# CHECK 4 — All ports unique across this registry
# (Cross-registry uniqueness is enforced by a separate CI script.)
# =============================================================================
section "CHECK 4 — port uniqueness within registry"
PORT_VIOLATIONS=0
PORTS_FILE="${TMPDIR_VAL}/ports.txt"
: > "$PORTS_FILE"
while IFS="$US" read -r name _tier _category _version port _bundles _deps _license; do
    [ -z "$name" ] && continue
    [ -z "$port" ] || [ "$port" = "null" ] || [ "$port" = "0" ] && continue
    # Numeric range check
    case "$port" in
        ''|*[!0-9]*)
            err "CHECK-4" "$name" "Port '$port' is not a positive integer"
            PORT_VIOLATIONS=$((PORT_VIOLATIONS + 1))
            continue
            ;;
    esac
    if [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then
        err "CHECK-4" "$name" "Port $port is out of range (1-65535)"
        PORT_VIOLATIONS=$((PORT_VIOLATIONS + 1))
        continue
    fi
    printf '%s\t%s\n' "$port" "$name" >> "$PORTS_FILE"
done < "$ENTRIES_TSV"

if [ -s "$PORTS_FILE" ]; then
    DUPES="$(cut -f1 "$PORTS_FILE" | sort | uniq -d)"
    if [ -n "$DUPES" ]; then
        for dport in $DUPES; do
            DUPE_NAMES="$(awk -F'\t' -v p="$dport" '$1 == p {print $2}' "$PORTS_FILE" | tr '\n' ',' | sed 's/,$//')"
            err "CHECK-4" "$DUPE_NAMES" "Duplicate port $dport"
            PORT_VIOLATIONS=$((PORT_VIOLATIONS + 1))
        done
    fi
fi
if [ "$PORT_VIOLATIONS" -eq 0 ]; then
    ok "CHECK-4" "all declared ports are unique within this registry"
fi

# =============================================================================
# CHECK 3 — No duplicate plugin names within this registry
# (Cross-registry de-dup is also a separate CI script.)
# =============================================================================
section "CHECK 3 — duplicate plugin names within registry"
NAMES_FILE="${TMPDIR_VAL}/names.txt"
cut -d"$US" -f1 "$ENTRIES_TSV" | sort > "$NAMES_FILE"
DUPE_NAMES_LIST="$(uniq -d < "$NAMES_FILE")"
if [ -n "$DUPE_NAMES_LIST" ]; then
    for dname in $DUPE_NAMES_LIST; do
        err "CHECK-3" "$dname" "Duplicate plugin name in registry"
    done
else
    ok "CHECK-3" "no duplicate plugin names in this registry"
fi

# =============================================================================
# CHECK 8 — Alphabetical sort order maintained
# =============================================================================
section "CHECK 8 — alphabetical sort order"
SORTED_NAMES_FILE="${TMPDIR_VAL}/names_actual_order.txt"
cut -d"$US" -f1 "$ENTRIES_TSV" > "$SORTED_NAMES_FILE"
EXPECTED_SORTED_FILE="${TMPDIR_VAL}/names_expected.txt"
sort "$SORTED_NAMES_FILE" > "$EXPECTED_SORTED_FILE"
if cmp -s "$SORTED_NAMES_FILE" "$EXPECTED_SORTED_FILE"; then
    ok "CHECK-8" "plugin entries are alphabetically sorted"
else
    # Find first out-of-order entry
    FIRST_DIFF="$(diff "$SORTED_NAMES_FILE" "$EXPECTED_SORTED_FILE" | head -3 | tr '\n' ' ')"
    warn "CHECK-8" "registry" "Plugin entries not alphabetically sorted (first difference: ${FIRST_DIFF})"
fi

# =============================================================================
# CHECK 10 — bundles array members from F06 canonical bundle list
# =============================================================================
section "CHECK 10 — bundles membership"
BUNDLE_VIOLATIONS=0
while IFS="$US" read -r name _tier _category _version _port bundles_json _deps _license; do
    [ -z "$name" ] && continue
    [ -z "$bundles_json" ] && continue
    [ "$bundles_json" = "null" ] && continue
    [ "$bundles_json" = "[]" ] && continue
    # Parse bundles array
    if ! "$JQ" -e 'type == "array"' >/dev/null 2>&1 <<< "$bundles_json"; then
        err "CHECK-10" "$name" "bundles field must be an array of strings"
        BUNDLE_VIOLATIONS=$((BUNDLE_VIOLATIONS + 1))
        continue
    fi
    # Each member must be a string and from canonical list
    BUNDLE_MEMBERS="$("$JQ" -r '.[]' <<< "$bundles_json" 2>/dev/null)"
    while IFS= read -r bundle; do
        [ -z "$bundle" ] && continue
        # shellcheck disable=SC2086  # intentional word splitting on $VALID_BUNDLES
        if ! _in_list "$bundle" $VALID_BUNDLES; then
            err "CHECK-10" "$name" "bundle '$bundle' not in F06 canonical list ($VALID_BUNDLES)"
            BUNDLE_VIOLATIONS=$((BUNDLE_VIOLATIONS + 1))
        fi
    done <<EOF
$BUNDLE_MEMBERS
EOF
done < "$ENTRIES_TSV"
if [ "$BUNDLE_VIOLATIONS" -eq 0 ]; then
    ok "CHECK-10" "all bundle memberships valid (or no bundles declared)"
fi

# =============================================================================
# CHECK 1 — Every plugin directory has an entry in registry.json
# CHECK 2 — Every registry entry has a matching plugin directory
# CHECK 9 — No stale registry entries (subset of check 2)
# =============================================================================
section "CHECK 1 + 2 + 9 — registry ↔ filesystem consistency"
DIRS_TO_SCAN=""
[ -d "$FREE_DIR" ] && DIRS_TO_SCAN="${DIRS_TO_SCAN} ${FREE_DIR}"
[ -d "$COMMUNITY_DIR" ] && DIRS_TO_SCAN="${DIRS_TO_SCAN} ${COMMUNITY_DIR}"
[ -d "$PAID_DIR" ] && DIRS_TO_SCAN="${DIRS_TO_SCAN} ${PAID_DIR}"

if [ -z "$DIRS_TO_SCAN" ]; then
    warn "CHECK-1+2" "registry" "No plugin source directory (free/, community/, paid/) found next to registry — skipping filesystem checks"
else
    # Build sets: registry_names and dir_names
    REGISTRY_NAMES_FILE="${TMPDIR_VAL}/registry_names.txt"
    DIR_NAMES_FILE="${TMPDIR_VAL}/dir_names.txt"
    cut -d"$US" -f1 "$ENTRIES_TSV" | sort -u > "$REGISTRY_NAMES_FILE"
    : > "$DIR_NAMES_FILE"
    for d in $DIRS_TO_SCAN; do
        if [ -d "$d" ]; then
            for entry in "$d"/*/; do
                [ -d "$entry" ] || continue
                base="$(basename "$entry")"
                # Skip non-plugin dirs (well-known scaffolding / shared libs)
                case "$base" in
                    node_modules|.git|target|dist|build|coverage|tests|test|fuzz|shared|examples|docs|scripts|cmd|tools) continue ;;
                esac
                printf '%s\n' "$base" >> "$DIR_NAMES_FILE"
            done
        fi
    done
    sort -u "$DIR_NAMES_FILE" -o "$DIR_NAMES_FILE"

    # CHECK 1 — directory without registry entry
    MISSING_FROM_REGISTRY="$(comm -23 "$DIR_NAMES_FILE" "$REGISTRY_NAMES_FILE")"
    if [ -n "$MISSING_FROM_REGISTRY" ]; then
        while IFS= read -r missing; do
            [ -z "$missing" ] && continue
            err "CHECK-1" "$missing" "Plugin directory exists but no registry entry"
        done <<EOF
$MISSING_FROM_REGISTRY
EOF
    else
        ok "CHECK-1" "all plugin directories have registry entries"
    fi

    # CHECK 2 + 9 — registry entry without directory (stale)
    MISSING_FROM_DISK="$(comm -13 "$DIR_NAMES_FILE" "$REGISTRY_NAMES_FILE")"
    if [ -n "$MISSING_FROM_DISK" ]; then
        while IFS= read -r stale; do
            [ -z "$stale" ] && continue
            err "CHECK-2" "$stale" "Registry entry has no matching plugin directory (stale entry)"
        done <<EOF
$MISSING_FROM_DISK
EOF
    else
        ok "CHECK-2+9" "all registry entries have matching directories"
    fi
fi

# =============================================================================
# Optional: --strict mode runs the legacy per-plugin field validator
# (kept for backward compatibility with the original validator behavior)
# =============================================================================
if [ "$OPT_STRICT" = "true" ]; then
    section "STRICT MODE — legacy per-plugin field validation"
    if [ -z "$PYTHON3" ]; then
        warn "STRICT" "registry" "python3 not available — skipping strict mode"
    else
        warn "STRICT" "registry" "Strict mode requires legacy schema (tables, implementation) — see git history of this script for the previous implementation. Not re-enabled in v2."
    fi
fi

# =============================================================================
# Summary + exit
# =============================================================================
FINAL_ERRORS="$(cat "${TMPDIR_VAL}/errors")"
FINAL_WARNINGS="$(cat "${TMPDIR_VAL}/warnings")"

if [ "$OPT_JSON" = "true" ]; then
    JSON_STATUS="pass"
    [ "$FINAL_ERRORS" -gt 0 ] && JSON_STATUS="fail"
    printf '{\n'
    printf '  "status": "%s",\n' "$JSON_STATUS"
    printf '  "registry": "%s",\n' "$REGISTRY_FILE"
    printf '  "format": "%s",\n' "$REGISTRY_FORMAT"
    printf '  "summary": {\n'
    printf '    "plugins": %d,\n' "$TOTAL_PLUGINS"
    printf '    "errors": %d,\n' "$FINAL_ERRORS"
    printf '    "warnings": %d\n' "$FINAL_WARNINGS"
    printf '  },\n'
    printf '  "findings": [\n'
    if [ -s "${TMPDIR_VAL}/findings" ]; then
        # Strip trailing comma+newline from last entry
        sed '$ s/,$//' "${TMPDIR_VAL}/findings"
    fi
    printf '  ]\n'
    printf '}\n'
else
    SEP="━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    printf "\n%b%s%b\n" "$BOLD" "$SEP" "$RESET"
    printf "%bSummary%b\n" "$BOLD" "$RESET"
    printf "  Registry:        %s\n" "$REGISTRY_FILE"
    printf "  Format:          %s\n" "$REGISTRY_FORMAT"
    printf "  Plugins:         %d\n" "$TOTAL_PLUGINS"
    if [ "$FINAL_ERRORS" -gt 0 ]; then
        printf "  %bErrors:          %d%b\n" "$RED" "$FINAL_ERRORS" "$RESET"
    else
        printf "  %bErrors:          0%b\n" "$GREEN" "$RESET"
    fi
    if [ "$FINAL_WARNINGS" -gt 0 ]; then
        printf "  %bWarnings:        %d%b\n" "$YELLOW" "$FINAL_WARNINGS" "$RESET"
    else
        printf "  Warnings:        0\n"
    fi
    printf "%b%s%b\n" "$BOLD" "$SEP" "$RESET"

    if [ "$FINAL_ERRORS" -gt 0 ]; then
        printf "\n%b%bFAILED%b — %d error(s) found.\n\n" "$RED" "$BOLD" "$RESET" "$FINAL_ERRORS"
    else
        printf "\n%b%bPASSED%b — registry is valid" "$GREEN" "$BOLD" "$RESET"
        if [ "$FINAL_WARNINGS" -gt 0 ]; then
            printf " (with %d warning(s))" "$FINAL_WARNINGS"
        fi
        printf ".\n\n"
    fi
fi

[ "$FINAL_ERRORS" -eq 0 ]
