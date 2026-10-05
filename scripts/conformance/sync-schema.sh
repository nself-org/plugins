#!/usr/bin/env bash
# sync-schema.sh: PULL schemas/plugin-manifest.v2.schema.json from nself-org/cli at the commit pinned in
# scripts/cli-tools.version and record its sha256 in schemas/SOURCE. Never edit the copy by hand.
# Usage: sync-schema.sh          write the copy and SOURCE
#        sync-schema.sh --check  fail unless the copy, SOURCE and the pinned cli commit agree
set -eu
. "$(dirname "$0")/lib.sh"
rel=schemas/plugin-manifest.v2.schema.json
dst=$CONF_ROOT/$rel; src_file=$CONF_ROOT/schemas/SOURCE
sha=$(pin_value sha); repo=$(pin_value repo)
cli=$(cli_src); upstream=$cli/$rel
[ -f "$upstream" ] || { echo "sync-schema: $rel missing at cli $sha" >&2; exit 1; }
case ${1:-} in
  "") mkdir -p "$CONF_ROOT/schemas"; cp "$upstream" "$dst"
      printf '# Written by scripts/conformance/sync-schema.sh. Do not edit.\nrepo=%s\nsha=%s\npath=%s\nsha256=%s\n' "$repo" "$sha" "$rel" "$(sha256_of "$dst")" > "$src_file"
      echo "synced $rel from cli $sha";;
  --check)
    [ -f "$dst" ] && [ -f "$src_file" ] || { echo "sync-schema: schemas/ copy or SOURCE missing" >&2; exit 1; }
    [ "$(sed -n 's/^sha=//p' "$src_file")" = "$sha" ] || { echo "sync-schema: SOURCE sha differs from scripts/cli-tools.version" >&2; exit 1; }
    [ "$(sed -n 's/^sha256=//p' "$src_file")" = "$(sha256_of "$dst")" ] || { echo "sync-schema: schema copy was edited (sha256 differs from SOURCE)" >&2; exit 1; }
    cmp -s "$dst" "$upstream" || { echo "sync-schema: schema copy differs from cli $sha" >&2; exit 1; }
    echo "schema copy matches cli $sha";;
  *) echo "usage: sync-schema.sh [--check]" >&2; exit 2;;
esac
