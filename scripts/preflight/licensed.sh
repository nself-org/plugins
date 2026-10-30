#!/usr/bin/env bash
# licensed.sh - served-bytes preflight, licensed tier (P7-PLUG-39).
#
# Downloads, for every licensed registry entry, the tarball ping.nself.org
# serves with the scoped, expiring, revocable CI licence, and applies the
# P7-PLUG-02 pipeline and classes (lib.sh). Adds class `entitlement`: the
# download answered 401/403 although the CI licence should cover the entry.
# A skip (for example --no-build) is not a pass.
#
# Usage:
#   scripts/preflight/licensed.sh --out report.json [--md summary.md] [--strict]
#       [--no-build] [--registry FILE] [--ping-url URL] [--log-dir DIR]
# The key is read from the environment variable NSELF_CI_LICENSE_KEY only, is
# sent as X-License-Key on the first hop (never to a redirect target), and is
# never printed. Registry default: $NSELF/plugins-pro/registry.json.
# Exit: 0 report written, 1 --strict and not pass, 2 usage/infrastructure,
# 3 coverage failure (fewer entries measured than the registry lists).
# shellcheck disable=SC2034
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "$SCRIPT_DIR/lib.sh"
# shellcheck source=docker.sh
. "$SCRIPT_DIR/docker.sh"

OUT="" MD="" STRICT=0 REGISTRY="${NSELF:+$NSELF/plugins-pro/registry.json}"
PING_URL="https://ping.nself.org"
PF_NO_BUILD=0 PF_LOG_DIR="" PF_BUILD_CACHE=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --out) OUT="${2:-}"; shift 2 ;;
    --md) MD="${2:-}"; shift 2 ;;
    --strict) STRICT=1; shift ;;
    --no-build) PF_NO_BUILD=1; shift ;;
    --registry) REGISTRY="${2:-}"; shift 2 ;;
    --ping-url) PING_URL="${2:-}"; shift 2 ;;
    --log-dir) PF_LOG_DIR="${2:-}"; shift 2 ;;
    -h | --help) sed -n 2,17p "$0"; exit 0 ;;
    *) pf_die "unknown argument: $1" ;;
  esac
done
[ -n "$OUT" ] || pf_die "--out FILE is required"
[ -n "$REGISTRY" ] && [ -f "$REGISTRY" ] || pf_die "licensed registry not found (set NSELF or --registry)"
[ -n "${NSELF_CI_LICENSE_KEY:-}" ] || pf_die "NSELF_CI_LICENSE_KEY is not set"
PING_URL="${PING_URL%/}"
[ -n "$MD" ] || MD="${OUT%.json}.md"
rm -f "$OUT" "$MD"

pf_need curl jq tar awk find
[ "$PF_NO_BUILD" = 1 ] || { pf_need docker; docker info >/dev/null 2>&1 || pf_die "docker daemon not reachable (use --no-build only to record skips)"; }
PF_RUN_ID="pfl-$(date -u +%Y%m%dT%H%M%SZ)-$$"
PF_BUILDER=""
PF_TIMEOUT_BIN=""
for t in timeout gtimeout; do command -v "$t" >/dev/null 2>&1 && { PF_TIMEOUT_BIN="$t"; break; }; done

PF_WORK="$(mktemp -d "${TMPDIR:-/tmp}/preflight-licensed.XXXXXX")"
trap '[ "$PF_NO_BUILD" = 1 ] || pf_docker_cleanup || true; rm -rf "$PF_WORK" "$OUT.tmp" "$MD.tmp"' EXIT
mkdir -p "$PF_WORK/dl" "$PF_WORK/build" "$PF_WORK/logs"
PF_ENTRIES="$PF_WORK/entries.jsonl"
: >"$PF_ENTRIES"
PF_MAIN_TABLE="$PF_WORK/main.tsv"
PF_SERVED_TABLE="$PF_MAIN_TABLE" # no second registry: drift is not measured here

# The header lives in a 0600 file so the key never appears in an argv.
HDR="$PF_WORK/auth.hdr"
(umask 077; printf 'X-License-Key: %s\n' "$NSELF_CI_LICENSE_KEY" >"$HDR")

jq -e '.plugins | type == "object"' "$REGISTRY" >/dev/null || pf_die "registry has no plugins object"
# Entries are fetched through ping, whatever host the registry's download_url names.
jq -r '.plugins | to_entries | map(select((.value.requires_license // true) == true)) | .[]
  | [.key, (.value.version // "-"), (if .value.installable == false then "false" else "true" end),
     ("PING/plugins/" + .key + "/download"), (.value.checksum // "-")] | @tsv' "$REGISTRY" \
  | LC_ALL=C sort | sed "s#PING#$PING_URL#" >"$PF_MAIN_TABLE"
EXPECTED=$(wc -l <"$PF_MAIN_TABLE" | tr -d ' ')

# Override: the first hop carries the key; a redirect target gets a plain GET.
pf_fetch_cached() {
  local key loc
  key=$(pf_sha256_str "$1")
  PF_DL_FILE="$PF_WORK/dl/$key.bin"
  if [ -f "$PF_WORK/dl/$key.code" ]; then PF_DL_CODE=$(cat "$PF_WORK/dl/$key.code"); return 0; fi
  PF_DL_CODE=$(pf_curl_code "$PF_DL_FILE" "$1" -H "@$HDR" -D "$PF_WORK/dl/$key.hdr")
  case "$PF_DL_CODE" in
    301 | 302 | 303 | 307 | 308)
      loc=$(tr -d '\r' <"$PF_WORK/dl/$key.hdr" | awk 'tolower($1) == "location:" {print $2; exit}')
      case "$loc" in /*) loc="${1%%://*}://$(printf '%s' "${1#*://}" | cut -d/ -f1)$loc" ;; esac
      PF_DL_CODE=$(pf_curl_code "$PF_DL_FILE" "$loc" -L)
      ;;
  esac
  [ "$PF_DL_CODE" = 200 ] || rm -f "$PF_DL_FILE"
  printf '%s' "$PF_DL_CODE" >"$PF_WORK/dl/$key.code"
}

# Wrap pf_done: a 401/403 download is class entitlement, never skipped or 404.
eval "pf_done_base() $(declare -f pf_done | tail -n +2)"
pf_done() {
  if [ "$1" = fail ] && [ "$2" = 404 ] && { [ "$dl" = 401 ] || [ "$dl" = 403 ]; }; then
    pf_done_base fail entitlement "CI licence rejected (HTTP $dl) for an entry it should cover"
  else
    pf_done_base "$@"
  fi
}

n=0
while IFS=$'\t' read -r slug ver inst url sum <&3; do
  n=$((n + 1))
  pf_log "[licensed $n/$EXPECTED] $slug@$ver"
  pf_check_entry main "$slug" "$ver" "$inst" "$url" "$sum"
done 3<"$PF_MAIN_TABLE"

mkdir -p "$(dirname "$OUT")" "$(dirname "$MD")"
jq -S -s --arg run_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg ping "$PING_URL" --arg run_id "$PF_RUN_ID" \
  --argjson expected "$EXPECTED" --argjson nobuild "$PF_NO_BUILD" '
  (sort_by(.slug) | map(.source = "licensed")) as $e
  | {schema: "preflight-licensed/1",
     run: {run_at: $run_at, run_id: $run_id, ping_url: $ping, no_build: ($nobuild == 1)},
     summary: {expected: $expected,
               coverage: {ok: (($e | length) == $expected and $expected > 0)},
               counts: {pass: ($e | map(select(.result == "pass")) | length),
                        fail: ($e | map(select(.result == "fail")) | length),
                        skip: ($e | map(select(.result == "skip")) | length)},
               by_class: ($e | map(select(.result != "pass")) | group_by(.class) | map({(.[0].class // "none"): length}) | add // {}),
               pass: (($e | length) == $expected and $expected > 0 and ($e | map(select(.result != "pass")) | length) == 0)},
     entries: $e}' "$PF_ENTRIES" >"$OUT.tmp"
mv "$OUT.tmp" "$OUT"
{
  printf '# Licensed preflight\n\n'
  jq -r '"Entries: \(.summary.expected); pass \(.summary.counts.pass), fail \(.summary.counts.fail), skip \(.summary.counts.skip). Overall pass: \(.summary.pass).\n",
         "| slug | result | class | reason |", "|---|---|---|---|",
         (.entries[] | "| \(.slug) | \(.result) | \(.class // "-") | \(.reason | gsub("\\|"; "/")) |")' "$OUT"
} >"$MD.tmp"
mv "$MD.tmp" "$MD"
pf_log "wrote $OUT and $MD"

jq -e '.summary.coverage.ok' "$OUT" >/dev/null || { pf_log "ERROR: coverage failure ($(jq '.entries | length' "$OUT") of $EXPECTED)"; exit 3; }
if [ "$STRICT" = 1 ] && ! jq -e '.summary.pass' "$OUT" >/dev/null; then
  pf_log "FAIL: $(jq -c '.summary.counts' "$OUT")"
  exit 1
fi
exit 0
