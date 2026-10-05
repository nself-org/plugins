#!/usr/bin/env bash
# manifest-v2.sh [--dry-run] [--drop-dev-tool-keys] <plugin dir>: convert plugin.json to canonical manifest v2
# with generated compatibility keys (manifestv2migrate), set repository on free manifests and drop the dead keys
# in the cli drop list. Keys that plugins dev tools still read (dev-tool-keys.txt) are KEPT, so a manifest
# carrying one is reported blocked with the key named, until a Ticket whose scope includes the readers passes
# --drop-dev-tool-keys. A key neither mapped nor listed STOPS the run with the key named; nothing is written
# then. A converted manifest whose compose fragment is missing is reported manual and not written.
# Prints one line: "manifest <state> [detail]".
# State: converted | noop | blocked: <keys> | manual: <why>. Idempotent: a second run prints noop.
set -u
. "$(dirname "$0")/../conformance/lib.sh"
dry=0; dropdev=0
while [ $# -gt 1 ]; do case $1 in --dry-run) dry=1;; --drop-dev-tool-keys) dropdev=1;; esac; shift; done
dir=$1; f=$dir/plugin.json
tool=$(cli_tool manifestv2migrate) || { echo "manifest blocked: cannot build manifestv2migrate"; exit 1; }
dead=$(cli_src)/tools/manifestv2migrate/dead-keys.txt
tmp=$(mktemp); out=$(mktemp); err=$(mktemp); trap 'rm -f "$tmp" "$out" "$err"' EXIT
devkeys=; [ $dropdev -eq 1 ] && devkeys=$(awk '!/^[[:space:]]*(#|$)/ { printf "%s ", $1 }' "$CONF_ROOT/scripts/codemods/dev-tool-keys.txt")
prep='reduce ($keys | split(" ") | map(select(length > 0)))[] as $k (.; del(.[$k]))
  | if (if .manifest_version == 2 then (.license // "free") != "licensed" else ((.isCommercial // false) | not) and ((.tier // "free") == "free") end) then .repository = $repo else . end'
dropped=$(jq -r --arg keys "$devkeys" '. as $d | ($keys | split(" ") | map(select(length > 0) | select(. as $k | $d | has($k)))) | join(",")' "$f")
jq --arg keys "$devkeys" --arg repo "$FREE_REPO_URL" "$prep" "$f" > "$tmp" || { echo "manifest blocked: plugin.json is not valid JSON"; exit 1; }
run_tool() { "$tool" -in "$tmp" -drop "$dead" > "$out" 2> "$err"; }
if ! run_tool; then
  # env / env_vars have a lossy v1 shape; the shared envVars key keeps the documentation, so drop them only then.
  if [ $dropdev -eq 1 ] && grep -qE '^\s+\$\.(env|env_vars) ' "$err" && jq -e '(.envVars // null) != null' "$tmp" >/dev/null; then
    jq 'del(.env, .env_vars)' "$tmp" > "$tmp.2" && mv "$tmp.2" "$tmp"; dropped="${dropped:+$dropped,}env,env_vars(envVars kept)"
    run_tool
  fi
fi
if [ -s "$out" ] && [ ! -s "$err" ] || { [ -s "$out" ] && ! grep -q refusing "$err"; }; then :; else
  keys=$(grep -oE '^\s+\$\.[A-Za-z0-9_]+' "$err" | tr -d ' ' | sed 's/^\$\.//' | sort -u | tr '\n' ',' | sed 's/,$//')
  echo "manifest blocked: ${keys:-$(head -1 "$err")}"; exit 1
fi
dead_keys=$(grep -oE '^\s+\$\.[A-Za-z0-9_]+' "$err" | tr -d ' ' | sed 's/^\$\.//' | sort -u | tr '\n' ',' | sed 's/,$//')
cp "$out" "$tmp"; "$tool" -check -in "$tmp" >/dev/null 2>"$err" || { echo "manifest blocked: converted file fails -check: $(head -1 "$err")"; exit 1; }
detail=""; [ -n "$dropped" ] && detail=" dropped=$dropped"; [ -n "$dead_keys" ] && detail="$detail dead=$dead_keys"
if [ "$(jq -r '.service.kind // ""' "$tmp")" = compose ] && [ -z "$(jq -r '.service.image // empty' "$tmp")" ]; then
  frag=$(jq -r '.service.compose // ""' "$tmp")
  [ -n "$frag" ] && [ -f "$dir/$frag" ] || { echo "manifest manual: service.compose fragment '$frag' does not exist (not a compose plugin?); nothing written"; exit 0; }
fi
if cmp -s "$tmp" "$f"; then echo "manifest noop"; exit 0; fi
[ $dry -eq 0 ] && cp "$tmp" "$f"
echo "manifest converted$detail"
