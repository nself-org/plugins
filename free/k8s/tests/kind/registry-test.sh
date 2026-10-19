#!/usr/bin/env bash
# Purpose: offline tests for registry.sh (P7-DEPL-31), the registry leg's selection and assertions.
#   Runs in the pull-request job; needs only bash and jq (no cluster, no docker, no network).
# Inputs: fixture/registry/*.json (synthetic registries, help listings and manifests).
# Outputs: exit 0 and "registry-test: ok (N cases)" when every case behaves; 1 with the failing
#   case names otherwise.
# Constraints: the fixtures hold fake checksums only; nothing outside a mktemp dir is written.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
fx="$here/fixture/registry"
# shellcheck disable=SC1091
. "$here/registry.sh"

work="$(mktemp -d "${TMPDIR:-/tmp}/registry-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
cases=0
failed=""

# ok <name> <command...>: the command must succeed.
ok() {
  local name="$1"
  shift
  cases=$((cases + 1))
  if ! "$@" >"$work/out" 2>"$work/err"; then
    failed="$failed [$name: exited non-zero: $(cat "$work/err")]"
  fi
}

# bad <name> <stderr substring...> -- <command...>: the command must fail and its stderr must
# contain every substring.
bad() {
  local name="$1" want=() s
  shift
  while [ "$1" != "--" ]; do want+=("$1"); shift; done
  shift
  cases=$((cases + 1))
  if "$@" >"$work/out" 2>"$work/err"; then
    failed="$failed [$name: expected failure, got success]"
    return
  fi
  for s in "${want[@]}"; do
    grep -qF -- "$s" "$work/err" || failed="$failed [$name: stderr lacks '$s': $(cat "$work/err")]"
  done
}

# eq <name> <want> <got>
eq() {
  cases=$((cases + 1))
  [ "$2" = "$3" ] || failed="$failed [$1: want '$2' got '$3']"
}

# Registry shapes: an array under .plugins (as served), a map keyed by name, a bare array.
for shape in served-array-1.2.6 served-map-1.2.6 served-bare-array-1.2.6; do
  eq "version from $shape" "1.2.6" "$(registry_served_version "$fx/$shape.json" k8s)"
done
eq "platform checksum" "0000000000000000000000000000000000000000000000000000000000000001" \
  "$(registry_platform_checksum "$fx/served-array-1.2.6.json" k8s linux-amd64)"
eq "unknown platform is empty" "" "$(registry_platform_checksum "$fx/served-array-1.2.6.json" k8s plan9-386)"
eq "other row is not mixed up" "1.0.0" "$(registry_served_version "$fx/served-map-1.2.6.json" ci)"
bad "row missing" "no row for k8s" -- registry_served_version "$fx/served-missing.json" k8s
bad "unreadable registry" "not a readable registry" -- registry_row "$fx/help-old.json.nope" k8s

# Served vs checkout version.
ok "served equals checkout" assert_served_matches_checkout 1.2.6 1.2.6
bad "served older than checkout" "served k8s is 1.2.5" "older than the checkout's 1.2.6" -- assert_served_matches_checkout 1.2.5 1.2.6
bad "served newer than checkout" "served k8s is 1.3.0" "newer than the checkout's 1.2.6" -- assert_served_matches_checkout 1.3.0 1.2.6
bad "1.2.10 is newer than 1.2.9" "newer than" -- assert_served_matches_checkout 1.2.10 1.2.9

# Installed manifest vs served row vs checkout.
ok "installed manifest matches" assert_installed_version "$fx/installed-1.2.6.json" 1.2.6 1.2.6
bad "installed manifest differs from the row" "installed manifest says 1.2.5" "served row says 1.2.6" -- \
  assert_installed_version "$fx/installed-1.2.5.json" 1.2.6 1.2.6
bad "installed manifest unreadable" "no readable version" -- assert_installed_version "$fx/nope.json" 1.2.6 1.2.6
bad "installed and served older than checkout" "older than the checkout's 1.2.6" -- \
  assert_installed_version "$fx/installed-1.2.5.json" 1.2.5 1.2.6

# Chart-era commands: the same version with OLD bytes must fail and name what is missing.
bad "same version, old bytes" "served k8s 1.2.6 predates the chart" "lacks: values backup" -- assert_chart_era "$fx/help-old.json" 1.2.6
ok "same version, chart-era bytes" assert_chart_era "$fx/help-new.json" 1.2.6
bad "help is not a listing" "not a command listing" -- assert_chart_era "$fx/served-missing.json.nope" 1.2.6
eq "full leg, old bytes fail at the chart check" "1" \
  "$(assert_served_matches_checkout 1.2.6 1.2.6 2>/dev/null && assert_chart_era "$fx/help-old.json" 1.2.6 2>/dev/null; echo $?)"

# Empty plugin dir: a branch-built plugin laid out beforehand fails the leg.
mkdir -p "$work/empty" "$work/laid/k8s"
ok "plugin dir absent" assert_empty_plugin_dir "$work/absent"
ok "plugin dir empty" assert_empty_plugin_dir "$work/empty"
bad "plugin dir holds a plugin" "not empty before nself add k8s" -- assert_empty_plugin_dir "$work/laid"

# The registry URL: the override wins; otherwise the cli's own default is read.
mkdir -p "$work/cli/internal/plugin"
printf 'package plugin\n\nconst (\n\tDefaultRegistryURL = "https://registry.example.test/registry.json"\n\tFallbackRegistryURL = "https://fallback.example.test/r.json"\n)\n' >"$work/cli/internal/plugin/registry.go"
eq "url from the cli default" "https://registry.example.test/registry.json" "$(NSELF_PLUGIN_REGISTRY='' registry_url "$work/cli")"
eq "url from the override" "https://override.example.test/r.json" "$(NSELF_PLUGIN_REGISTRY=https://override.example.test/r.json registry_url "$work/cli")"
bad "no override, no cli source" "no NSELF_PLUGIN_REGISTRY" -- env NSELF_PLUGIN_REGISTRY= bash "$here/registry.sh" registry_url "$work/none"

if [ -n "$failed" ]; then
  echo "registry-test: FAIL:$failed" >&2
  exit 1
fi
echo "registry-test: ok ($cases cases)"
