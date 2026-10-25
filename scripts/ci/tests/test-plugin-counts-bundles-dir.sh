#!/usr/bin/env bash
# test-plugin-counts-bundles-dir.sh
#
# Purpose: regression cases for the bundles checkout autodetect of scripts/plugin-counts.sh (D-1169). From a git worktree
#   of the plugins repo the script looks beside the MAIN checkout: both <main>/../bundles and <main>/../plugins-pro must
#   be found (the bundles sibling was missing).
# Inputs: none. Builds a throwaway tree: <tmp>/nself/plugins (a repo holding the script), a worktree of it under
#   .claude/worktrees, and an empty git repo as the bundles sibling.
# Outputs: "ok:"/"FAIL:" per case, final summary; exit 1 on any failure.
# Constraints: bash 3.2 compatible; no docker, no network; the sibling holds no commit, so the script stops at its
#   "does not hold <ref>" precondition (exit 3) and the message names the checkout it detected.
set -u

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/plugin-counts-bd.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT
fail=0

main="$tmp/nself/plugins"
mkdir -p "$main/scripts" "$tmp/nself/.claude"
cp "$repo/scripts/plugin-counts.sh" "$repo/scripts/bundles.ref" "$main/scripts/"
echo '{}' > "$main/releases.json"
g() { git -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false "$@"; }
g -C "$main" init -q . && g -C "$main" add scripts releases.json && g -C "$main" commit -q -m seed || { echo "FAIL: cannot build the fixture repo" >&2; exit 1; }
g -C "$main" worktree add -q "$main/.claude/worktrees/wt" -b wt || { echo "FAIL: cannot add the worktree" >&2; exit 1; }
wt="$main/.claude/worktrees/wt"

# run_case NAME SIBLING WANT_RC: run --check from the worktree with only SIBLING beside the main checkout.
run_case() {
  name="$1" sibling="$2" want="$3"
  rm -rf "$tmp/nself/bundles" "$tmp/nself/plugins-pro"
  [ -z "$sibling" ] || g init -q "$tmp/nself/$sibling"
  out="$(env -u BUNDLES_DIR -u NSELF bash "$wt/scripts/plugin-counts.sh" --check 2>&1)"; rc=$?
  case "$want" in
    none) ok=0; [ "$rc" -eq 3 ] && printf '%s\n' "$out" | grep -q 'no bundles checkout' && ok=1 ;;
    *) ok=0; [ "$rc" -eq 3 ] && printf '%s\n' "$out" | grep -q "/\.\./\.\./$want does not hold" && ok=1 ;;
  esac
  if [ "$ok" -eq 1 ]; then echo "ok: $name"; else echo "FAIL: $name (rc $rc): $out" >&2; fail=1; fi
}

run_case worktree-finds-bundles-sibling bundles bundles
run_case worktree-finds-plugins-pro-sibling plugins-pro plugins-pro
run_case worktree-without-sibling-says-so "" none

if [ "$fail" -ne 0 ]; then echo "test-plugin-counts-bundles-dir: FAILED" >&2; exit 1; fi
echo "test-plugin-counts-bundles-dir: OK"
