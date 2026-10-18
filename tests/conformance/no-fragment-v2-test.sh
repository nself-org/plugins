#!/usr/bin/env bash
# no-fragment-v2-test.sh (D-0812): free plugins with no compose fragment are manifest v2 library plugins, so the
# conformance gate covers them. Requires, per plugin: manifest_version 2, service.kind library with the manifest
# port, no compose key, boot migrations, a Dockerfile, and the static gate rules pass with the plugin counted as v2
# (a v1 plugin fails check.sh: "plugin.json is not manifest_version 2"). No Docker, no network
# beyond the cli-tools fetch the gate itself needs. Usage: no-fragment-v2-test.sh [plugin ...] (default: the set below)
set -u
HERE=$(cd "$(dirname "$0")" && pwd); . "$HERE/../../scripts/conformance/lib.sh"
plugins=${*:-content-safety crdt}; rc=0
for p in $plugins; do
  d=$CONF_ROOT/free/$p; pj=$d/plugin.json
  [ -f "$pj" ] || { echo "FAIL  $p: no plugin.json"; rc=1; continue; }
  if jq -e '.manifest_version == 2 and .service.kind == "library" and (.service.port == .port) and (.service | has("compose") | not)
            and .migrations.apply == "boot"' "$pj" >/dev/null; then echo "pass  $p: manifest v2 library service on the manifest port, boot migrations"
  else echo "FAIL  $p: want manifest v2, service.kind library with service.port == port, no service.compose, migrations.apply boot"; rc=1; fi
  [ -f "$d/Dockerfile" ] || { echo "FAIL  $p: no Dockerfile"; rc=1; }
  if out=$(bash "$CONF_ROOT/scripts/conformance/check.sh" --plugins "$p" --rules exceptions,schema,compat,healthcheck,replace,docker-socket,port-bind,migrations,boot-schema,legacy-renames,repository,ports 2>&1); then
    printf '%s\n' "$out" | grep -q 'checked 1 v2 plugin' && echo "pass  $p: static gate rules pass as a v2 plugin" || { echo "FAIL  $p: gate checked no v2 plugin"; rc=1; }
  else echo "FAIL  $p: static gate rules failed"; printf '%s\n' "$out" | sed 's/^/        /'; rc=1; fi
done
[ $rc -eq 0 ] && echo "no-fragment-v2: ok" || echo "no-fragment-v2: FAIL"; exit $rc
