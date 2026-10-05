#!/usr/bin/env bash
# Rule healthcheck: a compose service has a real health check: a compose `healthcheck:` that is not disabled
# (disable: true, test NONE), or a Dockerfile HEALTHCHECK whose last instruction is not NONE. A disabled compose
# healthcheck also switches off the image one, so it fails even when the Dockerfile has one.
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir"); pj=$dir/plugin.json
[ "$(jq -r '.service.kind // ""' "$pj")" = compose ] || exit 0
frag=$dir/$(jq -r '.service.compose // ""' "$pj")
if [ ! -f "$frag" ] || [ -d "$frag" ]; then fail healthcheck "$name" "service.compose fragment missing: $frag"; exit 1; fi
# compose_health <file>: prints "none" (no healthcheck block), "disabled" or "ok".
compose_health() {
  awk 'function ind(s) { match(s, /^[[:space:]]*/); return RLENGTH }
    /^[[:space:]]*healthcheck:/ { found = 1; hi = ind($0); inh = 1
      if ($0 ~ /disable:[[:space:]]*(true|yes|"true")/ || $0 ~ /NONE/) bad = 1; next }
    inh { if ($0 ~ /^[[:space:]]*$/ || $0 ~ /^[[:space:]]*#/) next
      if (ind($0) <= hi) inh = 0
      else { if ($0 ~ /disable:[[:space:]]*(true|yes|"true")/ || ($0 ~ /test:/ && $0 ~ /NONE/) || $0 ~ /^[[:space:]]*-[[:space:]]*"?NONE"?[[:space:]]*$/) bad = 1 } }
    END { print !found ? "none" : (bad ? "disabled" : "ok") }' "$1"
}
state=$(compose_health "$frag")
case $state in
  disabled) fail healthcheck "$name" "the compose healthcheck is disabled (disable: true or test NONE)"; exit 1;;
  ok) exit 0;;
esac
if [ -f "$dir/Dockerfile" ]; then
  last=$(grep -Ei '^[[:space:]]*HEALTHCHECK[[:space:]]' "$dir/Dockerfile" | tail -1)
  if [ -n "$last" ]; then
    printf '%s\n' "$last" | grep -Eiq '^[[:space:]]*HEALTHCHECK[[:space:]]+NONE([[:space:]]|$)' && { fail healthcheck "$name" "Dockerfile ends with HEALTHCHECK NONE"; exit 1; }
    exit 0
  fi
fi
fail healthcheck "$name" "neither the compose fragment nor the Dockerfile declares a health check"
exit 1
