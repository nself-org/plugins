#!/usr/bin/env bash
# run.sh: run the three codemods (manifest-v2.sh, boot-migrations.sh, vendor.sh) over plugins.
# Usage: run.sh [--dry-run] (--all | --plugins a,b | --fixtures [--twice] [--out DIR] [--update-golden]) [--report FILE]
#   --all / --plugins   operate on free/<name> in place (--dry-run writes nothing and sizes the batches)
#   --fixtures          convert a copy of tests/conformance/fixtures/v1, compare with tests/conformance/expected/,
#                       and with --twice prove a second run changes nothing
# Output: one "plugin <name> manifest=<s> boot=<s> vendor=<s>" line per plugin (detail lines indented).
# Exit 1 when a plugin is blocked (a v1 key neither mapped nor on a drop list) or a fixture check fails.
set -u
. "$(dirname "$0")/../conformance/lib.sh"
CM=$CONF_ROOT/scripts/codemods
EXTRA=; dry=0; mode=; list=; twice=0; out=; update=0; report=
while [ $# -gt 0 ]; do case $1 in
  --dry-run) dry=1;; --all) mode=all;; --plugins) mode=plugins; list=$2; shift;; --fixtures) mode=fixtures;;
  --twice) twice=1;; --drop-dev-tool-keys) EXTRA=--drop-dev-tool-keys;; --out) out=$2; shift;; --update-golden) update=1;; --report) report=$2; shift;;
  *) echo "run.sh: unknown argument $1" >&2; exit 2;; esac; shift; done
[ -n "$mode" ] || { echo "run.sh: one of --all, --plugins, --fixtures is required" >&2; exit 2; }
df=; [ $dry -eq 1 ] && df=--dry-run

state_of() { printf '%s' "$1" | awk '{ s = $2; sub(/:$/, "", s); print s }'; }
# convert_one <dir>: run the steps as ONE all-or-nothing unit (a snapshot restores the plugin on any failure),
# print the plugin line; returns 1 when blocked. Boot and vendor run only after the manifest step converted or
# was already a no-op. Sets STATES. $EXTRA is passed to manifest-v2.sh (--drop-dev-tool-keys).
convert_one() {
  local d=$1 n m b v rc tidy=
  n=$(basename "$d")
  # Pre-flight: a plugin whose vendor step is manual or blocked is not touched at all (all-or-nothing).
  v=$(bash "$CM/vendor.sh" --dry-run "$d" 2>&1 | tail -1)
  case $v in
    "vendor manual"*|"vendor blocked"*)
      b="boot skipped: vendor step cannot run, plugin left untouched"; m="manifest skipped: vendor step cannot run, plugin left untouched"
      STATES="$(state_of "$m") $(state_of "$b") $(state_of "$v")"
      printf 'plugin %s manifest=%s boot=%s vendor=%s\n' "$n" $STATES; printf '  %s\n  %s\n  %s\n' "$m" "$b" "$v"
      case $v in "vendor blocked"*) return 1;; esac; return 0;;
  esac
  [ $dry -eq 1 ] || snap_take "$d" || { echo "plugin $n manifest=blocked boot=skipped vendor=skipped"; return 1; }
  m=$(bash "$CM/manifest-v2.sh" $df $EXTRA "$d" 2>&1 | tail -1)
  case $m in
    "manifest converted"*|"manifest noop"*)
      b=$(bash "$CM/boot-migrations.sh" $df "$d" 2>&1); rc=$?; b=$(printf '%s\n' "$b" | tail -1)
      if [ $rc -ne 0 ]; then v="vendor skipped: boot step failed"; else
        case $b in *converted*) tidy=--tidy;; esac
        v=$(bash "$CM/vendor.sh" $df $tidy "$d" 2>&1); rc=$?; v=$(printf '%s\n' "$v" | tail -1)
        [ $rc -ne 0 ] && b="boot skipped: vendor step failed"
      fi
      if [ $rc -ne 0 ]; then m="manifest skipped: rolled back, a later step failed"; [ $dry -eq 1 ] || snap_restore; else [ $dry -eq 1 ] || snap_drop; fi;;
    *) b="boot skipped: manifest step did not convert"; v="vendor skipped: manifest step did not convert"
       [ $dry -eq 1 ] || snap_restore;;
  esac
  STATES="$(state_of "$m") $(state_of "$b") $(state_of "$v")"
  printf 'plugin %s manifest=%s boot=%s vendor=%s\n' "$n" $STATES
  printf '  %s\n  %s\n  %s\n' "$m" "$b" "$v"
  case "$m$b$v" in *blocked*) return 1;; esac
  return 0
}
run_list() {
  local rc=0 d total=0 line
  SUMMARY=$(mktemp)
  for d in "$@"; do [ -f "$d/plugin.json" ] || continue; total=$((total + 1))
    convert_one "$d" > "$SUMMARY.one" || rc=1; cat "$SUMMARY.one"; head -1 "$SUMMARY.one" >> "$SUMMARY"; done
  rm -f "$SUMMARY.one"
  echo "summary: $total plugins"
  for step in manifest boot vendor; do
    printf 'summary %s:' $step
    for st in converted noop none manual blocked skipped; do printf ' %s=%s' $st "$(grep -cE " $step=$st( |\$)" "$SUMMARY")"; done; echo
  done
  line=$(grep -E ' (manifest|boot|vendor)=blocked( |$)' "$SUMMARY" | awk '{ print $2 }' | tr '\n' ' '); echo "summary blocked: $line"
  line=$(grep -E ' (manifest|boot|vendor)=manual( |$)' "$SUMMARY" | awk '{ print $2 }' | tr '\n' ' '); echo "summary manual: $line"
  rm -f "$SUMMARY"; return $rc
}

fixtures() {
  local fx=$CONF_ROOT/tests/conformance/fixtures/v1 exp=$CONF_ROOT/tests/conformance/expected rc=0 d n
  [ -n "$out" ] || out=$(conf_work)/fx-converted
  rm -rf "$out"; mkdir -p "$out"; cp -R "$fx/." "$out/"
  # Dev-tool keys are kept by default: without --drop-dev-tool-keys a fixture that carries them is blocked and untouched.
  local nf; nf=$(conf_work)/fx-noflag; rm -rf "$nf"; mkdir -p "$nf"; cp -R "$fx/." "$nf/"
  local th; th=$(tree_hash "$nf"); EXTRA=; convert_one "$nf/alpha" >"$nf.log" 2>&1
  if grep -q 'manifest=blocked boot=skipped vendor=skipped' "$nf.log" && [ "$th" = "$(tree_hash "$nf")" ]; then echo "pass  dev-tool keys kept: alpha blocked and untouched without --drop-dev-tool-keys"
  else echo "FAIL  alpha was not blocked/untouched without --drop-dev-tool-keys"; cat "$nf.log"; rc=1; fi
  EXTRA=--drop-dev-tool-keys
  rc=$(( rc | $(fixture_atomicity "$fx" >&2; echo $?) ))
  local dirs=; for d in "$out"/*/; do [ -f "$d/plugin.json" ] && dirs="$dirs ${d%/}"; done
  run_list $dirs || rc=1
  local sdk; sdk=$(cli_src)/sdk/go/migrate
  for d in $dirs; do n=$(basename "$d")
    if [ $update -eq 1 ]; then rm -rf "$exp/$n"; mkdir -p "$exp/$n"; ( cd "$d" && tar cf - --exclude=vendor . ) | ( cd "$exp/$n" && tar xf - ); continue; fi
    diff -r -x vendor "$exp/$n" "$d" >/dev/null || { echo "FAIL golden $n: converted tree differs from tests/conformance/expected/$n"; diff -r -x vendor "$exp/$n" "$d" | head -20; rc=1; }
    if [ -d "$d/third_party/cli-sdk-go" ]; then
      for f in "$d"/third_party/cli-sdk-go/migrate/*.go; do cmp -s "$f" "$sdk/$(basename "$f")" || { echo "FAIL golden $n: $(basename "$f") differs from cli sdk/go/migrate"; rc=1; }; done
    fi
  done
  if [ $twice -eq 1 ]; then
    local h1 h2; h1=$(tree_hash "$out"); echo "--- second run"; STATES=
    for d in $dirs; do convert_one "$d" >/tmp/run.$$.txt || rc=1; cat /tmp/run.$$.txt
      for s in $STATES; do case $s in noop|none|manual) ;; *) echo "FAIL idempotence $(basename "$d"): second run reported $s"; rc=1;; esac; done; done
    rm -f /tmp/run.$$.txt; h2=$(tree_hash "$out")
    [ "$h1" = "$h2" ] && echo "idempotent: second run left the tree unchanged" || { echo "FAIL idempotence: second run changed files"; rc=1; }
  fi
  [ $rc -eq 0 ] && echo "codemods: fixtures ok" || echo "codemods: fixtures FAIL"
  return $rc
}
# fixture_atomicity <fixtures dir>: nested module reports manual and writes nothing from boot; a blocked manifest
# leaves the plugin untouched and skips boot and vendor; a failing vendor step rolls the whole plugin back.
fixture_atomicity() {
  local fx=$1 w rc=0 h log; w=$(conf_work)/fx-atomic
  scen() { rm -rf "$w"; mkdir -p "$w"; cp -R "$fx/." "$w/"; }
  scen; mkdir -p "$w/alpha/sdk"; printf 'module example.com/nest/sdk\n\ngo 1.25.0\n' > "$w/alpha/sdk/go.mod"; printf 'package sdk\n' > "$w/alpha/sdk/s.go"
  convert_one "$w/alpha" >"$w.log" 2>&1
  if grep -q 'boot=manual' "$w.log" && [ ! -f "$w/alpha/cmd/boot_migrations.go" ] && [ ! -f "$w/alpha/migrations/embed.go" ] && [ ! -d "$w/alpha/third_party/cli-sdk-go" ]; then
    echo "pass  nested go.mod: boot reports manual and writes nothing"; else echo "FAIL  nested go.mod plugin was written by the boot codemod"; cat "$w.log"; rc=1; fi
  scen; h=$(tree_hash "$w/alpha"); jq '.bogusUnmappedKey = 1' "$w/alpha/plugin.json" > "$w/p.tmp" && mv "$w/p.tmp" "$w/alpha/plugin.json"; h=$(tree_hash "$w/alpha")
  convert_one "$w/alpha" >"$w.log" 2>&1
  if grep -q 'manifest=blocked boot=skipped vendor=skipped' "$w.log" && [ "$h" = "$(tree_hash "$w/alpha")" ]; then echo "pass  blocked manifest: plugin untouched, boot and vendor skipped"
  else echo "FAIL  blocked manifest plugin was modified or ran boot/vendor"; cat "$w.log"; rc=1; fi
  scen; sed -i.bak 's#=> ../_libs/localsdk#=> ../_libs/missing#' "$w/alpha/go.mod"; rm -f "$w/alpha/go.mod.bak"; h=$(tree_hash "$w/alpha")
  convert_one "$w/alpha" >"$w.log" 2>&1
  if grep -q 'vendor=blocked' "$w.log" && [ "$h" = "$(tree_hash "$w/alpha")" ]; then echo "pass  failing vendor step: whole plugin rolled back to the original"
  else echo "FAIL  failing vendor step left a partial conversion"; cat "$w.log"; rc=1; fi
  return $rc
}
tree_hash() { ( cd "$1" && find . -type f | sort | while IFS= read -r f; do printf '%s %s\n' "$f" "$(sha256_of "$f")"; done ) | { sha256sum 2>/dev/null || shasum -a 256; } | cut -d' ' -f1; }

case $mode in
  fixtures) fixtures; exit $?;;
  all) dirs=$(ls -d "$CONF_ROOT"/free/*/ | sed 's#/$##');;
  plugins) dirs=$(for p in $(printf '%s' "$list" | tr ',' ' '); do echo "$CONF_ROOT/free/$p"; done);;
esac
# A dry run only sizes the batches: a blocked plugin is data in the report, not a failure of the run.
if [ -n "$report" ]; then run_list $dirs | tee "$report"; rc=${PIPESTATUS[0]}; else run_list $dirs; rc=$?; fi
[ $dry -eq 1 ] && exit 0
exit $rc
