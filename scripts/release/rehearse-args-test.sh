#!/usr/bin/env bash
# rehearse-args-test.sh: scripts/release/rehearse.sh must refuse a --tag or --out with no value (exit 2, a message naming the
# flag) instead of looping forever on it (D-1107: the failed `shift 2` was ignored). Each case runs under a watchdog, so a
# regression shows as a HUNG failure, not a stuck test. No key, no tarball and no network is touched: every case is refused
# during argument parsing, before the script reads anything else.
# Run from any directory: bash scripts/release/rehearse-args-test.sh
set -u
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
T="$(mktemp -d "${TMPDIR:-/tmp}/rehearse-args.XXXXXX")"; trap 'rm -rf "$T"' EXIT
fails=0

# case_rc <label> <want rc> <want message fragment> args...
case_rc() {
  local label=$1 want=$2 frag=$3; shift 3
  bash "$HERE/rehearse.sh" "$@" >"$T/out" 2>&1 &
  local pid=$! i=0
  while kill -0 "$pid" 2>/dev/null && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i + 1)); done
  if kill -0 "$pid" 2>/dev/null; then
    kill -9 "$pid" 2>/dev/null; wait "$pid" 2>/dev/null
    echo "FAIL $label: HUNG (no exit after 10s)"; fails=$((fails + 1)); return
  fi
  wait "$pid"; local rc=$?
  if [ "$rc" -eq "$want" ] && grep -qF -- "$frag" "$T/out"; then echo "ok   $label"
  else echo "FAIL $label: rc $rc, want $want with '$frag'"; sed 's/^/       /' "$T/out"; fails=$((fails + 1)); fi
}

case_rc "trailing --tag" 2 "--tag needs a value" --no-upload --out "$T/o" --tag
case_rc "trailing --out" 2 "--out needs a value" --no-upload --tag v0.0.0-rehearsal --out
case_rc "only --tag" 2 "--tag needs a value" --tag
case_rc "unknown argument" 2 "unknown argument" --nope
case_rc "missing --no-upload" 2 "usage:" --tag v0.0.0-rehearsal --out "$T/o"
[ "$fails" = 0 ] && echo "PASS rehearse-args" || { echo "FAILED: $fails case(s)"; exit 1; }
