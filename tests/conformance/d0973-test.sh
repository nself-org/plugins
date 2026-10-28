#!/usr/bin/env bash
# d0973-test.sh: regression proof for D-0973. build-from-tarball.sh prunes the BuildKit cache on exit only when
# CI=true (a runner), never on a developer machine, and also on a failed build. A stub `docker` on PATH records
# its calls; no real Docker or network is used. Usage: d0973-test.sh
set -u
HERE=$(cd "$(dirname "$0")" && pwd); ROOT=$(cd "$HERE/../.." && pwd); rc=0
no() { echo "FAIL  d0973: $1"; rc=1; }
T=$(mktemp -d "${TMPDIR:-/tmp}/d0973.XXXXXX"); trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/plugins/demo"
printf '{"name":"demo"}\n' > "$T/plugins/demo/plugin.json"; printf 'FROM scratch\n' > "$T/plugins/demo/Dockerfile"
cat > "$T/bin/docker" <<'STUB'
#!/bin/sh
echo "$*" >> "$STUB_LOG"
[ "$1" = build ] && [ -n "${STUB_BUILD_FAIL:-}" ] && { echo "boom"; exit 1; }
exit 0
STUB
chmod +x "$T/bin/docker"
run() { # CI value (empty = unset), build-fail flag
  : > "$T/log"; env -u CI ${1:+CI=$1} STUB_LOG="$T/log" ${2:+STUB_BUILD_FAIL=1} CONF_WORKDIR="$T/work" PATH="$T/bin:$PATH" \
    bash "$ROOT/scripts/conformance/build-from-tarball.sh" "$T/plugins/demo" >/dev/null 2>&1
}
pruned() { grep -q '^builder prune -f' "$T/log"; }
run true;  pruned && echo "pass  d0973: CI=true prunes the builder cache"        || no "CI=true did not run docker builder prune -f"
run true 1; pruned && echo "pass  d0973: CI=true prunes after a failed build"     || no "CI=true did not prune after a failed build"
run "";    pruned && no "CI unset ran docker builder prune (developer machine)"   || echo "pass  d0973: CI unset never prunes"
run false; pruned && no "CI=false ran docker builder prune"                       || echo "pass  d0973: CI=false never prunes"
run true; grep -q '^rmi -f' "$T/log" && echo "pass  d0973: the image is still removed" || no "docker rmi -f missing"
[ $rc -eq 0 ] && echo "d0973: ok" || echo "d0973: FAIL"; exit $rc
