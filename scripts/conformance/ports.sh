#!/usr/bin/env bash
# ports.sh: port uniqueness (final gate G10). Reads the top-level port, config.port and every *_PORT
# default under env.optional of each plugin.json under the root, plus reserved-ports.yaml.
# A port shared by two plugins, or a plugin on a reserved port, prints "collision <port> <owner> <owner>"
# and exits 1. Port 0 or absent is ignored.
# Usage: ports.sh [--plugins a,b | --changed | --all | --fixtures] [--root DIR] [--report]
set -u
. "$(dirname "$0")/lib.sh"
mode=all; list=; root=$CONF_ROOT/free; report=0
while [ $# -gt 0 ]; do case $1 in
  --plugins) mode=plugins; list=$2; shift;; --changed) mode=changed;; --all) mode=all;;
  --fixtures) mode=fixtures;; --root) root=$2; shift;; --report) report=1;;
  *) echo "ports.sh: unknown argument $1" >&2; exit 2;; esac; shift; done

# claims root: "<port>\t<owner>\t<key>" for every plugin under root (and a plugins-pro checkout next to the repo).
claims() {
  local r=$1 f n
  for f in "$r"/*/plugin.json; do
    [ -f "$f" ] || continue; n=$(jq -r '.name // empty' "$f")
    jq -r --arg n "$n" '
      def p(k; v): if v == null then empty else "\(v)\t\($n)\t\(k)" end;
      p("port"; (.port | numbers)), p("config.port"; (.config.port? | numbers)),
      p("service.port"; (.service.port? | numbers)),
      ((.env.optional? // .envVars.optional? // {}) | if type == "object" then to_entries[] | select(.key | test("_PORT$")) | p(.key; (.value | tonumber? // empty)) else empty end)' "$f" 2>/dev/null
  done | awk -F'\t' '$1 != 0'
}
reserved() { yaml_rows "$CONF_DIR/reserved-ports.yaml" port owner | awk -F'\t' '{ o = $2; gsub(/ /, "_", o); print $1 "\treserved:" o "\t-" }'; }

case $mode in
  fixtures) exec bash "$CONF_ROOT/tests/conformance/ports-fixtures.sh";;
  changed) list=$(git -C "$CONF_ROOT" diff --name-only origin/main... -- free 2>/dev/null | awk -F/ 'NF > 2 { print $2 }' | sort -u | tr '\n' ',');;
esac
all=$( { claims "$root"; [ "$root" = "$CONF_ROOT/free" ] && [ -d "$CONF_ROOT/../plugins-pro/paid" ] && claims "$CONF_ROOT/../plugins-pro/paid"; reserved; } )
collisions=$(printf '%s\n' "$all" | awk -F'\t' '
  NF { key = $1; if (!((key SUBSEP $2) in seen)) { seen[key SUBSEP $2] = 1; n[key]++; own[key] = own[key] (own[key] ? " " : "") $2 } }
  END { for (k in n) if (n[k] > 1) print "collision " k " " own[k] }' | sort)
if [ "$report" = 1 ]; then printf '%s\n' "$collisions" | grep . ; exit 0; fi
if [ "$mode" = plugins ] || [ "$mode" = changed ]; then
  collisions=$(printf '%s\n' "$collisions" | awk -v L=",$list," '{ for (i = 3; i <= NF; i++) { o = $i; if (index(L, "," o ",")) { print; break } } }')
fi
[ -z "$collisions" ] && exit 0
printf '%s\n' "$collisions"; exit 1
