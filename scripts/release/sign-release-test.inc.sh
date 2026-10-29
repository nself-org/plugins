#!/usr/bin/env bash
# Sourced by platform-signatures-test.sh with its throwaway fixtures and assertion helpers.
# --- the workflow's complete sign step ---------------------------------------------------------------------------------------
RUN="$T/release-run"
mkdir -p "$RUN/published" "$RUN/expected" "$RUN/free/demo" "$T/pinned"
cp "$GOOD"/*.tar.gz "$RUN/published/"
printf '{"version":"1.2.3"}\n' > "$RUN/free/demo/plugin.json"
cp "$T/test.pub" "$T/pinned/plugins-release-free-test.pub"
# Exercise a second signed slug so a shallow merge or a lost source signature cannot pass.
mkdir -p "$RUN/free/other"
cp "$RUN/free/demo/plugin.json" "$RUN/free/other/plugin.json"
for f in "$GOOD"/*.tar.gz; do
  cp "$f" "$RUN/published/other-${f##*/demo-}"
done
(cd "$RUN/published" && for f in *-linux-amd64.tar.gz *-darwin-arm64.tar.gz; do printf '%s  %s\n' "$(sha256_file "$f")" "$f"; done) > "$RUN/expected/built-sha256.txt"
run_sign_step() {
  NSELF_RELEASE_SIGNING_KEY_FREE="$(cat "$T/key.pem")" NSELF_RELEASE_SIGNING_KEY_FREE_ID="$ID" OPENSSL="$O" \
    bash "$HERE/sign-release.sh" v1.2.3 "$RUN" "${1:-$T/pinned}"
}
if out="$(run_sign_step 2>&1)"; then
  if jq -e --arg id "$ID" 'length == 2 and all(.[];
      .release_signature.key_id == $id and (.platform_signatures | length) == 2
      and all(.platform_signatures[]; .key_id == $id and (.sha256 | length) == 64))' "$RUN/signatures.json" >/dev/null; then
    pass "workflow sign step preserves both sources and all platform signatures"
  else fail "workflow sign step lost signatures during deep merge"; fi
else fail "workflow sign step rejected fixture: $out"; fi
if run_sign_step "$HERE/keys" > "$T/post-check.log" 2>&1; then
  fail "workflow sign step accepted a key absent from committed keys"
elif grep -q 'signing secret is not a pinned key' "$T/post-check.log"; then
  pass "workflow sign step checks signatures against pinned keys"
else fail "workflow sign step failed before pinned-key post-check"; fi
if [ ! -e "$RUN/signatures.json" ]; then pass "refused rerun clears earlier signatures artifact"; else fail "refused rerun left stale signatures artifact"; fi
: > "$RUN/expected/built-sha256.txt"
if run_sign_step > "$T/built-check.log" 2>&1; then
  fail "workflow sign step signed platform tarballs absent from built list"
elif grep -q 'not built by this run' "$T/built-check.log"; then
  pass "workflow sign step enforces platform built list"
else fail "workflow sign step failed before platform built check"; fi
if [ ! -e "$RUN/signatures.json" ]; then pass "refused sign step publishes no signatures artifact"; else fail "refused sign step left signatures artifact"; fi

