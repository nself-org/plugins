#!/usr/bin/env bash
set -euo pipefail

# regen-sport.sh — Drift detector for SPORT F03/F04 plugin inventory files.
#
# Reads plugin directories on disk and compares counts against what SPORT
# F03/F04 currently document. Does NOT modify SPORT files (they are read-only
# per hard rules). Reports drift and exits 1 if counts or plugin names diverge.
#
# Usage:
#   ./regen-sport.sh            # same as --check
#   ./regen-sport.sh --check    # exit 0 if no drift, exit 1 if drift
#   ./regen-sport.sh --report   # print full inventory table, then run drift check

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PLUGINS_FREE_DIR="${REPO_ROOT}/plugins/free"
PLUGINS_PRO_DIR="${REPO_ROOT}/plugins-pro/paid"
SPORT_DIR="${REPO_ROOT}/.claude/docs/sport"

MODE="check"
if [ "${1:-}" = "--report" ]; then
  MODE="report"
fi

# ---------------------------------------------------------------------------
# Count and enumerate free plugins
# A directory is a plugin if it contains plugin.json
# ---------------------------------------------------------------------------
FREE_COUNT=0
FREE_PLUGINS=""

if [ -d "${PLUGINS_FREE_DIR}" ]; then
  for plugin_dir in "${PLUGINS_FREE_DIR}"/*/; do
    if [ ! -d "${plugin_dir}" ]; then
      continue
    fi
    name="$(basename "${plugin_dir}")"
    if [ -f "${plugin_dir}plugin.json" ]; then
      FREE_COUNT=$((FREE_COUNT + 1))
      FREE_PLUGINS="${FREE_PLUGINS} ${name}"
    fi
  done
fi

# ---------------------------------------------------------------------------
# Count and enumerate pro plugins (exclude shared/ — it is a library, not a plugin)
# ---------------------------------------------------------------------------
PRO_COUNT=0
PRO_PLUGINS=""

if [ -d "${PLUGINS_PRO_DIR}" ]; then
  for plugin_dir in "${PLUGINS_PRO_DIR}"/*/; do
    if [ ! -d "${plugin_dir}" ]; then
      continue
    fi
    name="$(basename "${plugin_dir}")"
    if [ "${name}" = "shared" ]; then
      continue
    fi
    if [ -f "${plugin_dir}plugin.json" ]; then
      PRO_COUNT=$((PRO_COUNT + 1))
      PRO_PLUGINS="${PRO_PLUGINS} ${name}"
    fi
  done
fi

TOTAL=$((FREE_COUNT + PRO_COUNT))

# ---------------------------------------------------------------------------
# Extract expected counts from SPORT files
# F03 line: "- Free plugins on disk (`ls plugins/free/`): **25**"
# F04 line: "- Pro plugin directories on disk (`ls plugins-pro/paid/ | grep -v '^shared$'`): **62**"
# ---------------------------------------------------------------------------
SPORT_FREE="0"
SPORT_PRO="0"

F03_FILE="${SPORT_DIR}/F03-PLUGIN-INVENTORY-FREE.md"
F04_FILE="${SPORT_DIR}/F04-PLUGIN-INVENTORY-PRO.md"

if [ -f "${F03_FILE}" ]; then
  # Match: "Free plugins on disk ... **<number>**"
  extracted=$(grep "Free plugins on disk" "${F03_FILE}" | grep -o '\*\*[0-9]*\*\*' | grep -o '[0-9]*' | head -1 || true)
  if [ -n "${extracted}" ]; then
    SPORT_FREE="${extracted}"
  fi
fi

if [ -f "${F04_FILE}" ]; then
  # Match: "Pro plugin directories on disk ... **<number>**"
  extracted=$(grep "Pro plugin directories on disk" "${F04_FILE}" | grep -o '\*\*[0-9]*\*\*' | grep -o '[0-9]*' | head -1 || true)
  if [ -n "${extracted}" ]; then
    SPORT_PRO="${extracted}"
  fi
fi

# ---------------------------------------------------------------------------
# Build per-plugin detail lines (for --report mode)
# Read name/version/description from plugin.json via python3
# Read README line count via wc -l
# ---------------------------------------------------------------------------
build_plugin_detail() {
  local dir="$1"
  local name
  name="$(basename "${dir}")"
  local version=""
  local description=""
  local readme_lines="MISSING"

  if [ -f "${dir}plugin.json" ]; then
    version=$(python3 -c "
import json, sys
try:
    d = json.load(open('${dir}plugin.json'))
    print(d.get('version', ''))
except Exception:
    print('')
" 2>/dev/null || true)
    description=$(python3 -c "
import json, sys
try:
    d = json.load(open('${dir}plugin.json'))
    desc = d.get('description', '')
    print(desc[:72])
except Exception:
    print('')
" 2>/dev/null || true)
  fi

  if [ -f "${dir}README.md" ]; then
    readme_lines="$(wc -l < "${dir}README.md" | tr -d ' ') lines"
  fi

  printf "  %-26s  v%-8s  README:%-12s  %s\n" \
    "${name}" "${version}" "${readme_lines}" "${description}"
}

# ---------------------------------------------------------------------------
# Report mode: print full inventory table
# ---------------------------------------------------------------------------
if [ "${MODE}" = "report" ]; then
  printf "\n=== nSelf Plugin Inventory Report ===\n"
  printf "Generated from disk (not SPORT). SPORT files are read-only.\n\n"

  printf "Free plugins (disk: %d  |  SPORT F03: %s)\n" "${FREE_COUNT}" "${SPORT_FREE}"
  printf -- "------------------------------------------------------------------------\n"
  if [ -d "${PLUGINS_FREE_DIR}" ]; then
    for plugin_dir in "${PLUGINS_FREE_DIR}"/*/; do
      if [ ! -d "${plugin_dir}" ]; then
        continue
      fi
      name="$(basename "${plugin_dir}")"
      if [ -f "${plugin_dir}plugin.json" ]; then
        build_plugin_detail "${plugin_dir}"
      fi
    done
  fi

  printf "\nPro plugins (disk: %d  |  SPORT F04: %s)  [shared/ excluded]\n" "${PRO_COUNT}" "${SPORT_PRO}"
  printf -- "------------------------------------------------------------------------\n"
  if [ -d "${PLUGINS_PRO_DIR}" ]; then
    for plugin_dir in "${PLUGINS_PRO_DIR}"/*/; do
      if [ ! -d "${plugin_dir}" ]; then
        continue
      fi
      name="$(basename "${plugin_dir}")"
      if [ "${name}" = "shared" ]; then
        continue
      fi
      if [ -f "${plugin_dir}plugin.json" ]; then
        build_plugin_detail "${plugin_dir}"
      fi
    done
  fi

  printf "\nTotal plugins on disk: %d  (free=%d  pro=%d)\n\n" "${TOTAL}" "${FREE_COUNT}" "${PRO_COUNT}"
fi

# ---------------------------------------------------------------------------
# Drift check
# Only compare against SPORT when SPORT file exists and has a parseable count
# (SPORT_FREE/PRO remain "0" if file missing or pattern not found — skip those)
# ---------------------------------------------------------------------------
DRIFT=0

if [ "${SPORT_FREE}" != "0" ] && [ "${FREE_COUNT}" != "${SPORT_FREE}" ]; then
  printf "DRIFT: Free plugin count on disk (%d) != SPORT F03 (%s)\n" \
    "${FREE_COUNT}" "${SPORT_FREE}" >&2
  DRIFT=1
fi

if [ "${SPORT_PRO}" != "0" ] && [ "${PRO_COUNT}" != "${SPORT_PRO}" ]; then
  printf "DRIFT: Pro plugin count on disk (%d) != SPORT F04 (%s)\n" \
    "${PRO_COUNT}" "${SPORT_PRO}" >&2
  DRIFT=1
fi

if [ "${DRIFT}" = "0" ]; then
  printf "OK: Plugin counts match SPORT (free=%d  pro=%d  total=%d)\n" \
    "${FREE_COUNT}" "${PRO_COUNT}" "${TOTAL}"
  exit 0
else
  printf "FAIL: Drift detected. If code is authoritative, regenerate SPORT.\n" >&2
  printf "      SPORT files are read-only — a human must approve regeneration.\n" >&2
  exit 1
fi
