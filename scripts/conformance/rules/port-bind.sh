#!/usr/bin/env bash
# Rule port-bind (security, D-0453): every published port of every compose service in the plugin binds the
# literal 127.0.0.1 (short or long syntax); network_mode host and a ports value that cannot be read also fail.
# An unexpired `rule: port-bind` entry for the plugin in scripts/conformance/exceptions.yaml lets it pass. A deliberate public
# port is not an exception: public-ports.yaml names the plugin and the exact container port (D-0626), checked per port.
# Scans the compose fragment the manifest names (service.compose; required to exist and parse when
# service.kind is compose or a fragment is named) and every *.yml and *.yaml in the plugin (not vendor, node_modules, third_party or testdata:
# a Go test fixture is not deployed). A plugin with no YAML at all has nothing to publish and passes. Other YAML without a top-level `services`
# mapping (k8s manifests, workflows) is skipped by the portbind tool (scripts/conformance/portbind).
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir"); exc=${2:-$CONF_DIR/exceptions.yaml}; pub=${3:-$CONF_DIR/public-ports.yaml}
exception_active "$exc" port-bind "$name" && exit 0
bin=$(local_tool portbind "$CONF_DIR/portbind") || { fail port-bind "$name" "cannot build portbind"; exit 1; }
frag=$(jq -r '.service.compose // empty' "$dir/plugin.json" 2>/dev/null)
kind=$(jq -r '.service.kind // empty' "$dir/plugin.json" 2>/dev/null)
set --
while IFS="$(printf '\t')" read -r pp port; do
  [ "$pp" = "$name" ] && [ -n "$port" ] && set -- "$@" -allow "$port"
done <<EOF
$(yaml_rows "$pub" plugin port)
EOF
if [ -n "$frag" ] && [ -f "$dir/$frag" ]; then set -- "$@" -require "$dir/$frag"
elif [ -n "$frag" ] || [ "$kind" = compose ]; then fail port-bind "$name" "service.compose names ${frag:-no file}, which does not exist"; exit 1; fi
# NUL-separated: a tracked file name may hold a newline (free/media-processing has one).
while IFS= read -r -d '' f; do set -- "$@" "$f"; done < <(find "$dir" \( -name vendor -o -name node_modules -o -name third_party -o -name testdata \) -prune -o \
  \( -name '*.yml' -o -name '*.yaml' \) -type f -print0 | sort -z)
[ $# -gt 0 ] || exit 0
out=$("$bin" "$@" 2>&1); r=$?
if [ $r -eq 0 ]; then exit 0; fi
if [ $r -eq 1 ]; then
  printf '%s\n' "$out" | while IFS= read -r l; do fail port-bind "$name" "${l#"$dir"/}"; done
else
  # Usage or parse errors already start with "portbind:"; strip the plugin dir wherever it appears.
  printf '%s\n' "$out" | while IFS= read -r l; do fail port-bind "$name" "${l//"$dir"\//}"; done
fi
exit 1
