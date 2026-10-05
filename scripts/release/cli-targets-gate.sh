#!/usr/bin/env bash
# cli-targets-gate.sh [--root DIR] [--exceptions FILE] --out DIR: compute the binary targets of every
# plugin under DIR (default free/) with cli-targets.sh and write the ones the release may build to
# OUT/<plugin>.tsv (empty file: builds nothing). P7-PLUG-55.
# A plugin is BLOCKED, and ships no binary, when
#   - the pinned normalizer refuses its plugin.json,
#   - a command is a core verb, an existing top-level command or one of their aliases (rm, up, down, ...)
#     of the pinned cli (read from that commit's .github/command-registry.json: .verbs plus every
#     command whose parent is "nself", with its aliases),
#   - a command is outside ^[a-z][a-z0-9-]*$ (the binary is nself-<command>), or
#   - another plugin yields the same binary.
# Every blocked plugin needs an unexpired entry (rule cli-targets, plugin, reason, owner_ref naming
# the Ticket that fixes its manifest, expires within the PLUG-03 cap) in the exceptions file, else the
# run fails. Blocked plugins are listed on stdout and in $GITHUB_STEP_SUMMARY.
# Data checks over the tool's output only: no manifest is parsed here.
# Env: CLI_TARGETS_TOOL (built manifestv2migrate), CLI_CORE_REGISTRY (registry json; default from the
# pinned cli source), CONF_TODAY (test date). Exit: 0 ok, 1 an unlisted block or a bad exceptions file, 2 usage.
set -u
. "$(dirname "$0")/../conformance/lib.sh"
root=$CONF_ROOT/free; exc=$CONF_DIR/exceptions.yaml; out=
while [ $# -gt 0 ]; do case $1 in
  --root) root=$2; shift;; --exceptions) exc=$2; shift;; --out) out=$2; shift;;
  *) echo "usage: cli-targets-gate.sh [--root DIR] [--exceptions FILE] --out DIR" >&2; exit 2;; esac; shift; done
[ -n "$out" ] || { echo "cli-targets-gate: --out is required" >&2; exit 2; }
mkdir -p "$out"; work=$(mktemp -d); rc=0
# The runner's Go must be at least the pinned cli's go.mod directive (GOTOOLCHAIN=local forbids a
# silent toolchain download), or the tool build below fails with an unclear message.
need=$(sed -n 's/^go //p' "$(cli_src)/go.mod" | head -1); have=$(cd "$(cli_src)" && go env GOVERSION 2>/dev/null | sed 's/^go//')
[ -n "$need" ] && [ -n "$have" ] && [ "$(printf '%s\n%s\n' "$need" "$have" | sort -V | head -1)" = "$need" ] \
  || { echo "cli-targets-gate: Go $have is older than the pinned cli go.mod needs ($need)" >&2; exit 1; }
[ -n "${CLI_TARGETS_TOOL:-}" ] || CLI_TARGETS_TOOL=$(cli_tool manifestv2migrate) || exit 1
export CLI_TARGETS_TOOL
reg=${CLI_CORE_REGISTRY:-$(cli_src)/.github/command-registry.json}
[ -f "$reg" ] || { echo "cli-targets-gate: core command registry missing: $reg" >&2; exit 1; }
jq -r '[(.verbs // [])[], ((.commands // [])[] | select(.parent == "nself") | .name, (.aliases // [])[])] | unique[]' "$reg" > "$work/core"
[ -s "$work/core" ] || { echo "cli-targets-gate: no core verbs read from $reg (a gate that checks nothing)" >&2; exit 1; }
bash "$CONF_DIR/rules/exceptions.sh" "$exc" || rc=1
tab=$(printf '\t')
# Pass 1: targets per plugin, or a refusal.
for dir in "$root"/*/; do
  p=$(basename "$dir"); [ -f "$dir/plugin.json" ] || continue
  if ! bash "$CONF_ROOT/scripts/release/cli-targets.sh" "$dir" > "$work/$p.tsv" 2> "$work/$p.err"; then
    printf 'normalizer refused: %s\n' "$(head -1 "$work/$p.err" | cut -c1-160)" > "$work/$p.block"; : > "$work/$p.tsv"
  fi
done
# Pass 2: per-plugin data checks. Name and core-verb blocks first; the duplicate check then runs only
# among plugins still clean, so a plugin that is blocked anyway (and ships nothing) cannot take a
# legitimate plugin's binary down with it.
for f in "$work"/*.tsv; do
  p=$(basename "$f" .tsv)
  while IFS="$tab" read -r bin cmd; do
    [ -n "$bin" ] || continue
    printf '%s' "$cmd" | grep -Eq '^[a-z][a-z0-9-]*$' || echo "name '$cmd' is outside ^[a-z][a-z0-9-]*\$" >> "$work/$p.block"
    grep -qx -- "${cmd%% *}" "$work/core" && echo "command '$cmd' is a core verb or top-level command of the pinned cli" >> "$work/$p.block"
  done < "$f"
done
for f in "$work"/*.tsv; do
  p=$(basename "$f" .tsv); [ -s "$work/$p.block" ] || cut -f1 "$f" | sed "s|\$|$tab$p|" >> "$work/clean"
done
for f in "$work"/*.tsv; do
  p=$(basename "$f" .tsv); [ -s "$work/$p.block" ] && continue
  while IFS="$tab" read -r bin cmd; do
    [ -n "$bin" ] || continue
    o=$(awk -F'\t' -v b="$bin" -v p="$p" '$1 == b && $2 != p { printf "%s ", $2 }' "$work/clean")
    [ -z "$o" ] || echo "binary $bin is also yielded by $o" >> "$work/$p.block"
  done < "$f"
done
# Pass 3: blocked plugins need an exception; others are released.
blocked=0; summary="## Blocked CLI binary targets (P7-PLUG-55)"$'\n'
for f in "$work"/*.tsv; do
  p=$(basename "$f" .tsv)
  if [ -s "$work/$p.block" ]; then
    reasons=$(sort -u "$work/$p.block" | head -3 | tr '\n' ';')
    : > "$out/$p.tsv"
    if exception_active "$exc" cli-targets "$p"; then
      ref=$(yaml_rows "$exc" rule plugin owner_ref | awk -F'\t' -v p="$p" '$1 == "cli-targets" && $2 == p { print $3 }' | head -1)
      echo "BLOCKED $p (excepted, fix $ref): $reasons"; summary="$summary- $p: blocked, fix $ref. $reasons"$'\n'; blocked=$((blocked + 1))
    else
      fail cli-targets "$p" "blocked and no unexpired cli-targets exception: $reasons"; rc=1
      summary="$summary- $p: BLOCKED WITHOUT AN EXCEPTION. $reasons"$'\n'
    fi
  else
    cp "$f" "$out/$p.tsv"
  fi
done
[ "$blocked" -gt 0 ] || summary="$summary- none"$'\n'
[ -z "${GITHUB_STEP_SUMMARY:-}" ] || printf '%s\n' "$summary" >> "$GITHUB_STEP_SUMMARY"
echo "cli-targets-gate: $(ls "$out" | wc -l | tr -d ' ') plugin(s), $blocked excepted block(s), exit $rc"
rm -rf "$work"
exit $rc
