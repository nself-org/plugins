#!/usr/bin/env bash
set -euo pipefail
runner=$(cd "$(dirname "$0")" && pwd)/mutate.sh
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch/module/mutants"
printf 'module example.test/self\n\ngo 1.22\n' > "$scratch/module/go.mod"
printf 'package self\nfunc Value() int { return 1 }\n' > "$scratch/module/value.go"
printf 'package self\nimport "testing"\nfunc TestValue(t *testing.T) { if Value()!=1 { t.Fatal("wrong") } }\n' > "$scratch/module/value_test.go"
if bash "$runner" "$scratch/module" mutants TestValue ./... >"$scratch/out" 2>&1; then echo 'empty patches accepted'; exit 1; fi
grep -q 'no patches' "$scratch/out"
printf '%s\n' '--- a/value.go' '+++ b/value.go' '@@ -1,2 +1,2 @@' ' package self' '-func Value() int { return 1 }' '+func Value() int { return 2 }' > "$scratch/module/mutants/caught.patch"
if ! bash "$runner" "$scratch/module" mutants TestValue ./... >"$scratch/out" 2>&1; then cat "$scratch/out"; exit 1; fi
grep -q 'KILLED caught.patch' "$scratch/out"
printf '%s\n' '--- a/value.go' '+++ b/value.go' '@@ -1,2 +1,2 @@' ' package self' '-func Value() int { return 1 }' '+func Value() int { return 1+0 }' > "$scratch/module/mutants/caught.patch"
if bash "$runner" "$scratch/module" mutants TestValue ./... >"$scratch/out" 2>&1; then echo 'survivor accepted'; exit 1; fi
grep -q 'SURVIVED caught.patch' "$scratch/out"
if bash "$runner" "$scratch/module" mutants NoMatchingTest ./... >"$scratch/out" 2>&1; then echo 'empty baseline accepted'; exit 1; fi
grep -q 'baseline empty' "$scratch/out"
printf 'package self\nimport "testing"\nfunc TestValue(t *testing.T) { t.Fatal("red") }\n' > "$scratch/module/value_test.go"
if bash "$runner" "$scratch/module" mutants TestValue ./... >"$scratch/out" 2>&1; then echo 'red baseline accepted'; exit 1; fi
grep -q 'baseline red' "$scratch/out"
echo 'mutate-selftest PASS'
