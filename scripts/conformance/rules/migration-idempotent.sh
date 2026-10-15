#!/usr/bin/env bash
# Rule migration-idempotent (D-0346): every boot migration that can still be applied for the first time must be safe to
# replay, because `plugin remove` drops the boot ledger but keeps the public tables and a reinstall applies every file
# again over them. Usage: migration-idempotent.sh <plugin dir> <exceptions> <basedir>
# Checked files: those not in the released baseline when the baseline plugin was already boot-applied (released files are
# frozen by migration-immutable), otherwise all of them. The lint is the Go tool in migrationlint/ (statement level rules:
# IF NOT EXISTS, OR REPLACE, guarded DO blocks, ON CONFLICT ...). A real non-idempotent statement needs an entry in
# scripts/conformance/exceptions.yaml (rule migration-idempotent) naming the file and statement.
set -u
. "$(dirname "$0")/../lib.sh"; . "$(dirname "$0")/migration-lib.sh"
dir=$1; exc=${2:-$CONF_DIR/exceptions.yaml}; base=${3:-}; name=$(plugin_name "$dir")
[ -n "$base" ] || { fail migration-idempotent "$name" "no migration baseline (pass the release tree as the third argument)"; exit 1; }
bdir=$base/$(basename "$dir"); files=
released_boot "$bdir"; rb=$?
[ $rb -ne 2 ] || { fail migration-idempotent "$name" "the baseline manifest $bdir/plugin.json cannot be read (malformed JSON): cannot tell which migration files are new"; exit 1; }
if [ $rb -eq 0 ]; then
  old=$(applied_files "$bdir")
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    printf '%s\n' "$old" | grep -qxF "$f" || files="$files migrations/$f"
  done <<EOT
$(applied_files "$dir")
EOT
else
  for f in $(applied_files "$dir"); do files="$files migrations/$f"; done
fi
[ -n "$files" ] || exit 0
exception_active "$exc" migration-idempotent "$name" && exit 0
bin=$(local_tool migrationlint "$CONF_DIR/rules/migrationlint") || { fail migration-idempotent "$name" "cannot build migrationlint"; exit 1; }
out=$(cd "$dir" && "$bin" idempotent $files 2>&1); r=$?
[ $r -eq 0 ] && exit 0
printf '%s\n' "$out" | grep . | while IFS= read -r l; do fail migration-idempotent "$name" "$l"; done
exit 1
