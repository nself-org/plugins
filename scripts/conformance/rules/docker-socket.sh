#!/usr/bin/env bash
# Rule docker-socket (ADR 0027): no Docker socket mount in a plugin container, unless
# scripts/conformance/exceptions.yaml holds an unexpired entry for the plugin.
# Scans the compose fragment the manifest names (service.compose, resolved to the real file whatever it is
# called), every *.yml and *.yaml in the plugin, and every Dockerfile*. A bind of /var/run itself counts.
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir"); exc=${2:-$CONF_DIR/exceptions.yaml}; rc=0
frag=$(jq -r '.service.compose // empty' "$dir/plugin.json" 2>/dev/null)
# NUL-separated: a tracked file name may hold a newline (D-0628; port-bind has been NUL-safe since D-0453).
while IFS= read -r -d '' f; do
  [ -n "$f" ] || continue
  if grep -nE 'docker\.sock|/var/run/docker|(^|[[:space:]"'"'"'-])/var/run/?([:"'"'"'[:space:]]|$)' "$f" >/dev/null 2>&1; then
    if exception_active "$exc" docker-socket "$name"; then continue; fi
    fail docker-socket "$name" "${f#$dir/} mounts the Docker socket (ADR 0027) with no unexpired exception"; rc=1
  fi
done < <( { [ -n "$frag" ] && [ -f "$dir/$frag" ] && printf '%s\0' "$dir/$frag"
  find "$dir" \( -name vendor -o -name node_modules -o -name third_party \) -prune -o \
    \( -name '*.yml' -o -name '*.yaml' -o -name 'Dockerfile*' \) -type f -print0; } | sort -zu )
exit $rc
