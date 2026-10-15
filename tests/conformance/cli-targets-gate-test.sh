#!/usr/bin/env bash
# Purpose: prove exception use follows current target blocks (D-0671), without network or builds.
# Inputs: Bash, Go, jq; temporary manifests supply a fake normalizer's target data.
# Outputs: pass lines or a nonzero exit; all fixture files are removed on exit.
set -eu
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/cli" "$work/plugins/alpha"
printf 'module fixture\ngo 1.20\n' > "$work/cli/go.mod"
printf '{"verbs":["init"]}\n' > "$work/core.json"
printf '{"targets":[["nself-alpha","alpha"]]}\n' > "$work/plugins/alpha/plugin.json"
cat > "$work/targets" <<'TOOL'
#!/usr/bin/env bash
set -eu
jq -e '.refused != true' "$2" >/dev/null || exit 1
jq -r '.targets[]? | @tsv' "$2"
TOOL
chmod +x "$work/targets"
export CLI_TOOLS_SRC="$work/cli" CLI_TARGETS_TOOL="$work/targets"
export CLI_CORE_REGISTRY="$work/core.json" CONF_TODAY=2026-10-15
unset GITHUB_STEP_SUMMARY

# Purpose: write one complete exception; inputs are rule, plugin and expiry.
exception() {
  printf 'exceptions:\n  - rule: %s\n    plugin: %s\n    reason: fixture\n    owner_ref: D-0671\n    expires: %s\n' \
    "$1" "$2" "$3" > "$work/exceptions.yaml"
}

# Purpose: check exit status and diagnostic, so setup failures cannot satisfy a negative proof.
expect() {
  local name=$1 want=$2 message=$3 got=0
  bash "$ROOT/scripts/release/cli-targets-gate.sh" --root "$work/plugins" \
    --exceptions "$work/exceptions.yaml" --out "$work/out" > "$work/log" 2>&1 || got=$?
  if [ "$got" -ne "$want" ] || ! grep -qF "$message" "$work/log"; then
    printf 'FAIL %s: exit %s, want %s and %s\n' "$name" "$got" "$want" "$message"
    cat "$work/log"
    exit 1
  fi
  printf 'pass cli-targets: %s\n' "$name"
}

exception cli-targets alpha 2026-12-31
expect 'unused active exception fails' 1 'FAIL cli-targets alpha: unused'

printf '{"targets":[]}\n' > "$work/plugins/alpha/plugin.json"
expect 'commandless plugin cannot use an exception' 1 'FAIL cli-targets alpha: unused'

exception cli-targets missing 2026-12-31
expect 'missing plugin cannot use an exception' 1 'FAIL cli-targets missing: unused'

printf '{"targets":[["nself-init","init"]]}\n' > "$work/plugins/alpha/plugin.json"
exception cli-targets alpha 2026-12-31
expect 'blocked plugin uses its exception' 0 'BLOCKED alpha (excepted, fix D-0671)'
[ ! -s "$work/out/alpha.tsv" ]

printf '{"refused":true}\n' > "$work/plugins/alpha/plugin.json"
expect 'normalizer refusal uses its exception' 0 'BLOCKED alpha (excepted, fix D-0671)'

exception cli-targets alpha 2026-10-14
expect 'expired exception still fails' 1 'cli-targets exception expired'

printf '{"targets":[["nself-alpha","alpha"]]}\n' > "$work/plugins/alpha/plugin.json"
exception docker-socket alpha 2026-12-31
expect 'other exception rules are unaffected' 0 'exit 0'
grep -qxF "$(printf 'nself-alpha\talpha')" "$work/out/alpha.tsv"

printf 'exceptions: []\n' > "$work/exceptions.yaml"
expect 'clean plugin without an exception passes' 0 'exit 0'
