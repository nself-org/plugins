#!/usr/bin/env bash
# Rule boot-schema (D-0453): a manifest with migrations.apply "boot" declares `schema` equal to
# np_<name with - replaced by _>. The pinned v2 schema (schemas/plugin-manifest.v2.schema.json) describes
# `schema` that way, and the SDK migration ledger lives in that schema. The Go boot code must agree: any
# `Schema: "np_..."` literal (migrate.Options) in non-test Go source (not vendor/, third_party/,
# node_modules/) that differs from the manifest schema fails.
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir"); rc=0
[ "$(jq -r '.migrations.apply // empty' "$dir/plugin.json")" = boot ] || exit 0
want=np_$(printf '%s' "$name" | tr '-' '_')
got=$(jq -r '.schema // empty' "$dir/plugin.json")
if [ "$got" != "$want" ]; then
  fail boot-schema "$name" "migrations.apply boot needs schema $want, got ${got:-none}"; rc=1
fi
lits=$(find "$dir" \( -name vendor -o -name node_modules -o -name third_party \) -prune -o \
  -name '*.go' ! -name '*_test.go' -type f -print | sort | while IFS= read -r f; do
    [ -n "$f" ] || continue
    grep -nE 'Schema:[[:space:]]*"np_[a-z0-9_]+"' "$f" 2>/dev/null | grep -vE '^[0-9]+:[[:space:]]*//' \
      | sed -E 's/^.*Schema:[[:space:]]*"(np_[a-z0-9_]+)".*$/\1/'
  done | sort -u)
for lit in $lits; do
  if [ "$lit" != "${got:-none}" ]; then
    fail boot-schema "$name" "Go boot code applies migrations to $lit but plugin.json schema is ${got:-none}"; rc=1
  fi
done
exit $rc
