#!/usr/bin/env bash
# Purpose: the registry leg of tests/kind/run.sh (P7-DEPL-31): pick the k8s row out of the served
#   plugin registry, and assert that what `nself add k8s` installed is the served plugin, at the
#   checkout's version, with the chart-era commands. Sourced by run.sh and registry-test.sh; no
#   cluster, no docker and no network are needed by any function except registry_fetch.
# Inputs: function arguments (files and strings); NSELF_PLUGIN_REGISTRY (optional) for
#   registry_url; jq on PATH.
# Outputs: functions print the value asked for on stdout and return 0; a failed assertion prints
#   one "FAIL: registry leg: ..." line on stderr naming the served version and what is missing,
#   and returns 1.
# Constraints: never reads free/k8s sources (the checkout version is passed in by run.sh from
#   plugin.json); no secret on any argv; the registry URL is never a literal here, it is the
#   NSELF_PLUGIN_REGISTRY override or the default the cli at CLI_DIR was built with.

# Commands a chart-era k8s plugin (P7-DEPL-05..08) lists under `nself help --json k8s` and a
# pre-chart one (1.2.6 as served before P7-PLUG-86) does not.
CHART_ERA_COMMANDS="${CHART_ERA_COMMANDS:-values backup}"

# reg_fail <message>: report and return 1.
reg_fail() { echo "FAIL: registry leg: $*" >&2; return 1; }

# registry_url <cli_dir>: the registry the cli reads, from the override or the cli's own default.
registry_url() {
  local url="${NSELF_PLUGIN_REGISTRY:-}" src
  if [ -z "$url" ]; then
    src="$1/internal/plugin/registry.go"
    [ -f "$src" ] || { reg_fail "no NSELF_PLUGIN_REGISTRY and no $src to read the cli default from"; return 1; }
    url="$(sed -n 's/^[[:space:]]*DefaultRegistryURL[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' "$src" | head -n 1)"
    [ -n "$url" ] || { reg_fail "DefaultRegistryURL not found in $src"; return 1; }
  fi
  printf '%s\n' "$url"
}

# registry_fetch <url> <out file>: download the registry document (three tries).
registry_fetch() {
  curl -fsS --retry 3 --retry-delay 2 --max-time 60 -o "$2" "$1" || { reg_fail "could not fetch the registry $1"; return 1; }
  jq -e . "$2" >/dev/null || { reg_fail "the registry at $1 is not JSON"; return 1; }
}

# registry_row <registry.json> <plugin name>: the plugin's row. The registry is an array under
# .plugins (as served), a bare array, or a map keyed by name (as registry.json on main is
# generated). Fails when there is no row.
registry_row() {
  local row
  row="$(jq -c --arg n "$2" '
    (if type == "array" then . else (.plugins // .) end) as $p
    | first(if ($p | type) == "array" then ($p[] | select(.name == $n))
      elif ($p | type) == "object" then ($p[$n] // empty)
      else empty end)' "$1")" || { reg_fail "$1 is not a readable registry"; return 1; }
  [ -n "$row" ] || { reg_fail "the registry has no row for $2"; return 1; }
  printf '%s\n' "$row"
}

# registry_served_version <registry.json> <plugin name>: the served version.
registry_served_version() {
  local row v
  row="$(registry_row "$@")" || return 1
  v="$(printf '%s' "$row" | jq -r '.version // empty')"
  [ -n "$v" ] || { reg_fail "the registry row for $2 has no version"; return 1; }
  printf '%s\n' "$v"
}

# registry_platform_checksum <registry.json> <plugin name> <platform, e.g. linux-amd64>: the
# served checksum for that platform, or empty when the row carries none.
registry_platform_checksum() {
  local row
  row="$(registry_row "$1" "$2")" || return 1
  printf '%s' "$row" | jq -r --arg p "$3" '.checksums.platforms[$p] // empty'
}

# version_lt <a> <b>: true when a sorts before b.
version_lt() { [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n 1)" = "$1" ]; }

# assert_served_matches_checkout <served version> <checkout version>: the served plugin is the
# one this checkout would release.
assert_served_matches_checkout() {
  local served="$1" checkout="$2"
  [ "$served" = "$checkout" ] && return 0
  if version_lt "$served" "$checkout"; then
    reg_fail "the served k8s is $served, older than the checkout's $checkout: the release has not been served yet (P7-PLUG-86)"
  else
    reg_fail "the served k8s is $served, newer than the checkout's $checkout: rerun from a commit that includes that release"
  fi
}

# assert_installed_version <installed plugin.json> <served version> <checkout version>: the
# manifest `nself add` wrote carries the served row's version, which is the checkout's.
assert_installed_version() {
  local got
  got="$(jq -r '.version // empty' "$1" 2>/dev/null)" || got=""
  [ -n "$got" ] || { reg_fail "$1 has no readable version (served k8s $2)"; return 1; }
  if [ "$got" != "$2" ]; then
    reg_fail "the installed manifest says $got but the served row says $2"
    return 1
  fi
  assert_served_matches_checkout "$2" "$3"
}

# assert_chart_era <help --json k8s output file> <served version>: the installed plugin lists
# every command in CHART_ERA_COMMANDS. The served k8s 1.2.6 has the same version as the checkout
# but pre-chart bytes, so the version check alone would pass.
assert_chart_era() {
  local paths c missing=""
  paths="$(jq -er '.data.commands | if type == "array" then .[].path | sub("^nself "; "") else error("no commands") end' "$1" 2>/dev/null)" ||
    { reg_fail "help --json k8s of the served k8s $2 is not a command listing"; return 1; }
  for c in $CHART_ERA_COMMANDS; do
    printf '%s\n' "$paths" | grep -qxF "k8s $c" || missing="$missing $c"
  done
  if [ -n "$missing" ]; then
    reg_fail "the served k8s $2 predates the chart: nself help --json k8s lacks:$missing (the served bytes are older than P7-DEPL-05..08; release P7-PLUG-86 serves them)"
    return 1
  fi
}

# assert_empty_plugin_dir <plugin dir>: nothing is installed yet, so a passing leg proves
# `nself add` laid the plugin out and not a branch build.
assert_empty_plugin_dir() {
  if [ -d "$1" ] && [ -n "$(ls -A "$1" 2>/dev/null)" ]; then
    reg_fail "the plugin dir $1 is not empty before nself add k8s: $(find "$1" -mindepth 1 -maxdepth 1 -exec basename {} \; | tr '\n' ' ')"
    return 1
  fi
}

# When executed (not sourced), run one function: registry.sh <function> [args...].
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  set -euo pipefail
  "$@"
fi
