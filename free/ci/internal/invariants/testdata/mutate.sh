#!/usr/bin/env bash
# mutate.sh: apply each planted-regression patch to a copy of a Go module and require the tests to fail on it.
# Usage: mutate.sh module patch-dir regex go-test-args...   (exit 0: baseline green and every patch KILLED)
# Env MUTATE_BASELINE: run (default: green baseline first, then every patch), only (the baseline alone, then exit 0, no
#   patch applied), skip (patches only, for copies whose identical baseline ran once elsewhere). D-1541: parallel copies
#   each ran the baseline and, under load, all hit the 120 s test timeout together; the drill now runs `only` once, alone,
#   then `skip` in the parallel copies. The 120 s limit is unchanged and a surviving patch still fails the run.
set -euo pipefail
if [ "$#" -lt 4 ]; then echo 'usage: mutate.sh module patch-dir regex go-test-args...' >&2; exit 2; fi
module=$(cd "$1" && pwd)
patch_dir=$2
regex=$3
shift 3
shopt -s nullglob
patches=("$module/$patch_dir"/*.patch)
if [ "${#patches[@]}" -eq 0 ]; then echo 'no patches' >&2; exit 1; fi
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
copy_module() {
  mkdir -p "$scratch/module"
  rsync -a --exclude vendor "$module/" "$scratch/module/"
  if [ -d "$module/vendor" ]; then ln -s "$module/vendor" "$scratch/module/vendor"; fi
  # Tests of the security suite read the sibling plugins (../../../../forgejo, ../../../../github-runner from
  # test/security/redteam). The copy lives in $scratch, so link those siblings next to it or the baseline is red.
  for sibling in forgejo github-runner; do
    if [ -d "$module/../$sibling" ] && [ ! -e "$scratch/$sibling" ]; then ln -s "$(cd "$module/../$sibling" && pwd)" "$scratch/$sibling"; fi
  done
}
baseline=${MUTATE_BASELINE:-run}
case "$baseline" in run | only | skip) ;; *) echo "MUTATE_BASELINE must be run, only or skip" >&2; exit 2 ;; esac
copy_module
if [ "$baseline" != skip ]; then
  if (cd "$scratch/module" && CGO_ENABLED=0 go test -mod=vendor -count=1 -timeout 120s -v -run "$regex" "$@") >"$scratch/baseline.out" 2>&1; then
    if ! grep -q -- '^--- PASS:' "$scratch/baseline.out"; then echo 'baseline empty'; exit 1; fi
  else
    echo 'baseline red'; cat "$scratch/baseline.out"; exit 1
  fi
  [ "$baseline" != only ] || exit 0
fi
failed=0
for patch_file in "${patches[@]}"; do
  rm -rf "$scratch/module"
  copy_module
  name=$(basename "$patch_file")
  if ! (cd "$scratch/module" && patch --batch -p1 < "$patch_file") >"$scratch/patch.out" 2>&1; then
    echo "UNAPPLICABLE $name"; cat "$scratch/patch.out"; failed=1; continue
  fi
  if (cd "$scratch/module" && CGO_ENABLED=0 go test -mod=vendor -count=1 -timeout 120s -v -run "$regex" "$@") >"$scratch/mutant.out" 2>&1; then
    echo "SURVIVED $name"; failed=1
  elif ! grep -q -- '^--- FAIL:' "$scratch/mutant.out"; then
    echo "SURVIVED $name (non-test failure)"; cat "$scratch/mutant.out"; failed=1
  else
    echo "KILLED $name"
  fi
done
exit "$failed"
