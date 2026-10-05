#!/usr/bin/env bash
# v14-oracle.sh: the released-CLI oracle (D1). Downloads the nself v1.4.12 linux release asset from
# github.com/nself-org/cli (nothing else), verifies its sha256 against the release checksums.txt, then in a
# container with NO network installs each plugin twice from a local registry (once as its v1 original, once
# converted) and requires identical generated compose, .nself/compose.env PLUGIN_*_INTERNAL_URL lines,
# np_plugins seed SQL, `plugin list --installed --detailed` (tier) and plugin remove guard output.
# Intentional differences live in tests/conformance/oracle-deltas.yaml (slug, key, from, to, owner_ref, expires).
# Usage: v14-oracle.sh [--plugins a,b | --fixtures]   (--plugins: originals come from git origin/main free/<n>)
set -eu
. "$(dirname "$0")/lib.sh"
mode=fixtures; list=
while [ $# -gt 0 ]; do case $1 in --plugins) mode=plugins; list=$2; shift;; --fixtures) mode=fixtures;; *) echo "unknown argument $1" >&2; exit 2;; esac; shift; done
command -v docker >/dev/null && docker info >/dev/null 2>&1 || { echo "FAIL oracle: docker is not available (a skipped oracle is a failure)"; exit 1; }
W=$(conf_work)/oracle; mkdir -p "$W/dl"; VER=1.4.12
case "$(docker info --format '{{.Architecture}}')" in aarch64|arm64) arch=arm64;; *) arch=amd64;; esac
asset=nself-$VER-linux-$arch.tar.gz; base=https://github.com/nself-org/cli/releases/download/v$VER
[ -s "$W/dl/checksums.txt" ] || curl -fsSL -o "$W/dl/checksums.txt" "$base/checksums.txt"
want=$(awk -v a="$asset" '$2 == a { print $1 }' "$W/dl/checksums.txt"); [ -n "$want" ] || { echo "FAIL oracle: $asset not in checksums.txt"; exit 1; }
if [ ! -f "$W/dl/$asset" ] || [ "$(sha256_of "$W/dl/$asset")" != "$want" ]; then curl -fsSL -o "$W/dl/$asset" "$base/$asset"; fi
[ "$(sha256_of "$W/dl/$asset")" = "$want" ] || { echo "FAIL oracle: checksum mismatch for $asset"; exit 1; }
echo "oracle: $asset sha256 verified ($want)"
rm -rf "$W/bin"; mkdir -p "$W/bin"; tar -xzf "$W/dl/$asset" -C "$W/bin"; bin=$(find "$W/bin" -type f -name nself | head -1); mv "$bin" "$W/bin/nself"; chmod +x "$W/bin/nself"
cp "$CONF_DIR/oracle/regsrv.go" "$CONF_DIR/oracle/inner.sh" "$W/"
# roots: originals and converted trees
if [ "$mode" = fixtures ]; then
  orig=$CONF_ROOT/tests/conformance/fixtures/v1; conv=$W/conv; bash "$CONF_ROOT/scripts/codemods/run.sh" --fixtures --out "$conv" >"$W/codemods.log" 2>&1 || { cat "$W/codemods.log"; exit 1; }
  names="beta gamma alpha"; remove=beta
else
  orig=$W/orig; conv=$CONF_ROOT/free; rm -rf "$orig"; mkdir -p "$orig"; names=$(printf '%s' "$list" | tr ',' ' '); remove=
  ( cd "$CONF_ROOT" && git archive origin/main $(for n in $names; do printf 'free/%s ' "$n"; done) ) | tar -x -C "$orig"; orig=$orig/free
  remove=$(jq -r 'if (.dependencies | type) == "array" then .dependencies[0] // empty else empty end' "$conv/${names%% *}/plugin.json")
fi
mkreg() { # variant root
  local v=$1 r=$2 n d sha entries=""; rm -rf "$W/reg-$v"; mkdir -p "$W/reg-$v/plugins"
  for n in $names; do
    mkdir -p "$W/reg-$v/plugins/$n"; bash "$CONF_ROOT/scripts/deterministic-tar.sh" "$W/reg-$v/plugins/$n/tarball" -C "$r" "$n" >/dev/null; sha=$(sha256_of "$W/reg-$v/plugins/$n/tarball")
    entries="$entries$(jq -c --arg sha "$sha" '{(.name): {name, version, description, category, tier: "free", license: "MIT", requires_license: false, language: (.language // "go"), dependencies: (if (.dependencies | type) == "array" then .dependencies else [] end), tables: [], author: (.author // "nself"), checksum: $sha, min_nself_version: "1.0.0"}}' "$r/$n/plugin.json")
"
  done
  printf '%s\n' "$entries" | jq -s '{schema_version: "1", plugins: (map(select(. != null)) | add)}' > "$W/reg-$v/registry.json"
}
mkreg orig "$orig"; mkreg conv "$conv"
rc=0
rm -rf "$W"/out-orig "$W"/out-conv "$W"/out-conv-raw
for v in orig conv; do
  echo "oracle: installing with v$VER ($v)"
  if ! out=$(docker run --rm --network none --user "$(id -u):$(id -g)" -e REMOVE="$remove" -v "$W":/w golang:1.26.6 bash /w/inner.sh "$v" $names 2>&1); then
    echo "FAIL oracle: v$VER could not install the $v variant (escalate: D1 key design):"; printf '%s\n' "$out" | tail -12; exit 1
  fi
done
rm -rf "$W/out-conv-raw"; cp -R "$W/out-conv" "$W/out-conv-raw"
. "$CONF_DIR/oracle/deltas.sh"
norm() { sed -E 's#[0-9]{4}[-/][0-9]{2}[-/][0-9]{2}[T ][0-9:.]+Z?#TS#g' "$1"; }
apply_deltas "$W/out-conv" "$names" "$CONF_ROOT/tests/conformance/oracle-deltas.yaml" || rc=1
for f in docker-compose.yml compose-files.txt compose-env-urls.txt seed.sql list.txt remove-guard.txt; do
  [ -f "$W/out-orig/$f" ] || continue
  if diff <(norm "$W/out-orig/$f") <(norm "$W/out-conv/$f") >"$W/diff.$f" 2>&1; then echo "pass  oracle $f identical"; else echo "FAIL  oracle $f differs between v1 original and converted:"; head -20 "$W/diff.$f"; rc=1; fi
done
if [ "$mode" = fixtures ]; then
  grep -q '^PLUGIN_BETA_INTERNAL_URL=' "$W/out-conv-raw/compose-env-urls.txt" && grep -q '^PLUGIN_GAMMA_INTERNAL_URL=' "$W/out-conv-raw/compose-env-urls.txt" || { echo "FAIL  oracle: dependency URLs missing from compose.env (the fixture no longer exercises dependencies)"; rc=1; }
  grep -q "('gamma', 'pro')" "$W/out-conv/seed.sql" || { echo "FAIL  oracle: licensed-shaped fixture is not seeded as pro"; rc=1; }
fi
if [ -n "$remove" ]; then grep -q '^rc=0$' "$W/out-conv/remove-guard.txt" && { echo "FAIL  oracle: remove guard let a depended-on plugin go"; rc=1; } || echo "pass  oracle remove guard refuses removing $remove"; fi
[ $rc -eq 0 ] && echo "oracle: ok" || echo "oracle: FAIL"; exit $rc
