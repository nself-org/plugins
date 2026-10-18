#!/usr/bin/env bash
# builder-prune-test.sh (D-0973): build-from-tarball.sh prunes the BuildKit cache when CI is set, and never on a
# developer machine (CI unset or empty). A fake `docker` on PATH records its calls; no real Docker, no network.
# Usage: builder-prune-test.sh
set -u
HERE=$(cd "$(dirname "$0")" && pwd); ROOT=$(cd "$HERE/../.." && pwd); rc=0
no() { echo "FAIL  builder-prune: $1"; rc=1; }
S=$(mktemp -d "${TMPDIR:-/tmp}/builder-prune.XXXXXX"); trap 'rm -rf "$S"' EXIT
mkdir -p "$S/bin" "$S/plug/demo"
cat > "$S/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
echo "$*" >> "$FAKE_DOCKER_LOG"
[ "$1" = build ] && [ -n "${FAKE_DOCKER_FAIL:-}" ] && { echo "boom"; exit 1; }
exit 0
DOCKER
chmod +x "$S/bin/docker"
printf '{"name":"demo","manifest_version":2}\n' > "$S/plug/demo/plugin.json"
printf 'FROM scratch\n' > "$S/plug/demo/Dockerfile"
# run <label> <expected prune count> <env assignments...>: one build, then count `builder prune` calls.
run() {
  local label=$1 want=$2; shift 2
  : > "$S/docker.log"
  env -u CI FAKE_DOCKER_LOG="$S/docker.log" CONF_WORKDIR="$S/work" PATH="$S/bin:$PATH" "$@" \
    bash "$ROOT/scripts/conformance/build-from-tarball.sh" "$S/plug/demo" >"$S/out" 2>&1 || true
  local got; got=$(grep -c '^builder prune' "$S/docker.log")
  [ "$got" = "$want" ] && echo "pass  builder-prune: $label (builder prune x$got)" || { no "$label: want $want builder prune call(s), got $got"; sed 's/^/        /' "$S/docker.log"; }
}
run "CI=true prunes after a good build" 1 CI=true
run "CI=true prunes after a failed build" 1 CI=true FAKE_DOCKER_FAIL=1
run "CI unset never prunes" 0
run "CI empty never prunes" 0 CI=
[ $rc -eq 0 ] && echo "builder-prune: ok" || echo "builder-prune: FAIL"; exit $rc
