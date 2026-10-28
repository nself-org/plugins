#!/usr/bin/env bash
# D-0671: cli-targets exceptions must describe a plugin the release gate still blocks.
# Uses the pinned normalizer and a fixture core registry; no Docker is needed.
set -eu
repo=$(cd "$(dirname "$0")/../.." && pwd)
. "$repo/scripts/conformance/lib.sh"
work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
CLI_TOOLS_SRC=$(cli_src)
export CLI_TOOLS_SRC
if [ -z "${CLI_TARGETS_TOOL:-}" ]; then CLI_TARGETS_TOOL=$(cli_tool manifestv2migrate); fi
export CLI_TARGETS_TOOL CONF_TODAY=2026-10-28
mkdir -p "$work/root/cli-fixture"
cp "$repo/tests/fixtures/manifest-v2-cli/plugin.json" "$work/root/cli-fixture/plugin.json"
printf '{"verbs":["init"]}\n' > "$work/core.json"
export CLI_CORE_REGISTRY="$work/core.json"

exception() {
  printf 'exceptions:\n  - rule: %s\n    plugin: %s\n    reason: "fixture"\n    owner_ref: D-0671\n    expires: 2026-12-31\n' "$1" "$2" > "$work/exceptions.yaml"
}
run_gate() {
  if bash "$repo/scripts/release/cli-targets-gate.sh" --root "$work/root" --exceptions "$work/exceptions.yaml" --out "$work/out" > "$work/result" 2>&1; then
    actual=0
  else
    actual=$?
  fi
  if [ "$actual" -ne "$1" ]; then
    cat "$work/result" >&2; echo "FAIL $2: exit $actual, expected $1" >&2; exit 1
  fi
  echo "ok: $2"
}

printf 'exceptions:\n' > "$work/exceptions.yaml"
run_gate 0 'clean plugin needs no exception'
grep -qx $'nself-cli-fixture\tcli-fixture' "$work/out/cli-fixture.tsv"

exception cli-targets cli-fixture
run_gate 1 'unused exception for a released plugin is rejected'
grep -q 'FAIL cli-targets cli-fixture: unused unexpired exception' "$work/result"

exception cli-targets absent
run_gate 1 'unused exception for an absent plugin is rejected'
grep -q 'FAIL cli-targets absent: unused unexpired exception' "$work/result"

exception migration-idempotent cli-fixture
run_gate 0 'other exception rules are unaffected'

# Turn the same fixture into a core-command collision, so its exception is needed.
printf '{"verbs":["cli-fixture"]}\n' > "$work/core.json"
printf 'exceptions:\n' > "$work/exceptions.yaml"
run_gate 1 'blocked plugin still needs an exception'
grep -q 'blocked and no unexpired cli-targets exception' "$work/result"
exception cli-targets cli-fixture
run_gate 0 'needed unexpired exception still permits the gate'
[ ! -s "$work/out/cli-fixture.tsv" ]

CONF_TODAY=2027-01-01 run_gate 1 'expired exception is rejected by existing validation'
grep -q 'cli-targets exception expired 2026-12-31' "$work/result"
