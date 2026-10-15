#!/usr/bin/env bash
# Purpose: reject dependency metadata drift in independently built Go modules.
# Inputs: tracked go.mod files; Go toolchain and dependency cache/network.
# Outputs: tidy diffs and a nonzero status if any module needs tidying.
# Constraints: ignore vendor, fixture and third-party modules; disable workspaces.
# SPORT: D-0654.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
failed=0
while IFS= read -r gomod; do
  case "$gomod" in
    */vendor/*|*/third_party/*|*/testdata/*|*/sdk/*) continue ;;
    free/*|sdk/go.mod|scripts/*) ;;
    *) continue ;;
  esac
  module="${gomod%/go.mod}"
  echo "=== tidy: $module ==="
  (cd "$module" && GOWORK=off go mod tidy -diff) || failed=1
done < <(git ls-files 'go.mod' '**/go.mod')
exit "$failed"
