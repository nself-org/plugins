#!/usr/bin/env bash
# d0815-test.sh: regression proof for D-0815 on the REAL free/ tree. forgejo and file-processing must be
# manifest v2 and pass the whole conformance gate (ports, port-bind, build from the extracted tarball with no
# network), and port 3844 must belong to forgejo alone (no reserved row, nself-sync on its own 3853).
# Usage: d0815-test.sh   Needs docker (the build rule fails without it, as the gate does).
set -u
HERE=$(cd "$(dirname "$0")" && pwd); ROOT=$(cd "$HERE/../.." && pwd); rc=0
no() { echo "FAIL  d0815: $1"; rc=1; }
for p in forgejo file-processing; do
  [ "$(jq -r '.manifest_version // 1' "$ROOT/free/$p/plugin.json")" = 2 ] && echo "pass  d0815: $p is manifest v2" || no "$p is not manifest v2"
done
img=$(jq -r '.service.image // empty' "$ROOT/free/forgejo/plugin.json")
printf '%s' "$img" | grep -Eq '^[a-z0-9][^@[:space:]]*:[^@:/[:space:]]+@sha256:[0-9a-f]{64}$' && echo "pass  d0815: forgejo service.image is pinned by tag and digest" || no "forgejo service.image is not <repo>:<tag>@sha256:<digest> ('$img')"
[ -n "$img" ] && grep -qF "image: $img" "$ROOT/free/forgejo/docker-compose.plugin.yml" && echo "pass  d0815: forgejo fragment runs the pinned image" || no "forgejo fragment does not use service.image"
[ "$(jq -r '.port' "$ROOT/free/forgejo/plugin.json")" = 3844 ] && echo "pass  d0815: forgejo keeps 3844" || no "forgejo port is not 3844"
[ "$(jq -r '.port' "$ROOT/free/nself-sync/plugin.json")" = 3853 ] && [ "$(jq -r '.env.optional.NSELF_SYNC_PORT' "$ROOT/free/nself-sync/plugin.json")" = 3853 ] && echo "pass  d0815: nself-sync is on 3853" || no "nself-sync port or NSELF_SYNC_PORT default is not 3853"
out=$(bash "$ROOT/scripts/conformance/ports.sh" --plugins forgejo,file-processing,nself-sync 2>&1) && echo "pass  d0815: no port collision for forgejo, file-processing, nself-sync" || { no "port collision"; printf '%s\n' "$out" | sed 's/^/        /'; }
out=$(bash "$ROOT/scripts/conformance/check.sh" --plugins forgejo,file-processing 2>&1) && echo "pass  d0815: conformance gate passes for both" || { no "conformance gate"; printf '%s\n' "$out" | tail -25 | sed 's/^/        /'; }
[ $rc -eq 0 ] && echo "d0815: ok" || echo "d0815: FAIL"; exit $rc
