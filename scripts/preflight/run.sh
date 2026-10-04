#!/usr/bin/env bash
# run.sh - served-bytes preflight, free tier (P7-PLUG-02).
#
# Measures what plugins.nself.org actually serves, per entry: route status,
# tarball bytes, sha256 vs the registry, extraction, plugin.json, fragment and
# Dockerfile presence, and a `docker build` from the extracted tree alone.
# Covers every entry of the served free registry and of registry.json at main.
#
# Usage:
#   scripts/preflight/run.sh --tier free --out report.json [--md summary.md]
#                            [--strict] [--no-build] [--log-dir DIR]
#                            [--build-cache DIR] [--served-url URL] [--main-ref REF]
#
# Exit: 0 report written (read summary.pass: exit 0 does NOT mean pass unless
# --strict), 1 with --strict when summary.pass is false, 2 usage or
# infrastructure error (any previous report at --out is deleted first, so a
# stale one is never left behind), 3 coverage failure (an empty registry, or
# fewer entries measured than the registry counts or lists as installable;
# summary.by_class.coverage names it and summary.pass is false).
# No credential, sequential. Docker work is scoped to this run (docker.sh).
# See scripts/preflight/README.md.

# PF_* variables are read by the sourced lib.sh.
# shellcheck disable=SC2034
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"
# shellcheck source=docker.sh
. "$SCRIPT_DIR/docker.sh"
# shellcheck source=report.sh
. "$SCRIPT_DIR/report.sh"

TIER="" OUT="" MD="" STRICT=0
PF_SERVED_URL="https://plugins.nself.org"
MAIN_REF="origin/main"
PF_NO_BUILD=0
PF_LOG_DIR=""
PF_BUILD_CACHE=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --tier) TIER="${2:-}"; shift 2 ;;
    --out) OUT="${2:-}"; shift 2 ;;
    --md) MD="${2:-}"; shift 2 ;;
    --strict) STRICT=1; shift ;;
    --no-build) PF_NO_BUILD=1; shift ;;
    --log-dir) PF_LOG_DIR="${2:-}"; shift 2 ;;
    --build-cache) PF_BUILD_CACHE="${2:-}"; shift 2 ;;
    --served-url) PF_SERVED_URL="${2:-}"; shift 2 ;;
    --main-ref) MAIN_REF="${2:-}"; shift 2 ;;
    -h | --help) sed -n 2,19p "$0"; exit 0 ;;
    *) pf_die "unknown argument: $1" ;;
  esac
done
[ "$TIER" = free ] || pf_die "--tier free is required (the licensed tier is P7-PLUG-39)"
[ -n "$OUT" ] || pf_die "--out FILE is required"
PF_SERVED_URL="${PF_SERVED_URL%/}"
[ -n "$MD" ] || MD="${OUT%.json}.md"
# Delete any previous report before anything can abort, so a stale one is never current.
rm -f "$OUT" "$MD"

pf_need curl jq tar awk find git
[ "$PF_NO_BUILD" = 1 ] || { pf_need docker; docker info >/dev/null 2>&1 || pf_die "docker daemon not reachable (use --no-build only to record skips)"; }
PF_RUN_ID="pf-$(date -u +%Y%m%dT%H%M%SZ)-$$"
PF_BUILDER=""
SCRIPT_COMMIT="$(git -C "$REPO_ROOT" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"
[ -z "$(git -C "$REPO_ROOT" status --porcelain -- scripts/preflight 2>/dev/null)" ] || SCRIPT_COMMIT="$SCRIPT_COMMIT-dirty"
PF_TIMEOUT_BIN=""
for t in timeout gtimeout; do command -v "$t" >/dev/null 2>&1 && { PF_TIMEOUT_BIN="$t"; break; }; done

PF_WORK="$(mktemp -d "${TMPDIR:-/tmp}/preflight.XXXXXX")"
trap 'pf_docker_cleanup; rm -rf "$PF_WORK" "$OUT.tmp" "$MD.tmp"' EXIT
mkdir -p "$PF_WORK/dl" "$PF_WORK/build" "$PF_WORK/logs"
PF_ENTRIES="$PF_WORK/entries.jsonl"
: >"$PF_ENTRIES"
PF_SERVED_TABLE="$PF_WORK/served.tsv"
PF_MAIN_TABLE="$PF_WORK/main.tsv"

pf_log "fetching served registry $PF_SERVED_URL/registry.json?tier=free"
code=$(pf_curl_code "$PF_WORK/served.json" "$PF_SERVED_URL/registry.json?tier=free")
[ "$code" = 200 ] || pf_die "served registry answered HTTP $code"
jq -e '.plugins | type == "array"' "$PF_WORK/served.json" >/dev/null || pf_die "served registry has no plugins array"

MAIN_SHA="$(git -C "$REPO_ROOT" rev-parse --verify --quiet "$MAIN_REF^{commit}")" || pf_die "ref $MAIN_REF not found"
git -C "$REPO_ROOT" show "$MAIN_REF:registry.json" >"$PF_WORK/main.json" || pf_die "registry.json missing at $MAIN_REF"
jq -e '.plugins | type == "object"' "$PF_WORK/main.json" >/dev/null || pf_die "main registry has no plugins object"

pf_registry_table "$PF_WORK/served.json" >"$PF_SERVED_TABLE"
pf_registry_table "$PF_WORK/main.json" >"$PF_MAIN_TABLE"
EXP_SERVED=$(jq -r '.pluginCount.free // (.plugins | length)' "$PF_WORK/served.json")
EXP_MAIN=$(jq -r '.plugins_count // (.plugins | length)' "$PF_WORK/main.json")
INS_SERVED=$(awk -F'\t' '$3 == "true"' "$PF_SERVED_TABLE" | wc -l | tr -d ' ')
INS_MAIN=$(awk -F'\t' '$3 == "true"' "$PF_MAIN_TABLE" | wc -l | tr -d ' ')
pf_log "run $PF_RUN_ID, script $SCRIPT_COMMIT; installable: served=$INS_SERVED main=$INS_MAIN"
pf_log "expected: served=$EXP_SERVED main=$EXP_MAIN (rows: $(wc -l <"$PF_SERVED_TABLE" | tr -d ' ') / $(wc -l <"$PF_MAIN_TABLE" | tr -d ' '))"

for src in served main; do
  if [ "$src" = served ]; then tbl="$PF_SERVED_TABLE"; else tbl="$PF_MAIN_TABLE"; fi
  n=0
  total=$(wc -l <"$tbl" | tr -d ' ')
  while IFS=$'\t' read -r slug ver inst url sum <&3; do
    n=$((n + 1))
    pf_log "[$src $n/$total] $slug@$ver"
    pf_check_entry "$src" "$slug" "$ver" "$inst" "$url" "$sum"
  done 3<"$tbl"
done

mkdir -p "$(dirname "$OUT")" "$(dirname "$MD")"
RUN_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
jq -S -s --arg run_at "$RUN_AT" --arg served_url "$PF_SERVED_URL" --arg main_sha "$MAIN_SHA" \
  --arg main_ref "$MAIN_REF" --arg run_id "$PF_RUN_ID" --arg script_commit "$SCRIPT_COMMIT" \
  --argjson es "$EXP_SERVED" --argjson em "$EXP_MAIN" --argjson is "$INS_SERVED" --argjson im "$INS_MAIN" \
  --argjson nobuild "$PF_NO_BUILD" --argjson bcache "$([ -n "$PF_BUILD_CACHE" ] && echo true || echo false)" '
  (sort_by([.source, .slug])) as $e
  | ($e | map(select(.source == "served")) | length) as $ns
  | ($e | map(select(.source == "main")) | length) as $nm
  | ([ (if $es == 0 then "served registry is empty" else empty end),
       (if $em == 0 then "main registry is empty" else empty end),
       (if $ns != $es then "served entries \($ns) != registry count \($es)" else empty end),
       (if $nm != $em then "main entries \($nm) != registry count \($em)" else empty end),
       (if $ns < $is then "served entries \($ns) < installable \($is)" else empty end),
       (if $nm < $im then "main entries \($nm) < installable \($im)" else empty end) ]) as $cov
  | {header: {run_at: $run_at, run_id: $run_id, script_commit: $script_commit, tier: "free",
              served_url: $served_url, served_registry: ($served_url + "/registry.json?tier=free"),
              main_ref: $main_ref, main_sha: $main_sha,
              build_cache_used: $bcache, no_build: ($nobuild == 1)},
     summary: {expected: ($es + $em), expected_served: $es, expected_main: $em,
               installable_served: $is, installable_main: $im,
               coverage: {ok: (($cov | length) == 0), problems: $cov},
               counts: {pass: ($e | map(select(.result == "pass")) | length),
                        fail: ($e | map(select(.result == "fail")) | length),
                        skip: ($e | map(select(.result == "skip")) | length)},
               by_class: (($e | map(select(.result != "pass")) | group_by(.class) | map({(.[0].class): length}) | add // {})
                          + (if ($cov | length) > 0 then {coverage: ($cov | length)} else {} end)),
               pass: ((($e | map(select(.result != "pass")) | length) == 0) and (($cov | length) == 0) and (($e | length) > 0))},
     entries: $e}' "$PF_ENTRIES" >"$OUT.tmp"
mv "$OUT.tmp" "$OUT"
pf_render_md "$OUT" "$SCRIPT_DIR/owners.tsv" >"$MD.tmp" && mv "$MD.tmp" "$MD"
pf_log "wrote $OUT and $MD"

if ! jq -e '.summary.coverage.ok and ((.entries | length) == .summary.expected)' "$OUT" >/dev/null; then
  pf_log "ERROR: coverage failure: $(jq -c '.summary.coverage.problems' "$OUT") (entries $(jq '.entries | length' "$OUT"), expected $(jq '.summary.expected' "$OUT"))"
  exit 3
fi
if [ "$STRICT" = 1 ] && ! jq -e '.summary.pass' "$OUT" >/dev/null; then
  pf_log "FAIL: $(jq -c '.summary.counts' "$OUT")"
  exit 1
fi
exit 0
