#!/usr/bin/env bash
# Rule replace: no go.mod / go.work replace or use directive points outside the plugin directory
# (quoted targets included).
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir"); root=$(norm_path "$dir"); rc=0
inside() { case "$(norm_path "$1")/" in "$root"/*) return 0;; esac; return 1; }
while IFS= read -r gm; do
  [ -n "$gm" ] || continue
  base=$(dirname "$gm")
  while read -r mod target; do
    [ -n "$mod" ] || continue
    case "$target" in /*) abs=$target;; *) abs=$base/$target;; esac
    inside "$abs" || { fail replace "$name" "$gm: replace $mod => $target leaves the plugin directory"; rc=1; }
  done <<EOF
$(local_replaces "$gm")
EOF
done <<EOF
$(go_mods "$dir"; go_works "$dir")
EOF
while IFS= read -r gw; do
  [ -n "$gw" ] || continue
  while IFS= read -r target; do
    [ -n "$target" ] || continue
    case "$target" in /*) abs=$target;; *) abs=$(dirname "$gw")/$target;; esac
    inside "$abs" || { fail replace "$name" "$gw: use $target leaves the plugin directory"; rc=1; }
  done <<EOF
$(go_work_uses "$gw")
EOF
done <<EOF
$(go_works "$dir")
EOF
exit $rc
