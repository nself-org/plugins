#!/usr/bin/env bash
# Rule migration-immutable (D-0346): a boot migration file that a release already shipped is frozen. The boot ledger
# (np_<name>.schema_migrations) stores the sha256 of each applied file's bytes and a changed applied file is FATAL at boot;
# a deleted or renamed one leaves a ledger row with no file. Usage: migration-immutable.sh <plugin dir> <exceptions> <basedir>
# <basedir> holds the plugin trees of the last release tag; the rule applies only when that baseline plugin is manifest
# v2 with migrations.apply boot (earlier releases had no boot ledger). Fix: add a new migration file instead of editing.
set -u
. "$(dirname "$0")/../lib.sh"; . "$(dirname "$0")/migration-lib.sh"
dir=$1; exc=${2:-$CONF_DIR/exceptions.yaml}; base=${3:-}; name=$(plugin_name "$dir")
[ -n "$base" ] || { fail migration-immutable "$name" "no migration baseline (pass the release tree as the third argument)"; exit 1; }
bdir=$base/$(basename "$dir"); tag=${CONF_MIG_LABEL:-the baseline}; rc=0
released_boot "$bdir" || exit 0
exception_active "$exc" migration-immutable "$name" && exit 0
while IFS= read -r f; do
  [ -n "$f" ] || continue
  if [ ! -f "$dir/migrations/$f" ]; then
    fail migration-immutable "$name" "migrations/$f was released in $tag and is gone (deleted or renamed): an install that applied it keeps its ledger row, and the file can never be removed"; rc=1
  elif ! cmp -s "$bdir/migrations/$f" "$dir/migrations/$f"; then
    fail migration-immutable "$name" "migrations/$f changed since $tag (sha256 $(short_sha "$bdir/migrations/$f").. -> $(short_sha "$dir/migrations/$f").. ): the boot ledger makes this fatal; add a new migration instead"; rc=1
  fi
done <<EOT
$(applied_files "$bdir")
EOT
exit $rc
