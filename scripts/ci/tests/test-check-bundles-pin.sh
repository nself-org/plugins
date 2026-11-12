#!/usr/bin/env bash
# test-check-bundles-pin.sh: TestWire17PinGateFailsOnDrift and friends, for scripts/ci/check-bundles-pin.sh (P7-WIRE-17).
#
# Purpose: drive the real gate script by argv against fixture git repos: a bundles clone whose main moved past the pin
#   (a chat membership change) must fail; the bumped pin with regenerated projections must pass.
# Fixture: <tmp>/plugins (bundles.ref, catalog.json, counts.json, the script) and <tmp>/bundles (a git repo with main).
# Outputs: "ok:"/"FAIL:" per case, final summary; exit 1 on any failure.
# Constraints: bash 3.2 compatible; no network; jq and git only.
set -u

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
gate="$repo/scripts/ci/check-bundles-pin.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/check-bundles-pin-test.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT
fail=0
g() { git -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false -c init.defaultBranch=main "$@"; }

bundles="$tmp/bundles"
mkdir -p "$bundles"
g -C "$bundles" init -q . || { echo "FAIL: cannot init the bundles fixture" >&2; exit 1; }
write_bundles() { # chat plugins as a JSON array
  printf '{"schema_version":"2.0.0","bundles":{"chat":{"display":"Chat Bundle","tier":"paid","plugins":%s},"task":{"display":"Task Bundle","tier":"free","plugins":["notify","jobs"]}}}\n' "$1" > "$bundles/bundles.json"
  echo '{"plugins":{}}' > "$bundles/registry.json"
  g -C "$bundles" add bundles.json registry.json && g -C "$bundles" commit -q -m "$2"
}
write_bundles '["bots","livekit","nself-sms","support"]' old || { echo "FAIL: fixture commit" >&2; exit 1; }
old="$(g -C "$bundles" rev-parse HEAD)"
write_bundles '["auth","bots","chat","livekit","moderation","nself-sms","realtime","support"]' new || { echo "FAIL: fixture commit" >&2; exit 1; }
new="$(g -C "$bundles" rev-parse HEAD)"
g -C "$bundles" commit -q --allow-empty -m "unrelated docs change" || exit 1
head_sha="$(g -C "$bundles" rev-parse HEAD)"

plugins="$tmp/plugins"
mkdir -p "$plugins/scripts"
# make_plugins PIN CHAT_JSON: the pin plus projections as scripts/plugin-counts.sh would write them from PIN.
make_plugins() {
  printf '# pin\n%s\n' "$1" > "$plugins/scripts/bundles.ref"
  printf '{"bundles":{"chat":{"display":"Chat Bundle","plugins":%s,"tier":"licensed"},"task":{"display":"Task Bundle","plugins":["jobs","notify"],"tier":"free"}},"generated_from":{"bundles_ref":"%s"}}\n' "$2" "$1" > "$plugins/catalog.json"
  printf '{"advertised":1,"generated_from":{"bundles_ref":"%s"}}\n' "$1" > "$plugins/counts.json"
}
OLD_CHAT='["bots","livekit","nself-sms","support"]'
NEW_CHAT='["auth","bots","chat","livekit","moderation","nself-sms","realtime","support"]'

# run_case NAME WANT_RC GREP [extra args...]
run_case() {
  name="$1" want="$2" pat="$3"; shift 3
  out="$(env -u BUNDLES_DIR bash "$gate" --root "$plugins" --bundles-dir "$bundles" "$@" 2>&1)"; rc=$?
  if [ "$rc" -eq "$want" ] && printf '%s\n' "$out" | grep -q -- "$pat"; then echo "ok: $name"
  else echo "FAIL: $name (rc $rc, want $want, grep '$pat'): $out" >&2; fail=1; fi
}

# TestWire17PinGateFailsOnDrift: behind main fails, bumped pin passes.
make_plugins "$old" "$OLD_CHAT"
run_case TestWire17PinGateFailsOnDrift/behind-main-fails 1 "is behind bundles main"
run_case TestWire17PinGateFailsOnDrift/behind-main-names-membership 1 "catalog.json bundle membership differs"
make_plugins "$new" "$NEW_CHAT"
run_case TestWire17PinGateFailsOnDrift/bumped-pin-passes 0 "check-bundles-pin: OK"
make_plugins "$head_sha" "$NEW_CHAT"
run_case TestWire17PinGateFailsOnDrift/pin-at-main-head-passes 0 "check-bundles-pin: OK"

# Pin bumped but projections not regenerated.
make_plugins "$new" "$OLD_CHAT"
printf '{"advertised":1,"generated_from":{"bundles_ref":"%s"}}\n' "$old" > "$plugins/counts.json"
run_case TestWire17StaleProjectionsFail/membership 1 "catalog.json bundle membership differs"
run_case TestWire17StaleProjectionsFail/counts-ref 1 "counts.json was generated from bundles_ref '$old'"

# Web pin and entitlement rows.
make_plugins "$new" "$NEW_CHAT"
printf '%s\n' "$old" > "$tmp/web.ref"
run_case TestWire17WebPinBehindFails 1 "web pin $old is behind" --web-pin "$tmp/web.ref"
printf '%s\n' "$head_sha" > "$tmp/web.ref"
run_case TestWire17WebPinAtMainPasses 0 "check-bundles-pin: OK" --web-pin "$tmp/web.ref"
printf '{"chat":%s,"task":["notify","jobs"],"media":["tmdb"]}\n' "$OLD_CHAT" > "$tmp/ent.json"
run_case TestWire17EntitlementRowsDriftFail 1 "bundle 'chat' differs" --entitlements "$tmp/ent.json"
printf '{"chat":%s,"media":["tmdb"]}\n' "$NEW_CHAT" > "$tmp/ent.json"
run_case TestWire17EntitlementRowMissingFails 1 "bundle 'task' differs" --entitlements "$tmp/ent.json"
printf '{"chat":%s,"task":["jobs","notify"],"media":["tmdb"]}\n' "$NEW_CHAT" > "$tmp/ent.json"
run_case TestWire17EntitlementRowsMatchPass 0 "check-bundles-pin: OK" --entitlements "$tmp/ent.json"

# Fail closed on missing preconditions.
out="$(env -u BUNDLES_DIR bash "$gate" --root "$plugins" 2>&1)"; rc=$?
if [ "$rc" -eq 3 ] && printf '%s\n' "$out" | grep -q "no bundles clone"; then echo "ok: TestWire17NoCloneFailsClosed"
else echo "FAIL: TestWire17NoCloneFailsClosed (rc $rc): $out" >&2; fail=1; fi
make_plugins "0000000000000000000000000000000000000000" "$NEW_CHAT"
run_case TestWire17UnknownPinFailsClosed 3 "does not hold"
make_plugins "$new" "$NEW_CHAT"
run_case TestWire17UnknownMainRefFailsClosed 3 "does not resolve" --main-ref nope
g -C "$bundles" checkout -q -b side "$old" && g -C "$bundles" commit -q --allow-empty -m side || exit 1
make_plugins "$(g -C "$bundles" rev-parse HEAD)" "$OLD_CHAT"
run_case TestWire17PinOffMainFails 1 "is not an ancestor"

# The committed repo: the pin carries a full sha and the projections name it.
pin="$(grep -v '^#' "$repo/scripts/bundles.ref" | head -1 | tr -d '[:space:]')"
for f in catalog.json counts.json; do
  if [ "$(jq -r '.generated_from.bundles_ref' "$repo/$f")" = "$pin" ]; then echo "ok: TestWire17Committed/$f-names-pin"
  else echo "FAIL: TestWire17Committed/$f-names-pin" >&2; fail=1; fi
done

if [ "$fail" -ne 0 ]; then echo "test-check-bundles-pin: FAILED" >&2; exit 1; fi
echo "test-check-bundles-pin: OK"
