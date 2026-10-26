#!/usr/bin/env bash
# Self-test of the isolation-levels drill (P7-TRUST-25): runs `run` against fake uname, docker, go and nself-ci
# programs, so every outcome (pass, fail, prerequisite missing, skipped test, direct run) is checked without a
# container runtime. Not a drill (the drill job is `run`); run it by hand or from a verify line.
# Inputs:  NSELF_CI_BIN (optional): a real nself-ci; when set, the printed document is also checked with `evidence verify`.
# Outputs: "ok" lines and exit 0, or the first failed expectation and exit 1.
set -uo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
run=$here/run
work=$(mktemp -d "${TMPDIR:-/tmp}/isolation-levels-selftest.XXXXXX") || exit 2
trap 'rm -rf "$work"' EXIT
bin=$work/bin
mkdir -p "$bin"
fail() { echo "FAIL: $*" >&2; exit 1; }

cat >"$bin/uname" <<'F'
#!/bin/sh
case "$1" in -s) echo "${FAKE_OS:-Linux}" ;; -m) echo "${FAKE_ARCH:-x86_64}" ;; *) echo "${FAKE_OS:-Linux}" ;; esac
F
cat >"$bin/docker" <<'F'
#!/bin/sh
[ -z "${FAKE_NO_DOCKER:-}" ] || exit 1
case "$1" in
info) if [ "$2" = "--format" ]; then if [ -z "${FAKE_NO_RUNSC:-}" ]; then echo '{"runc":{},"runsc":{}}'; else echo '{"runc":{}}'; fi; fi; exit 0 ;;
image | pull) exit 0 ;;
ps) [ -z "${FAKE_PROBES:-}" ] || cat "$FAKE_PROBES" 2>/dev/null; exit 0 ;;
rm) echo "$*" >>"$FAKE_RM_LOG"; exit 0 ;;
esac
exit 0
F
cat >"$bin/go" <<'F'
#!/bin/sh
for a in "$@"; do pkg=$a; done
echo "=== RUN   TestX"
[ -z "${FAKE_GO_LEAK:-}" ] || echo "$FAKE_GO_LEAK" >>"$FAKE_PROBES"
case "$pkg" in
*"${FAKE_GO_FAIL:-NONE}"*) echo "--- FAIL: TestX (0.00s)"; echo "FAIL"; exit 1 ;;
*"${FAKE_GO_SKIP:-NONE}"*) echo "--- SKIP: TestX (0.00s)"; echo "ok"; exit 0 ;;
esac
echo "--- PASS: TestX (0.00s)"
echo "ok  $pkg"
F
cat >"$bin/nself-ci" <<'F'
#!/bin/sh
echo "args: $*" >"$FAKE_CI_LOG"
echo "repo: $NSELF_CI_REPO v15: $NSELF_V15" >>"$FAKE_CI_LOG"
F
chmod +x "$bin"/*

# drill NAME ENV...: run the inner drill with the fakes first on PATH; document in $work/NAME.json, status in NAME.rc.
drill() {
	local name=$1
	shift
	env PATH="$bin:$PATH" NSELF_CI_RUN_ID=selftest TMPDIR="$work" "$@" bash "$run" >"$work/$name.out" 2>"$work/$name.err"
	echo $? >"$work/$name.rc"
	tail -n 1 "$work/$name.out" >"$work/$name.json"
}
res() { jq -r --arg id "isolation.$2" '.checks[] | select(.id == $id) | .result' "$work/$1.json"; }
rc() { cat "$work/$1.rc"; }
verify() { # verify NAME: a real nself-ci, when given, reads the document as valid evidence (it is self-reported, so never admitted: exit 11)
	[ -z "${NSELF_CI_BIN:-}" ] && return 0
	local out
	out=$(NSELF_V15=1 "$NSELF_CI_BIN" evidence verify --file "$work/$1.json" --at-revision "$(jq -r .revision "$work/$1.json")" --json 2>&1)
	printf '%s' "$out" | jq -e --arg r "$(jq -r .result "$work/$1.json")" '.data.evidence.schema == "ci.evidence/v1" and .data.result == $r' >/dev/null || fail "$1: evidence verify refused the document: $(printf %s "$out" | head -c 300)"
}

# 1. Linux, everything works: four passing checks, exit 0.
drill pass
[ "$(rc pass)" = 0 ] || fail "pass: exit $(rc pass): $(cat "$work/pass.err")"
[ "$(jq -r '.result, .counts.pass, .counts.total' "$work/pass.json" | tr '\n' ' ')" = "pass 4 4 " ] || fail "pass: counts $(cat "$work/pass.json")"
for l in container sandboxed-container network hygiene; do [ "$(res pass $l)" = pass ] || fail "pass: $l"; done
[ "$(jq -r '.checks | map(.id) | join(",")' "$work/pass.json")" = "isolation.container,isolation.sandboxed-container,isolation.network,isolation.hygiene" ] || fail "pass: ids"
verify pass
echo "ok pass"

# 2. One package fails: that level fails, the others still pass, exit 1.
drill failing FAKE_GO_FAIL=netscope
[ "$(rc failing)" = 1 ] && [ "$(res failing network)" = fail ] && [ "$(res failing container)" = pass ] && [ "$(jq -r .result "$work/failing.json")" = fail ] || fail "failing: $(cat "$work/failing.json")"
verify failing
echo "ok failing"

# 3. A test that skipped proves nothing.
drill skipped FAKE_GO_SKIP=hygiene
[ "$(rc skipped)" = 1 ] && [ "$(res skipped hygiene)" = fail ] || fail "skipped: $(cat "$work/skipped.json")"
echo "ok skipped"

# 4. No container runtime: every level is error with prerequisite.missing, never pass.
drill nodocker FAKE_NO_DOCKER=1
[ "$(rc nodocker)" = 1 ] && [ "$(jq -r .result "$work/nodocker.json")" = error ] || fail "nodocker: $(cat "$work/nodocker.json")"
[ "$(jq -r '[.checks[] | select(.result == "error" and (.excerpt | startswith("reason: prerequisite.missing")))] | length' "$work/nodocker.json")" = 4 ] || fail "nodocker: reasons"
verify nodocker
echo "ok nodocker"

# 5. No gVisor runtime: only sandboxed-container errors.
drill norunsc FAKE_NO_RUNSC=1
[ "$(res norunsc sandboxed-container)" = error ] && [ "$(res norunsc container)" = pass ] || fail "norunsc: $(cat "$work/norunsc.json")"
echo "ok norunsc"

# 6. ISOLATION_DRILL_LEVELS narrows the levels; vm on Linux is an error naming D-0120; an unknown level is a usage error.
drill narrow ISOLATION_DRILL_LEVELS=network,vm
[ "$(jq -r '.checks | map(.id) | join(",")' "$work/narrow.json")" = "isolation.network,isolation.vm" ] && [ "$(res narrow vm)" = error ] || fail "narrow: $(cat "$work/narrow.json")"
grep -q "D-0120" "$work/narrow.json" || fail "narrow: vm excerpt does not name D-0120"
drill unknown ISOLATION_DRILL_LEVELS=bogus
[ "$(rc unknown)" = 2 ] || fail "unknown level: exit $(rc unknown)"
echo "ok narrow"

# 7. Apple-silicon Mac without Tart: vm only, error. A Mac with Tart and its image variables runs the tart tests.
drill mac FAKE_OS=Darwin FAKE_ARCH=arm64
[ "$(jq -r '.checks | map(.id) | join(",")' "$work/mac.json")" = "isolation.vm" ] && [ "$(res mac vm)" = error ] || fail "mac: $(cat "$work/mac.json")"
printf '#!/bin/sh\nexit 0\n' >"$bin/tart"
chmod +x "$bin/tart"
drill mactart FAKE_OS=Darwin FAKE_ARCH=arm64 NSELF_TEST_TART_IMAGE=img@sha256:0 NSELF_TEST_TART_SSH_KEY=/dev/null
[ "$(rc mactart)" = 0 ] && [ "$(res mactart vm)" = pass ] || fail "mactart: $(cat "$work/mactart.json")"
rm -f "$bin/tart"
echo "ok mac"

# 8. A host class with no level: no document, exit 0, nothing to record.
drill none FAKE_OS=FreeBSD FAKE_ARCH=amd64
[ "$(rc none)" = 0 ] && [ ! -s "$work/none.out" ] || fail "none: $(cat "$work/none.out")"
echo "ok none"

# 9. Run directly (outside a job): the drill hands itself to nself-ci as the drill-isolation-levels job.
env PATH="$bin:$PATH" NSELF_CI_BIN="$bin/nself-ci" FAKE_CI_LOG="$work/direct.log" bash "$run" >/dev/null 2>&1 || fail "direct: exit $?"
grep -q "^args: --no-status --pipeline drills --job drill-isolation-levels$" "$work/direct.log" || fail "direct: $(cat "$work/direct.log")"
grep -q "v15: 1" "$work/direct.log" || fail "direct env: $(cat "$work/direct.log")"
grep -q "repo: $(cd "$here/../../.." && pwd) " "$work/direct.log" || fail "direct repo: $(cat "$work/direct.log")"
echo "ok direct"
# 10. A probe container the legs leave behind is removed on exit; one that was there before the drill started is not.
printf 'oldprobe\n' >"$work/probes"
: >"$work/rm.log"
drill leak FAKE_PROBES="$work/probes" FAKE_RM_LOG="$work/rm.log" FAKE_GO_LEAK=newprobe
[ "$(rc leak)" = 0 ] || fail "leak: exit $(rc leak): $(cat "$work/leak.err")"
grep -q '^rm -f newprobe$' "$work/rm.log" || fail "leak: the probe left by the legs was not removed: $(cat "$work/rm.log")"
! grep -q oldprobe "$work/rm.log" || fail "leak: removed a probe container that was there before the drill: $(cat "$work/rm.log")"
echo "ok leak"
echo "all ok"
