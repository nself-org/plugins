#!/usr/bin/env bash
# Rule compat: canonical v2, no forbidden keys (E111), compatibility keys equal the projection (E112).
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir")
tool=$(cli_tool manifestv2migrate) || { fail compat "$name" "cannot build manifestv2migrate"; exit 1; }
out=$("$tool" -check -in "$dir/plugin.json" 2>&1) && exit 0
printf '%s\n' "$out" | while IFS= read -r l; do fail compat "$name" "$l"; done
exit 1
