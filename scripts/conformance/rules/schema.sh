#!/usr/bin/env bash
# Rule schema: plugin.json validates against the pinned v2 schema copy (schemas/).
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir")
bin=$(local_tool schemacheck "$CONF_DIR/schemacheck") || { fail schema "$name" "cannot build schemacheck"; exit 1; }
out=$("$bin" "$CONF_ROOT/schemas/plugin-manifest.v2.schema.json" "$dir/plugin.json" 2>&1) || { printf '%s\n' "$out" | while IFS= read -r l; do fail schema "$name" "$l"; done; exit 1; }
