#!/usr/bin/env bash
# Rule legacy-renames (D-0346): the table renames a plugin's boot migrations perform must be the ones its legacy fixtures
# prove. Usage: legacy-renames.sh <plugin dir> <exceptions>
# The renames come from the migrationlint tool (literal ALTER TABLE ... RENAME TO, or a DO block that renames through a
# (VALUES ('old', 'new'), ...) list); a rename it cannot read fails closed. tests/boot/legacy-renames (`old new` lines)
# must hold exactly that set, and so must the `var legacyTables = ...{ {"old", "new"}, ... }` block of non-test Go source
# when there is one. tests/boot/legacy-probe.sql is run by tables-upgrade.sh after the legacy boot; a probe with no
# legacy-renames never runs, which fails. A plugin without renames and without a fixture passes.
set -u
. "$(dirname "$0")/../lib.sh"; . "$(dirname "$0")/migration-lib.sh"
dir=$1; exc=${2:-$CONF_DIR/exceptions.yaml}; name=$(plugin_name "$dir"); rc=0
fx=$dir/tests/boot/legacy-renames; probe=$dir/tests/boot/legacy-probe.sql
exception_active "$exc" legacy-renames "$name" && exit 0
# lines <text>: sorted unique non-blank lines.
lines() { printf '%s\n' "$1" | grep . | LC_ALL=C sort -u; }
# cmp_sets <label a> <set a> <label b> <set b>: FAIL lines for pairs only in one of the sets; sets rc=1.
cmp_sets() {
  local only
  only=$(LC_ALL=C comm -23 <(lines "$2") <(lines "$4")); [ -z "$only" ] || { fail legacy-renames "$name" "in $1 but not in $3: $(printf '%s' "$only" | tr '\n' ',' | sed 's/,$//; s/,/, /g')"; rc=1; }
  only=$(LC_ALL=C comm -13 <(lines "$2") <(lines "$4")); [ -z "$only" ] || { fail legacy-renames "$name" "in $3 but not in $1: $(printf '%s' "$only" | tr '\n' ',' | sed 's/,$//; s/,/, /g')"; rc=1; }
}
sqlf=; for f in $(applied_files "$dir"); do sqlf="$sqlf migrations/$f"; done
sqlp=; err=$(conf_work)/legacy-renames-err.$$; trap 'rm -f "$err"' EXIT
if [ -n "$sqlf" ]; then
  bin=$(local_tool migrationlint "$CONF_DIR/rules/migrationlint") || { fail legacy-renames "$name" "cannot build migrationlint"; exit 1; }
  if ! sqlp=$(cd "$dir" && "$bin" renames $sqlf 2>"$err"); then
    grep . "$err" | while IFS= read -r l; do fail legacy-renames "$name" "$l (fail closed: spell the rename as a literal ALTER TABLE or a (VALUES ('old', 'new')) list)"; done
    exit 1
  fi
fi
if [ -z "$sqlp" ] && [ ! -f "$fx" ]; then
  [ -f "$probe" ] && { fail legacy-renames "$name" "tests/boot/legacy-probe.sql exists but there is no tests/boot/legacy-renames: the probe never runs"; exit 1; }
  exit 0
fi
[ -f "$fx" ] || { fail legacy-renames "$name" "migrations rename tables ($(printf '%s' "$sqlp" | tr '\n' ',' | sed 's/,$//; s/,/, /g')) but tests/boot/legacy-renames does not exist"; exit 1; }
fxp=
while IFS= read -r line || [ -n "$line" ]; do
  case $line in ''|'#'*) continue;; esac
  set -f; set -- $line; set +f   # no globbing: a field like * must stay literal
  if [ $# -ne 2 ]; then fail legacy-renames "$name" "tests/boot/legacy-renames line '$line' is not 'old new'"; rc=1; continue; fi
  fxp="$fxp$1 $2
"
done < "$fx"
cmp_sets "the migrations" "$sqlp" "tests/boot/legacy-renames" "$fxp"
# Go source: the legacyTables block, when the plugin declares one outside tests and vendored trees.
gosrc=$(find "$dir" \( -name vendor -o -name node_modules -o -name third_party \) -prune -o -name '*.go' ! -name '*_test.go' -type f -exec grep -l 'var legacyTables' {} +)
for g in $gosrc; do
  gp=$(awk '/^var legacyTables/ { on = 1; next } on && /^}/ { on = 0 }
    on && match($0, /\{"[^"]*",[[:space:]]*"[^"]*"/) { s = substr($0, RSTART + 1, RLENGTH - 1); gsub(/[",]/, "", s); gsub(/[[:space:]]+/, " ", s); print s }' "$g")
  cmp_sets "the migrations" "$sqlp" "${g#$dir/} legacyTables" "$gp"
done
exit $rc
