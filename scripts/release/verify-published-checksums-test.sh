#!/usr/bin/env bash
# verify-published-checksums-test.sh: offline gate for scripts/verify-published-checksums.sh --write --signatures, the mode the
# release-data job runs. A stub `gh` serves a fake release from a temp directory; nothing is downloaded, signed or uploaded and no
# key is read. It covers:
#   - a good release is written with its platform checksums and the nself-* binaries of the linux-amd64 tarball;
#   - --signatures without --published-sha256 is refused (D-1105);
#   - a source or platform tarball whose bytes differ from the publish job's manifest, or is not in it, stops the write (D-1105);
#   - an unreadable linux-amd64 archive stops the write instead of clearing the binaries list (D-1168);
#   - a releases.json write that fails is an error, not "Wrote ..." with exit 0 (D-1167).
# Run from any directory: bash scripts/release/verify-published-checksums-test.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
command -v jq >/dev/null 2>&1 || { echo "FAIL: jq not found"; exit 1; }
T="$(mktemp -d "${TMPDIR:-/tmp}/vpc-test.XXXXXX")"
trap 'rm -rf "$T"' EXIT

sha256_file() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }

# A copy of the scripts the CLI sources, so REPO_ROOT (releases.json, free/) is the fixture and not this checkout.
TREE="$T/tree"
mkdir -p "$TREE/scripts/conformance" "$TREE/free/demo" "$T/bin"
cp "$REPO/scripts/verify-published-checksums.sh" "$REPO/scripts/verify-published-checksums-lib.sh" "$TREE/scripts/"
cp "$REPO/scripts/conformance/lib.sh" "$TREE/scripts/conformance/"
[ ! -f "$REPO/scripts/conformance/exceptions.yaml" ] || cp "$REPO/scripts/conformance/exceptions.yaml" "$TREE/scripts/conformance/"
printf '{"name":"demo","version":"1.2.3","binaryName":"nself-demo"}\n' > "$TREE/free/demo/plugin.json"

cat > "$T/bin/gh" <<'STUB'
#!/usr/bin/env bash
# Stub gh: `release view` succeeds, lists and "downloads" the files in $STUB_ASSETS.
case "$1 $2" in
  "release view")
    case " $* " in *" --json "*) ls "$STUB_ASSETS" ;; esac ;;
  "release download")
    while [ $# -gt 0 ]; do [ "$1" = "--dir" ] && dir="$2"; shift; done
    cp "$STUB_ASSETS"/* "$dir"/ ;;
  *) exit 1 ;;
esac
STUB
chmod +x "$T/bin/gh"

PLATFORMS="darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64"
ASSETS="$T/assets"
mkdir -p "$ASSETS" "$T/stub"
printf 'stub\n' > "$T/stub/nself-demo"
tar -czf "$ASSETS/demo-1.2.3.tar.gz" -C "$T/stub" nself-demo
for p in $PLATFORMS; do printf '%s\n' "$p" > "$T/stub/$p.txt"; tar -czf "$ASSETS/demo-1.2.3-$p.tar.gz" -C "$T/stub" nself-demo "$p.txt"; done

# manifest_for DIR: the publish job's published-sha256.txt for DIR.
manifest_for() { (cd "$1" && for f in *.tar.gz; do printf '%s  %s\n' "$(sha256_file "$f")" "$f"; done); }
# signatures_for DIR: the sign job's output for DIR's bytes.
signatures_for() {
  local d="$1" psigs="{}" p
  for p in $PLATFORMS; do
    psigs="$(jq -c --arg p "$p" --arg s "$(sha256_file "$d/demo-1.2.3-$p.tar.gz")" '.[$p] = {sha256: $s, key_id: "k", alg: "ed25519", sig: "x"}' <<<"$psigs")"
  done
  jq -nc --arg s "$(sha256_file "$d/demo-1.2.3.tar.gz")" --argjson ps "$psigs" \
    '{demo: {sha256: $s, release_signature: {key_id: "k", alg: "ed25519", sig: "x"}, platform_signatures: $ps}}'
}

fails=0
pass() { echo "ok   $1"; }
fail() { echo "FAIL $1"; fails=$((fails + 1)); }
EMPTY='{"schema_version":1,"releases":{}}'

# run_case: sets OUT and RC. Args: assets dir, manifest file ("" for none), then extra args.
run_case() {
  local assets="$1" manifest="$2"; shift 2
  printf '%s\n' "$EMPTY" > "$TREE/releases.json"
  signatures_for "$assets" > "$T/signatures.json"
  local args=(--write --signatures "$T/signatures.json")
  [ -z "$manifest" ] || args+=(--published-sha256 "$manifest")
  RC=0
  OUT="$(STUB_ASSETS="$assets" PATH="$T/bin:$PATH" bash "$TREE/scripts/verify-published-checksums.sh" "${args[@]}" "$@" v1.2.3 2>&1)" || RC=$?
}
unchanged() { [ "$(jq -c . "$TREE/releases.json")" = "$EMPTY" ]; }
# expect_refused LABEL NEEDLE: the last run exited non-zero, said NEEDLE and left releases.json alone.
expect_refused() {
  if [ "$RC" -eq 0 ]; then fail "$1 (exit 0)"; return; fi
  case "$OUT" in *"$2"*) ;; *) fail "$1 (no '$2' in: $OUT)"; return ;; esac
  unchanged && pass "$1" || fail "$1 (releases.json was modified)"
}

manifest_for "$ASSETS" > "$T/manifest.txt"

# --- good release ---------------------------------------------------------------------------------------------------------------
run_case "$ASSETS" "$T/manifest.txt"
if [ "$RC" -eq 0 ] && [ "$(jq -c '.releases.demo.binaries' "$TREE/releases.json")" = '["nself-demo"]' ] \
  && [ "$(jq -r '.releases.demo.platform_checksums["linux-amd64"]' "$TREE/releases.json")" = "$(sha256_file "$ASSETS/demo-1.2.3-linux-amd64.tar.gz")" ]; then
  pass "good release is written with platform checksums and binaries"
else
  fail "good release (rc=$RC): $OUT"
fi

# --- D-1105: the publish manifest is enforced ---------------------------------------------------------------------------------------
run_case "$ASSETS" ""
expect_refused "--signatures without --published-sha256 is refused" "--signatures needs --published-sha256"

# The Release served other bytes for one platform tarball after approval; the signer's file describes the new bytes, so only the
# manifest can catch it.
SWAP="$T/swap"; rm -rf "$SWAP"; cp -R "$ASSETS" "$SWAP"
printf 'swapped\n' > "$T/stub/evil.txt"
tar -czf "$SWAP/demo-1.2.3-darwin-arm64.tar.gz" -C "$T/stub" nself-demo evil.txt
run_case "$SWAP" "$T/manifest.txt"
expect_refused "platform tarball differing from the publish manifest" "UNAPPROVED demo/darwin-arm64"

printf 'swapped source\n' > "$T/stub/src.txt"
tar -czf "$SWAP/demo-1.2.3.tar.gz" -C "$T/stub" src.txt
run_case "$SWAP" "$T/manifest.txt"
expect_refused "source tarball differing from the publish manifest" "UNAPPROVED demo: demo-1.2.3.tar.gz"

grep -v 'linux-arm64' "$T/manifest.txt" > "$T/short-manifest.txt"
run_case "$ASSETS" "$T/short-manifest.txt"
expect_refused "platform tarball missing from the publish manifest" "UNAPPROVED demo/linux-arm64"

# --- D-1168: an unreadable linux-amd64 archive ----------------------------------------------------------------------------------------
BAD="$T/bad"; rm -rf "$BAD"; cp -R "$ASSETS" "$BAD"
printf 'not a gzip stream\n' > "$BAD/demo-1.2.3-linux-amd64.tar.gz"
manifest_for "$BAD" > "$T/bad-manifest.txt"
run_case "$BAD" "$T/bad-manifest.txt"
expect_refused "unreadable linux-amd64 archive stops the write" "UNREADABLE demo/linux-amd64"

# --- D-1167: a failed releases.json write ---------------------------------------------------------------------------------------------
printf '%s\n' "$EMPTY" > "$TREE/releases.json"
signatures_for "$ASSETS" > "$T/signatures.json"
# The directory cannot be written, as on a read-only disk. A pre-existing releases.json.tmp directory (with content) must survive the failure.
mkdir -p "$TREE/releases.json.tmp"
printf 'keep\n' > "$TREE/releases.json.tmp/sentinel"
chmod a-w "$TREE"
if [ -w "$TREE" ]; then
  echo "skip failed-write case: directory stays writable (running as root?)"
else
  RC=0
  OUT="$(STUB_ASSETS="$ASSETS" PATH="$T/bin:$PATH" bash "$TREE/scripts/verify-published-checksums.sh" --write --signatures "$T/signatures.json" --published-sha256 "$T/manifest.txt" v1.2.3 2>&1)" || RC=$?
  if [ "$RC" -ne 0 ] && [[ "$OUT" != *"Wrote "* ]] && [[ "$OUT" == *"could not write"* ]] && unchanged; then
    pass "failed releases.json write is an error, not Wrote"
  else
    fail "failed write (rc=$RC): $OUT"
  fi
  if [ -f "$TREE/releases.json.tmp/sentinel" ]; then
    pass "failed write leaves a pre-existing releases.json.tmp directory intact"
  else
    fail "failed write deleted a pre-existing releases.json.tmp directory"
  fi
fi
chmod u+w "$TREE"
rm -rf "$TREE/releases.json.tmp"

if [ "$fails" -ne 0 ]; then echo "$fails case(s) failed"; exit 1; fi
echo "all verify-published-checksums cases passed"
