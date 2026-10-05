#!/usr/bin/env bash
# cli-targets.sh DIR: print the command binaries a plugin ships, one "binary<TAB>command" line each.
# Purpose: the release pipeline builds exactly what the installer will look for (P7-PLUG-55). The
# list comes from the pinned cli tool manifestv2migrate -targets, which normalizes a v1 or v2
# plugin.json and applies the installer's own rule (cli: manifestv2.CLITargets). There is no
# manifest parsing here. A plugin with no command prints nothing and exits 0.
# Inputs: DIR holding plugin.json; scripts/cli-tools.version (pinned cli commit).
#   CLI_TARGETS_TOOL: path of a built manifestv2migrate (the workflow builds it once; unset builds it).
# Exit: 0 ok, 1 the tool refused the manifest, 2 usage or missing plugin.json.
set -eu
dir=${1:-}
[ -n "$dir" ] && [ -f "$dir/plugin.json" ] || { echo "usage: cli-targets.sh DIR (DIR/plugin.json must exist)" >&2; exit 2; }
if [ -z "${CLI_TARGETS_TOOL:-}" ]; then
  . "$(dirname "$0")/../conformance/lib.sh"
  CLI_TARGETS_TOOL=$(cli_tool manifestv2migrate)
fi
exec "$CLI_TARGETS_TOOL" -in "$dir/plugin.json" -targets
