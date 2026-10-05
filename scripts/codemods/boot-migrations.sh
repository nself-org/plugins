#!/usr/bin/env bash
# boot-migrations.sh [--dry-run] <plugin dir>: wire sdk/go/migrate into a Go plugin that has migrations/*.sql
# (contract plugin.boot-migrations v1): migrations.apply: boot in the manifest, a go:embed package in
# migrations/, one Apply call after the pgx pool is created, a /health "migrations" member, and the SDK
# package vendored under third_party/cli-sdk-go (the sdk/go/v2 tag does not exist yet; replace by a require
# when it does). A plugin with more than one go.mod (a nested module) is reported manual and nothing is written.
# All-or-nothing: a failure after the first write restores the plugin directory exactly. Prints "boot <state> [detail]": converted | noop | none | manual: <why>.
set -u
. "$(dirname "$0")/../conformance/lib.sh"
dry=0; [ "${1:-}" = --dry-run ] && { dry=1; shift; }
dir=$1; pj=$dir/plugin.json; name=$(plugin_name "$dir")
ls "$dir"/migrations/*.sql >/dev/null 2>&1 || { echo "boot none: no migrations/"; exit 0; }
[ "$(jq -r '.language // ""' "$pj")" = go ] || { echo "boot manual: non-Go plugin implements the contract in-plugin (migrations.apply: boot still required)"; exit 0; }
nmods=$(own_go_mods "$dir" | grep -c .)
[ "$nmods" -le 1 ] || { echo "boot manual: $nmods go.mod files (nested module); the migrations package and the Apply call need hand wiring"; exit 0; }
if [ "$(manifest_version "$dir")" != 2 ] && [ $dry -eq 0 ]; then echo "boot manual: convert the manifest first"; exit 0; fi
bm=$(local_tool bootmig "$CONF_ROOT/scripts/codemods/bootmig") || { echo "boot manual: cannot build bootmig"; exit 1; }
[ $dry -eq 1 ] || snap_take "$dir" || { echo "boot manual: cannot snapshot $dir"; exit 1; }
bail() { echo "boot manual: $1"; [ $dry -eq 1 ] || snap_restore; exit 1; }
schema="np_$(printf '%s' "$name" | tr '-' '_')"
flags=""; [ $dry -eq 1 ] && flags=-dry
res=$("$bm" $flags -dir "$dir" -schema "$schema") || bail "${res:-bootmig failed}"
case $res in manual*) echo "boot $res"; [ $dry -eq 1 ] || snap_restore; exit 0;; esac
changed=0; [ "$res" != noop ] && changed=1
if [ "$(jq -r '.migrations.apply // ""' "$pj")" != boot ] && [ "$(manifest_version "$dir")" = 2 ]; then
  changed=1
  if [ $dry -eq 0 ]; then
    tool=$(cli_tool manifestv2migrate) || bail "cannot build manifestv2migrate"; t=$(mktemp)
    { jq '.migrations = {dir: "migrations", apply: "boot"}' "$pj" > "$t" && "$tool" -in "$t" > "$t.2" && mv "$t.2" "$pj"; } || { rm -f "$t" "$t.2"; bail "cannot set migrations.apply"; }
    rm -f "$t" "$t.2"
  fi
fi
sdk=$(cli_src)/sdk/go; dest=$dir/third_party/cli-sdk-go
if [ ! -f "$dest/migrate/apply.go" ] && [ $dry -eq 0 ]; then
  changed=1; mkdir -p "$dest/migrate"
  for src in "$sdk"/migrate/*.go; do case $src in *_test.go) continue;; esac; cp "$src" "$dest/migrate/"; done
  cp "$(cli_src)/LICENSE" "$dest/LICENSE"   # the license gate walks every go.mod, so the copied module carries its license
  printf 'module github.com/nself-org/cli/sdk/go/v2\n\ngo 1.25.0\n\nrequire github.com/jackc/pgx/v5 %s\n' "$(awk '$1 == "github.com/jackc/pgx/v5" { print $2; exit }' "$sdk/go.mod")" > "$dest/go.mod"
  # tidy fills in the indirect requires and go.sum, so the copied module resolves on its own (license gate, builds)
  ( cd "$dest" && GOFLAGS=-mod=mod go mod tidy >/dev/null 2>&1 ) || bail "go mod tidy failed in the copied SDK module"
  ( cd "$dir" && go mod edit -require=github.com/nself-org/cli/sdk/go/v2@v2.0.0 -replace=github.com/nself-org/cli/sdk/go/v2=./third_party/cli-sdk-go )
fi
snap_drop
[ $changed -eq 0 ] && { echo "boot noop"; exit 0; }
echo "boot converted ($res)"
