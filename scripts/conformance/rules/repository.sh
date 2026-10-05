#!/usr/bin/env bash
# Rule repository: free manifests name nself-org/plugins, licensed ones nself-org/bundles.
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir"); got=$(jq -r '.repository // ""' "$dir/plugin.json")
if is_free "$dir"; then want=$FREE_REPO_URL; else want=$LICENSED_REPO_URL; fi
[ "$got" = "$want" ] && exit 0
fail repository "$name" "repository is '$got', want '$want'"; exit 1
