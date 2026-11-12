#!/usr/bin/env bash
# test-licensed-registry-ref.sh
#
# Purpose: P7-WIRE-16 (D-1483). Licensed installs match bytes:
#   TestWire16RegistryFetchUsesRef   the registry worker reads licensed rows at ?ref=<LICENSED_REF> and serves none
#                                    when the var is unset (node test through the worker's real fetch entry), and
#                                    wrangler.toml pins the same non-branch ref in [vars] and [env.production.vars].
#   TestWire16RowEqualsBytesContract scripts/ci/licensed-registry-contract.sh passes only when every licensed row's
#                                    checksum equals the sha256 of the bytes ping serves (probe: a differing row fails).
# Inputs: none. Ping is a file:// tree of fixture tarballs; the registry is a fixture file.
# Outputs: "ok:"/"FAIL:" per case, final summary; exit 1 on any failure.
# Constraints: bash 3.2 compatible; no network, no docker; node >= 22.18 (built-in TypeScript type stripping).
set -u

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
worker="$repo/.workers/plugins-registry"
contract="$repo/scripts/ci/licensed-registry-contract.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/licensed-ref.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT
fail=0

pass() { echo "ok: $1"; }
flunk() { echo "FAIL: $1" >&2; fail=1; }

# --- TestWire16RegistryFetchUsesRef -------------------------------------------------------------------------
if out="$(cd "$worker" && node --test src/registry-ref.test.mjs 2>&1)"; then
  pass "TestWire16RegistryFetchUsesRef: worker fetches ?ref=<LICENSED_REF> and fails closed when unset"
else
  flunk "TestWire16RegistryFetchUsesRef: node test failed:
$out"
fi

# toml_var SECTION: LICENSED_REF's value inside that exact [section].
toml_var() {
  awk -v want="[$1]" '
    /^\[/ { in_sec = ($0 == want); next }
    in_sec && /^LICENSED_REF[ \t]*=/ { sub(/^[^=]*=[ \t]*"/, ""); sub(/".*$/, ""); print; exit }
  ' "$worker/wrangler.toml"
}
dev_ref="$(toml_var vars)"
prod_ref="$(toml_var env.production.vars)"
case "$prod_ref" in
  ""|main|master|HEAD) flunk "TestWire16RegistryFetchUsesRef: [env.production.vars] LICENSED_REF must pin a tag or SHA (got '$prod_ref')" ;;
  *[!A-Za-z0-9._/-]*) flunk "TestWire16RegistryFetchUsesRef: LICENSED_REF '$prod_ref' is not a git ref" ;;
  *) if [ "$dev_ref" = "$prod_ref" ]; then pass "TestWire16RegistryFetchUsesRef: wrangler.toml pins LICENSED_REF=$prod_ref in both var blocks"
     else flunk "TestWire16RegistryFetchUsesRef: [vars] LICENSED_REF '$dev_ref' differs from production '$prod_ref'"; fi ;;
esac

# --- TestWire16RowEqualsBytesContract -----------------------------------------------------------------------
ping="$tmp/ping"
mkdir -p "$ping/plugins/ai" "$ping/plugins/voice"
printf 'ai bytes built at v1.2.9\n' > "$ping/plugins/ai/download"
printf 'voice bytes built at v1.2.9\n' > "$ping/plugins/voice/download"
sum() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi; }
ai_sum="$(sum "$ping/plugins/ai/download")"
voice_sum="$(sum "$ping/plugins/voice/download")"
other_sum="$(printf 'ai bytes built at v1.2.10\n' | { if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi; } | awk '{print $1}')"

# registry FILE AI_CHECKSUM: licensed ai (max) and voice (pro), plus a free row the contract must ignore.
registry() {
  jq -n --arg ai "$2" --arg voice "sha256:$voice_sum" '{plugins: {
    ai: {version: "1.2.9", tier: "max", checksum: $ai},
    voice: {version: "1.2.9", tier: "pro", checksum: $voice},
    notify: {version: "1.0.0", tier: "free", checksum: "0000"}
  }}' > "$1"
}

# run_contract WANT_RC GREP NAME ARGS...: run the contract against the file:// ping with a fixture key.
run_contract() {
  want="$1" pattern="$2" name="$3"; shift 3
  out="$(PING_API_BASE="file://$ping" NSELF_PLUGIN_LICENSE_KEY_OWNER="${KEY-nself_pro_fixture}" bash "$contract" "$@" 2>&1)"; rc=$?
  if [ "$rc" -eq "$want" ] && printf '%s\n' "$out" | grep -q -- "$pattern"; then
    pass "TestWire16RowEqualsBytesContract: $name"
  else
    flunk "TestWire16RowEqualsBytesContract: $name (rc $rc, want $want, pattern '$pattern'):
$out"
  fi
}

registry "$tmp/match.json" "$ai_sum"
run_contract 0 "ref v1.2.9: 2 match, 0 fail" "rows equal the served bytes" --ref v1.2.9 --registry "$tmp/match.json"
run_contract 0 "ref $prod_ref: 2 match" "ref defaults to the production LICENSED_REF" --registry "$tmp/match.json"

registry "$tmp/drift.json" "$other_sum"
run_contract 1 "MISMATCH  ai: row at v1.2.9 says $other_sum, ping serves $ai_sum" \
  "probe: a row checksum from another ref fails" --ref v1.2.9 --registry "$tmp/drift.json"

registry "$tmp/nosum.json" ""
run_contract 1 "FAIL      ai: licensed row at v1.2.9 has no checksum" "a licensed row without a checksum fails" \
  --ref v1.2.9 --registry "$tmp/nosum.json"

rm "$ping/plugins/voice/download"
run_contract 1 "FAIL      voice: download failed" "bytes ping cannot serve fail" --ref v1.2.9 --registry "$tmp/match.json"
run_contract 0 "1 match, 0 fail" "--plugin checks one slug" --ref v1.2.9 --registry "$tmp/match.json" --plugin ai
run_contract 1 "no licensed rows named notify" "a free slug is no licensed row" --ref v1.2.9 --registry "$tmp/match.json" --plugin notify

KEY="" run_contract 3 "NSELF_PLUGIN_LICENSE_KEY_OWNER is unset" "no license key is a precondition, not a pass" \
  --ref v1.2.9 --registry "$tmp/match.json"

if [ "$fail" -ne 0 ]; then echo "test-licensed-registry-ref: FAILED" >&2; exit 1; fi
echo "test-licensed-registry-ref: OK"
