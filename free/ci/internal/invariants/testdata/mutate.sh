#!/usr/bin/env bash
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
}
copy_module
if (cd "$scratch/module" && CGO_ENABLED=0 go test -mod=vendor -count=1 -timeout 120s -v -run "$regex" "$@") >"$scratch/baseline.out" 2>&1; then
  if ! grep -q -- '^--- PASS:' "$scratch/baseline.out"; then echo 'baseline empty'; exit 1; fi
else
  echo 'baseline red'; cat "$scratch/baseline.out"; exit 1
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
