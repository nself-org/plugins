#!/usr/bin/env bash
# check.sh: the free-plugin conformance gate (P7-PLUG-03). Enforced for every plugin whose plugin.json
# has manifest_version 2; v1 plugins are reported and keep the validate.yml checks.
# Usage: check.sh [--plugins a,b | --changed | --all | --fixtures] [--rules r1,r2] [--root DIR] [--exceptions FILE]
# Rules: exceptions (global), schema compat healthcheck replace docker-socket migrations repository ports build.
# --rules runs only the named rules and prints which ran; exit 0 means those rules passed, nothing else.
# Exit: 0 all pass, 1 a rule failed, 2 usage. A rule that cannot run (no docker, no tool) fails, a named plugin that
# does not exist fails, and --plugins/--all that checked no manifest v2 plugin fails (a gate that runs nothing is
# not a pass; --changed with nothing changed passes).
set -u
. "$(dirname "$0")/lib.sh"
mode=all; list=; root=$CONF_ROOT/free; rules=; exc=$CONF_DIR/exceptions.yaml
ALL="schema compat healthcheck replace docker-socket migrations repository ports build"
while [ $# -gt 0 ]; do case $1 in
  --plugins) mode=plugins; list=$2; shift;; --changed) mode=changed;; --all) mode=all;;
  --fixtures) mode=fixtures;; --rules) rules=$(printf '%s' "$2" | tr ',' ' '); shift;;
  --root) root=$2; shift;; --exceptions) exc=$2; shift;;
  *) echo "check.sh: unknown argument $1" >&2; exit 2;; esac; shift; done
[ -n "$rules" ] || rules="exceptions $ALL"
if [ "$mode" = fixtures ]; then exec bash "$CONF_DIR/fixtures.sh"; fi
case $mode in
  changed) list=$(git -C "$CONF_ROOT" diff --name-only origin/main... -- free 2>/dev/null | awk -F/ 'NF > 2 { print $2 }' | sort -u | tr '\n' ',');;
  all) list=$(cd "$root" && ls -d */ 2>/dev/null | tr -d '/' | tr '\n' ',');;
esac
rc=0; checked=0; v1=0
echo "rules: $rules"
case " $rules " in *" exceptions "*) bash "$CONF_DIR/rules/exceptions.sh" "$exc" || rc=1;; esac
for p in $(printf '%s' "$list" | tr ',' ' '); do
  dir=$root/$p
  if [ ! -f "$dir/plugin.json" ]; then
    [ "$mode" = plugins ] && { fail check "$p" "no plugin.json under $root"; rc=1; }
    continue
  fi
  if [ "$(manifest_version "$dir")" != 2 ]; then v1=$((v1 + 1)); continue; fi
  checked=$((checked + 1))
  for r in $rules; do
    case $r in exceptions) continue;; build) cmd="bash $CONF_DIR/build-from-tarball.sh $dir";;
      ports) cmd="bash $CONF_DIR/ports.sh --plugins $p --root $root";;
      docker-socket) cmd="bash $CONF_DIR/rules/docker-socket.sh $dir $exc";;
      *) cmd="bash $CONF_DIR/rules/$r.sh $dir";; esac
    out=$($cmd 2>&1) || { rc=1; printf '%s\n' "$out" | grep . | sed "s/^/[$p] /"; }
  done
done
echo "checked $checked v2 plugin(s); $v1 v1 plugin(s) keep the validate.yml checks"
if [ "$checked" -eq 0 ] && [ "$mode" != changed ]; then echo "FAIL check: no manifest v2 plugin was checked (a gate that ran nothing)"; rc=1; fi
[ $rc -eq 0 ] && echo "conformance: pass" || echo "conformance: FAIL"
exit $rc
