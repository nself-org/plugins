#!/usr/bin/env bash
# d0821-test.sh: regression proof for D-0821 on the REAL free/ tree. workflows listens on 3712 (Dockerfile EXPOSE,
# compose fragment, config default), so plugin.json port and service.port must say 3712 too, and no other plugin
# or reserved row may claim 3712. Needs jq only.
# Usage: d0821-test.sh
set -u
HERE=$(cd "$(dirname "$0")" && pwd); ROOT=$(cd "$HERE/../.." && pwd); rc=0
no() { echo "FAIL  d0821: $1"; rc=1; }
m=$ROOT/free/workflows/plugin.json
exp=$(awk '/^EXPOSE /{print $2}' "$ROOT/free/workflows/Dockerfile")
[ "$exp" = 3712 ] && echo "pass  d0821: Dockerfile exposes 3712" || no "Dockerfile EXPOSE is '$exp', expected 3712"
grep -qF '127.0.0.1:3712:3712' "$ROOT/free/workflows/docker-compose.plugin.yml" && echo "pass  d0821: compose fragment publishes 3712" || no "compose fragment does not publish 3712"
for k in .port .service.port; do
  v=$(jq -r "$k" "$m")
  [ "$v" = "$exp" ] && echo "pass  d0821: plugin.json $k is $exp" || no "plugin.json $k is $v, the service listens on $exp"
done
out=$(bash "$ROOT/scripts/conformance/ports.sh" --plugins workflows 2>&1) && echo "pass  d0821: no port collision for workflows" || { no "port collision"; printf '%s\n' "$out" | sed 's/^/        /'; }
[ $rc -eq 0 ] && echo "d0821: ok" || echo "d0821: FAIL"; exit $rc
