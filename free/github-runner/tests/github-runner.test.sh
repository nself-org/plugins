#!/bin/bash
# =============================================================================
# Tests for GitHub Actions Runner plugin
# =============================================================================
# Run: bash tests/github-runner.test.sh
#
# Tests validate script structure and offline logic only.
# Registration/download tests require live GitHub credentials.

set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); printf "  ✓ %s\n" "$1"; }
fail() { FAIL=$((FAIL + 1)); printf "  ✗ %s\n" "$1"; }

# =============================================================================
# 1. File structure tests
# =============================================================================
echo "File structure"

[[ -f "$PLUGIN_DIR/plugin.json" ]] && pass "plugin.json exists" || fail "plugin.json missing"
[[ -f "$PLUGIN_DIR/install.sh" ]] && pass "install.sh exists" || fail "install.sh missing"
[[ -f "$PLUGIN_DIR/uninstall.sh" ]] && pass "uninstall.sh exists" || fail "uninstall.sh missing"
[[ -f "$PLUGIN_DIR/README.md" ]] && pass "README.md exists" || fail "README.md missing"

for action in start stop status logs update; do
  [[ -f "$PLUGIN_DIR/actions/${action}.sh" ]] && pass "actions/${action}.sh exists" || fail "actions/${action}.sh missing"
done

# =============================================================================
# 2. plugin.json validation
# =============================================================================
echo ""
echo "plugin.json"

name=$(python3 -c "import json; print(json.load(open('$PLUGIN_DIR/plugin.json'))['name'])")
[[ "$name" == "github-runner" ]] && pass "name is 'github-runner'" || fail "name is '$name'"

license=$(python3 -c "import json; print(json.load(open('$PLUGIN_DIR/plugin.json'))['license_spdx'])")
[[ "$license" == "MIT" ]] && pass "license is MIT" || fail "license is '$license'"

req_env=$(python3 -c "import json; d=json.load(open('$PLUGIN_DIR/plugin.json')); print(len([e for e in d['envVars'] if e.get('required')]))")
[[ "$req_env" -ge 2 ]] && pass "has $req_env required env vars" || fail "expected >=2 required env vars, got $req_env"

# =============================================================================
# 3. Shell script syntax validation
# =============================================================================
echo ""
echo "Shell syntax (bash -n)"

for script in "$PLUGIN_DIR/install.sh" "$PLUGIN_DIR/uninstall.sh" "$PLUGIN_DIR"/actions/*.sh; do
  if bash -n "$script" 2>/dev/null; then
    pass "$(basename "$script") — valid syntax"
  else
    fail "$(basename "$script") — syntax error"
  fi
done

# =============================================================================
# 4. Architecture detection (offline)
# =============================================================================
echo ""
echo "Architecture detection"

# Source install.sh functions in a subshell to test detect_arch/detect_os
# We can't source directly because install.sh calls install_github_runner at the end.
# Instead, test that the functions exist by grepping.
grep -q 'detect_arch()' "$PLUGIN_DIR/install.sh" && pass "detect_arch() defined" || fail "detect_arch() missing"
grep -q 'detect_os()' "$PLUGIN_DIR/install.sh" && pass "detect_os() defined" || fail "detect_os() missing"
grep -q 'get_registration_token()' "$PLUGIN_DIR/install.sh" && pass "get_registration_token() defined" || fail "missing"
grep -q 'get_latest_runner_version()' "$PLUGIN_DIR/install.sh" && pass "get_latest_runner_version() defined" || fail "missing"

# =============================================================================
# 5. Action scripts check required vars
# =============================================================================
echo ""
echo "Action scripts guard env vars"

grep -q 'RUNNER_DIR' "$PLUGIN_DIR/actions/start.sh" && pass "start.sh references RUNNER_DIR" || fail "start.sh missing RUNNER_DIR"
grep -q 'RUNNER_DIR' "$PLUGIN_DIR/actions/stop.sh" && pass "stop.sh references RUNNER_DIR" || fail "stop.sh missing RUNNER_DIR"
grep -q 'RUNNER_DIR' "$PLUGIN_DIR/actions/status.sh" && pass "status.sh references RUNNER_DIR" || fail "status.sh missing RUNNER_DIR"
grep -q 'LOG_DIR' "$PLUGIN_DIR/actions/logs.sh" && pass "logs.sh references LOG_DIR" || fail "logs.sh missing LOG_DIR"

# =============================================================================
# 6. Trusted-only guard (stubbed curl, no network)
# =============================================================================
echo ""
echo "Trusted-only guard"

command -v jq >/dev/null 2>&1 || { echo "jq is required for the guard tests"; exit 1; }
: "${TMPDIR:?TMPDIR must be set}"
T=$(mktemp -d "${TMPDIR%/}/github-runner-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
FAKE_PAT="fake-pat-0123456789-not-a-token"
mkdir -p "$T/bin" "$T/api" "$T/home/.nself/runners/github-runner" "$T/home/.nself/logs/plugins/github-runner"

# Stub curl: serves $T/api/{groups-N,repo,user}.json, logs argv, appends the HTTP
# status after the body when asked with -w. Reads stdin for `-K -`.
cat > "$T/bin/curl" <<'STUB'
#!/bin/bash
printf '%s\n' "$*" >> "$STUB_DIR/argv.log"
args=("$@"); url=""; want_status=false; head_only=false
for a in "${args[@]}"; do
  [[ "$a" == http* ]] && url="$a"
  [[ "$a" == "-w" ]] && want_status=true
  [[ "$a" == "-sI" ]] && head_only=true
done
[[ " ${args[*]} " == *" -K "* ]] && cat > /dev/null
if $head_only; then
  # A fine-grained token gets no x-oauth-scopes header.
  if [[ -f "$STUB_DIR/no-scopes" ]]; then printf 'HTTP/2 200\r\n'; else printf 'HTTP/2 200\r\nx-oauth-scopes: admin:org\r\n'; fi
  exit 0
fi
case "$url" in
  # runner lists (default: no runner registered): a group's runners, then the org's
  */runner-groups/*/runners*) f="$STUB_DIR/api/group-runners.json"; [[ -f "$f" ]] || { code=200; echo '{"runners":[]}'; $want_status && printf '\n%s' "$code"; exit 0; } ;;
  */actions/runners\?*) f="$STUB_DIR/api/org-runners.json"; [[ -f "$f" ]] || { code=200; echo '{"runners":[]}'; $want_status && printf '\n%s' "$code"; exit 0; } ;;
  *runner-groups*page=*) f="$STUB_DIR/api/groups-${url##*page=}.json" ;;
  */repos/*/actions/runners/registration-token) f="$STUB_DIR/api/token.json" ;;
  */orgs/*/actions/runners/registration-token) f="$STUB_DIR/api/token.json" ;;
  */repos/*) f="$STUB_DIR/api/repo.json" ;;
  *) f="$STUB_DIR/api/none.json" ;;
esac
if [[ -f "$f" ]]; then code=200; cat "$f"; else code=404; fi
$want_status && printf '\n%s' "$code"
exit 0
STUB
chmod +x "$T/bin/curl"
export STUB_DIR="$T" PATH="$T/bin:$PATH"

reset_api() { rm -f "$T"/api/* "$T/argv.log"; : > "$T/argv.log"; }
group_json() { # name default public [id]
  printf '{"id":%s,"name":"%s","default":%s,"allows_public_repositories":%s}' "${4:-1}" "$1" "$2" "$3"
}
export GITHUB_RUNNER_PAT="$FAKE_PAT" GITHUB_RUNNER_ORG="acme" HOME="$T/home"
unset GITHUB_RUNNER_GROUP GITHUB_RUNNER_SCOPE GITHUB_RUNNER_REPO GITHUB_RUNNER_ALLOW_PUBLIC_REPOS ENV 2>/dev/null || true
source "$PLUGIN_DIR/actions/trust.sh"

run_check() { local rc=0; ERR=$(trust_check 2>&1 >/dev/null) || rc=$?; RC=$rc; }

reset_api
printf '{"runner_groups":[%s]}' "$(group_json Default true true)" > "$T/api/groups-1.json"
run_check
[[ "$RC" == 1 && "$ERR" == *runner_group_public* ]] && pass "public group is refused with runner_group_public" || fail "public group: rc=$RC err=$ERR"
[[ "$ERR" != *"$FAKE_PAT"* ]] && pass "refusal message does not print the PAT" || fail "PAT leaked in message"
! grep -q "$FAKE_PAT" "$T/argv.log" && pass "PAT never appears on curl argv" || fail "PAT on curl argv"

GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true run_check
[[ "$RC" == 0 && "$ERR" == *WARNING* ]] && pass "override proceeds with a warning" || fail "override: rc=$RC err=$ERR"

printf '{"runner_groups":[%s]}' "$(group_json Default true false)" > "$T/api/groups-1.json"
run_check
[[ "$RC" == 0 ]] && pass "private-only group proceeds" || fail "private group: rc=$RC err=$ERR"

# Empty group name resolves to the default group on page 2.
reset_api
jq -n '{runner_groups: [range(0;100) | {id: ., name: ("g\(.)"), default: false, allows_public_repositories: false}]}' > "$T/api/groups-1.json"
printf '{"runner_groups":[%s]}' "$(group_json Everyone true true)" > "$T/api/groups-2.json"
run_check
[[ "$RC" == 1 && "$ERR" == *Everyone* ]] && pass "empty group resolves the default group across two pages" || fail "pagination: rc=$RC err=$ERR"

# Named group, case-insensitive.
reset_api
printf '{"runner_groups":[%s,%s]}' "$(group_json Default true true)" "$(group_json CI false false)" > "$T/api/groups-1.json"
GITHUB_RUNNER_GROUP=ci run_check
[[ "$RC" == 0 ]] && pass "named private group proceeds while the default is public" || fail "named group: rc=$RC err=$ERR"

# A runner registered in another group than the configured one is refused (D-1180).
reset_api
printf '{"runner_groups":[%s,%s]}' "$(group_json Default true true)" "$(group_json CI false false 2)" > "$T/api/groups-1.json"
printf '{"runners":[{"name":"n1"}]}' > "$T/api/org-runners.json"
GITHUB_RUNNER_GROUP=CI GITHUB_RUNNER_NAME=n1 run_check
[[ "$RC" == 4 && "$ERR" == *runner_group_mismatch* ]] && pass "runner registered in another group is refused with runner_group_mismatch" || fail "mismatch: rc=$RC err=$ERR"
GITHUB_RUNNER_GROUP=CI GITHUB_RUNNER_NAME=n1 TRUST_REGISTERING=true run_check
[[ "$RC" == 0 ]] && pass "install (TRUST_REGISTERING) may move a runner into the configured group" || fail "registering: rc=$RC err=$ERR"
printf '{"runners":[{"name":"N1"}]}' > "$T/api/group-runners.json"
GITHUB_RUNNER_GROUP=CI GITHUB_RUNNER_NAME=n1 run_check
[[ "$RC" == 0 ]] && pass "runner registered in the configured group proceeds" || fail "in group: rc=$RC err=$ERR"
rm -f "$T/api/group-runners.json" "$T/api/org-runners.json"
GITHUB_RUNNER_GROUP=CI GITHUB_RUNNER_NAME=n1 run_check
[[ "$RC" == 0 ]] && pass "runner not registered anywhere proceeds" || fail "unregistered: rc=$RC err=$ERR"

# Missing flag and API failure fail closed.
reset_api
printf '{"runner_groups":[{"name":"Default","default":true}]}' > "$T/api/groups-1.json"
run_check
[[ "$RC" == 2 && "$ERR" == *runner_trust_unverifiable* ]] && pass "missing allows_public_repositories fails closed" || fail "missing flag: rc=$RC err=$ERR"
reset_api
run_check
[[ "$RC" == 2 ]] && pass "API failure fails closed" || fail "API failure: rc=$RC"

# Repo scope.
reset_api
echo '{"private":false}' > "$T/api/repo.json"
GITHUB_RUNNER_SCOPE=repo GITHUB_RUNNER_REPO=app run_check
[[ "$RC" == 1 && "$ERR" == *runner_group_public* ]] && pass "public repo is refused with the same code" || fail "public repo: rc=$RC err=$ERR"
echo '{"private":true}' > "$T/api/repo.json"
GITHUB_RUNNER_SCOPE=repo GITHUB_RUNNER_REPO=app run_check
[[ "$RC" == 0 ]] && pass "private repo proceeds" || fail "private repo: rc=$RC err=$ERR"

# deploy_host_warning
for e in prod production staging PROD; do
  [[ -n "$(ENV=$e trust_deploy_host_warning)" ]] && pass "deploy_host_warning for ENV=$e" || fail "no warning for ENV=$e"
done
for e in "" dev development; do
  [[ -z "$(ENV=$e trust_deploy_host_warning)" ]] && pass "no deploy_host_warning for ENV='$e'" || fail "unexpected warning for ENV='$e'"
done

# install.sh refuses a public group: non-zero exit, same message, no PAT printed.
reset_api
printf '{"runner_groups":[%s]}' "$(group_json Default true true)" > "$T/api/groups-1.json"
rc=0; out=$(bash "$PLUGIN_DIR/install.sh" 2>&1) || rc=$?
[[ "$rc" == 1 && "$out" == *runner_group_public* ]] && pass "install.sh exits non-zero with runner_group_public" || fail "install.sh: rc=$rc"
[[ "$out" != *"$FAKE_PAT"* ]] && pass "install.sh output never prints the PAT" || fail "install.sh printed the PAT"
[[ ! -f "$T/home/.nself/runners/github-runner/config.sh" ]] && pass "install.sh stopped before downloading or configuring" || fail "install.sh went on after refusal"

# A fine-grained token (no x-oauth-scopes header) must still reach the guard.
touch "$T/no-scopes"
rc=0; out=$(bash "$PLUGIN_DIR/install.sh" 2>&1) || rc=$?
[[ "$rc" == 1 && "$out" == *runner_group_public* ]] && pass "install.sh with a fine-grained token (no scopes header) reaches the guard" || fail "no scopes header: rc=$rc"
rm -f "$T/no-scopes"

# An unknown scope never skips the repo check; names are validated.
reset_api
echo '{"private":false}' > "$T/api/repo.json"
GITHUB_RUNNER_SCOPE=repository GITHUB_RUNNER_REPO=app run_check
[[ "$RC" == 2 && "$ERR" == *runner_trust_unverifiable* ]] && pass "unknown scope fails closed" || fail "unknown scope: rc=$RC err=$ERR"
GITHUB_RUNNER_SCOPE=REPO GITHUB_RUNNER_REPO=app run_check
[[ "$RC" == 1 && "$ERR" == *runner_group_public* ]] && pass "scope is case-insensitive (REPO)" || fail "REPO: rc=$RC err=$ERR"
GITHUB_RUNNER_SCOPE=repo GITHUB_RUNNER_REPO='app/../x' run_check
[[ "$RC" == 2 ]] && pass "repo name with a slash fails closed" || fail "bad repo name: rc=$RC"
GITHUB_RUNNER_ORG='ac me' run_check
[[ "$RC" == 2 ]] && pass "org name with a space fails closed" || fail "bad org name: rc=$RC"
export GITHUB_RUNNER_ORG=acme
reset_api
printf '{"runner_groups":[%s]}' "$(group_json Default true true)" > "$T/api/groups-1.json"

# start.sh refuses a public group and stops a running runner.
printf '#!/bin/sh\nexit 0\n' > "$T/home/.nself/runners/github-runner/run.sh"; chmod +x "$T/home/.nself/runners/github-runner/run.sh"
sleep 300 & sleeper=$!
echo "$sleeper" > "$T/home/.nself/logs/plugins/github-runner/runner.pid"
rc=0; out=$(bash "$PLUGIN_DIR/actions/start.sh" 2>&1) || rc=$?
sleep 0.2
[[ "$rc" == 1 && "$out" == *runner_group_public* ]] && pass "start.sh refuses a public group" || fail "start.sh: rc=$rc out=$out"
if kill -0 "$sleeper" 2>/dev/null; then fail "start.sh left the runner running"; kill "$sleeper" 2>/dev/null || true; else pass "start.sh stops a runner whose group turned public"; fi
wait "$sleeper" 2>/dev/null || true

# trust_stop_runner kills the whole process tree, not just the wrapper.
bash -c 'sleep 300 & echo $! > "$1"; wait' _ "$T/child.pid" & wrapper=$!
for _ in $(seq 1 100); do [[ -s "$T/child.pid" ]] && break; sleep 0.1; done
sleep 0.2
echo "$wrapper" > "$T/tree.pid"
trust_stop_runner "$T/tree.pid" "no-such-unit" > /dev/null
child=$(cat "$T/child.pid")
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  state=$(ps -o stat= -p "$child" 2>/dev/null | tr -d ' ' || true)
  [[ -z "$state" || "$state" == Z* ]] && break
  sleep 0.2
done
if [[ -n "$state" && "$state" != Z* ]]; then fail "trust_stop_runner left a child process running"; kill "$child" 2>/dev/null || true; else pass "trust_stop_runner stops the child processes too"; fi
wait "$wrapper" 2>/dev/null || true

# A child that ignores TERM and outlives its wrapper (reparented) must still be
# killed: the KILL pass uses the tree listed before the first signal.
spawn_orphan_prone_tree() { # PIDFILE_BASENAME: wrapper dies on TERM, child ignores it
  bash -c 'sh -c "trap \"\" TERM; exec sleep 300" & echo $! > "$1"; wait' _ "$T/$1.child" & wrapper=$!
  for _ in $(seq 1 100); do [[ -s "$T/$1.child" ]] && break; sleep 0.1; done
  sleep 0.3
  echo "$wrapper" > "$T/$1.pid"
  child=$(cat "$T/$1.child")
}
child_gone() { # PID
  local st i
  for i in $(seq 1 25); do
    st=$(ps -o stat= -p "$1" 2>/dev/null | tr -d ' ' || true)
    [[ -z "$st" || "$st" == Z* ]] && return 0
    sleep 0.2
  done
  return 1
}
spawn_orphan_prone_tree orphan1
rc=0; TRUST_STOP_GRACE=1 trust_stop_runner "$T/orphan1.pid" "no-such-unit" > /dev/null 2>&1 || rc=$?
if child_gone "$child" && [[ "$rc" == 0 ]]; then pass "trust_stop_runner KILLs a TERM-ignoring child that outlives its wrapper"; else fail "trust_stop_runner left a reparented child (rc=$rc)"; kill -KILL "$child" 2>/dev/null || true; fi
wait "$wrapper" 2>/dev/null || true

spawn_orphan_prone_tree orphan2
mkdir -p "$T/home/.nself/logs/plugins/github-runner"
cp "$T/orphan2.pid" "$T/home/.nself/logs/plugins/github-runner/runner.pid"
rc=0; TRUST_STOP_GRACE=1 bash "$PLUGIN_DIR/actions/stop.sh" > /dev/null 2>&1 || rc=$?
if child_gone "$child" && [[ "$rc" == 0 ]]; then pass "stop.sh KILLs a TERM-ignoring child that outlives its wrapper"; else fail "stop.sh left a reparented child (rc=$rc)"; kill -KILL "$child" 2>/dev/null || true; fi
wait "$wrapper" 2>/dev/null || true
rm -f "$T/home/.nself/logs/plugins/github-runner/runner.pid"

# systemctl stop/disable failures must not be reported as success.
mkdir -p "$T/failbin" "$T/okbin"
printf '#!/bin/sh\nexec "$@"\n' > "$T/failbin/sudo"
printf '#!/bin/sh\ncase "$1" in is-active|is-enabled) exit 0 ;; *) exit 1 ;; esac\n' > "$T/failbin/systemctl"
cp "$T/failbin/sudo" "$T/okbin/sudo"
printf '#!/bin/sh\nexit 0\n' > "$T/okbin/systemctl"
chmod +x "$T/failbin/sudo" "$T/failbin/systemctl" "$T/okbin/sudo" "$T/okbin/systemctl"
rc=0; out=$(PATH="$T/failbin:$PATH" trust_stop_runner "$T/none.pid" "x.service" 2>&1) || rc=$?
[[ "$rc" != 0 ]] && pass "trust_stop_runner fails when systemctl stop and disable fail" || fail "trust_stop_runner returned 0 after systemctl failures"
[[ "$out" == *"could not stop systemd service"* && "$out" == *"could not disable systemd service"* ]] && pass "trust_stop_runner says which systemctl step failed" || fail "failure message: $out"
[[ "$out" != *"stopped systemd service"* && "$out" != *"disabled systemd service"* ]] && pass "trust_stop_runner does not claim a failed stop/disable" || fail "claimed success: $out"
rc=0; out=$(PATH="$T/okbin:$PATH" trust_stop_runner "$T/none.pid" "x.service" 2>&1) || rc=$?
[[ "$rc" == 0 && "$out" == *"stopped systemd service"* && "$out" == *"disabled systemd service"* ]] && pass "trust_stop_runner succeeds when systemctl succeeds" || fail "ok path: rc=$rc out=$out"
rc=0; out=$(PATH="$T/failbin:$PATH" bash "$PLUGIN_DIR/actions/start.sh" 2>&1) || rc=$?
[[ "$rc" == 1 && "$out" == *runner_group_public* && "$out" == *"could NOT be fully stopped"* ]] && pass "start.sh reports a runner it could not stop" || fail "start.sh with failing systemctl: rc=$rc out=$out"

# status.sh stops the runner too and reports deploy_host_warning only in prod-like ENV.
sleep 300 & sleeper=$!
echo "$sleeper" > "$T/home/.nself/logs/plugins/github-runner/runner.pid"
out=$(ENV=prod bash "$PLUGIN_DIR/actions/status.sh" 2>&1) || true
sleep 0.2
[[ "$out" == *runner_group_public* && "$out" == *deploy_host_warning* ]] && pass "status.sh reports the refusal and deploy_host_warning" || fail "status.sh: $out"
if kill -0 "$sleeper" 2>/dev/null; then fail "status.sh left the runner running"; kill "$sleeper" 2>/dev/null || true; else pass "status.sh stops a runner whose group turned public"; fi
wait "$sleeper" 2>/dev/null || true
out=$(ENV=dev bash "$PLUGIN_DIR/actions/status.sh" 2>&1) || true
[[ "$out" != *deploy_host_warning* ]] && pass "status.sh has no deploy_host_warning outside prod-like ENV" || fail "status.sh warned in dev"

# =============================================================================
# Summary
# =============================================================================
echo ""
TOTAL=$((PASS + FAIL))
echo "Results: $PASS/$TOTAL passed"
[[ $FAIL -eq 0 ]] && echo "All tests passed." || { echo "$FAIL test(s) failed."; exit 1; }
