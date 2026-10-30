#!/usr/bin/env bash
# ci-conformance.sh: acceptance A1-A10 of the AUTO fabric as one suite (P7-AUTO-25).
# Purpose: one entry point. Sim legs are Go tests under free/ci/test/sim/auto (and the foreground
#   remote tests of P7-AUTO-28 under free/ci/test/sim); hardware legs are scripts in
#   free/ci/scripts/conformance/. Every leg prints one `ci-conformance/v1` JSON line
#   {leg, pass, reason, evidence_ref, duration_ms} (plus status and kind).
# Usage:   bash free/ci/scripts/ci-conformance.sh [--sim|--hardware] [--json] [--list]
#   --sim       run the sim legs (the default)
#   --hardware  run the hardware legs
#   --json      one JSON line per leg and a final {"summary":...} line (without it, plain text)
#   --list      print the leg catalogue (id, kind, script) and run nothing
# Env:     TMPDIR (required for a run); CI_CONFORMANCE_BREAK=<leg> makes that sim leg look for a test
#   that does not exist, to prove a broken leg fails the suite; CI_CONFORMANCE_OUT keeps the logs.
# Exit:    0 every selected leg passed; 1 a leg failed; 2 usage; 3 no leg failed but one was skipped
#   (a skipped leg is listed and never counted as a pass).
# Constraints: bash 3.2; sim legs use CGO_ENABLED=0 and -mod=vendor; no host is named here.
set -u

KIND=sim; JSON=0; LIST=0
for a in "$@"; do
  case "$a" in
    --sim) KIND=sim ;; --hardware) KIND=hardware ;; --json) JSON=1 ;; --list) LIST=1 ;;
    *) echo "usage: ci-conformance.sh [--sim|--hardware] [--json] [--list]" >&2; exit 2 ;;
  esac
done

HERE=$(cd "$(dirname "$0")" && pwd)    # free/ci/scripts
CI_DIR=$(cd "$HERE/.." && pwd)         # free/ci

# Catalogue rows: id|test name|package (sim) ; hardware rows: id|script.
SIM_LEGS="A3|TestConformanceA3Watch|./test/sim/auto/
A4|TestConformanceA4Hosted|./test/sim/auto/
A5|TestConformanceA5Offload|./test/sim/auto/
A6|TestConformanceA6Refusal|./test/sim/auto/
A7|TestConformanceA7Race|./test/sim/auto/
A9|TestConformanceA9DeadNode|./test/sim/auto/
A10|TestConformanceA10Scope|./test/sim/auto/
F28|TestForegroundRemote|./test/sim/"
HW_LEGS="A1|a01.sh
A2|a02.sh
A3|a03.sh
A4|a04.sh
A5|a05.sh
A6|a06.sh
A7|a07.sh
A8|a08.sh
A9|a09.sh
A10|a10.sh"

jesc() { printf '%s' "$1" | tr -d '\000-\037' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }
now_ms() { echo $(( $(date +%s) * 1000 )); }

if [ "$LIST" = 1 ]; then
  list_one() { # id kind path
    if [ "$JSON" = 1 ]; then printf '{"schema":"ci-conformance/v1","id":"%s","kind":"%s","path":"%s"}\n' "$1" "$2" "$3"
    else printf '%s\t%s\t%s\n' "$1" "$2" "$3"; fi
  }
  echo "$SIM_LEGS" | while IFS='|' read -r id name pkg; do
    [ "$KIND" = sim ] && list_one "$id" sim "free/ci/${pkg#./}"
  done
  echo "$HW_LEGS" | while IFS='|' read -r id script; do
    [ "$KIND" = hardware ] && list_one "$id" hardware "free/ci/scripts/conformance/$script"
  done
  exit 0
fi

: "${TMPDIR:?TMPDIR must be set}"
OUT=${CI_CONFORMANCE_OUT:-$(mktemp -d "$TMPDIR/ci-conformance.XXXXXX")} || exit 2
mkdir -p "$OUT"
NPASS=0; NFAIL=0; NSKIP=0

report() { # id status reason evidence start
  local pass=false
  [ "$2" = pass ] && pass=true
  case "$2" in pass) NPASS=$((NPASS + 1)) ;; fail) NFAIL=$((NFAIL + 1)) ;; skip) NSKIP=$((NSKIP + 1)) ;; esac
  if [ "$JSON" = 1 ]; then
    printf '{"schema":"ci-conformance/v1","leg":"%s","kind":"%s","status":"%s","pass":%s,"reason":"%s","evidence_ref":"%s","duration_ms":%s}\n' \
      "$1" "$KIND" "$2" "$pass" "$(jesc "$3")" "$(jesc "$4")" "$(( $(now_ms) - $5 ))"
  else
    printf '%-4s %-4s %s\n' "$1" "$2" "$3"
  fi
}

run_sim() { # id test pkg
  local id=$1 name=$2 pkg=$3 log="$OUT/sim-$1.log" start
  start=$(now_ms)
  [ "${CI_CONFORMANCE_BREAK:-}" = "$id" ] && name="${name}DeliberatelyBroken"
  if (cd "$CI_DIR" && CGO_ENABLED=0 go test -mod=vendor -count=1 -v -run "^${name}" "$pkg") >"$log" 2>&1; then
    if grep -q -- "^--- PASS: ${name}" "$log"; then report "$id" pass "ok" "$log" "$start"
    else report "$id" fail "no test named ${name} ran" "$log" "$start"; fi
  else
    report "$id" fail "$(grep -m1 -E -- '--- FAIL|FAIL|panic' "$log" | cut -c1-200)" "$log" "$start"
  fi
}

run_hw() { # id script
  local id=$1 script="$HERE/conformance/$2" log="$OUT/hw-$1.log" start rc line
  start=$(now_ms)
  CI_CONF_OUT="$OUT" bash "$script" >"$log" 2>&1; rc=$?
  line=$(grep -E '^\{.*"leg"' "$log" | tail -1)
  reason=$(printf '%s' "$line" | sed -n 's/.*"reason":"\([^"]*\)".*/\1/p')
  case $rc in
    0) report "$id" pass "${reason:-ok}" "$log" "$start" ;;
    3) report "$id" skip "${reason:-a required host is missing}" "$log" "$start" ;;
    *) report "$id" fail "${reason:-leg exited $rc}" "$log" "$start" ;;
  esac
}

if [ "$KIND" = sim ]; then
  while IFS='|' read -r id name pkg; do run_sim "$id" "$name" "$pkg"; done <<EOF2
$SIM_LEGS
EOF2
else
  while IFS='|' read -r id script; do run_hw "$id" "$script"; done <<EOF2
$HW_LEGS
EOF2
fi

if [ "$JSON" = 1 ]; then
  printf '{"schema":"ci-conformance/v1","summary":{"kind":"%s","pass":%s,"fail":%s,"skip":%s}}\n' "$KIND" "$NPASS" "$NFAIL" "$NSKIP"
else
  echo "ci-conformance ($KIND): $NPASS passed, $NFAIL failed, $NSKIP skipped (logs: $OUT)"
fi
[ "$NFAIL" -gt 0 ] && exit 1
[ "$NSKIP" -gt 0 ] && exit 3
exit 0
