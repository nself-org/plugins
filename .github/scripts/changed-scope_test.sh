#!/usr/bin/env bash
# changed-scope_test.sh — table tests for changed-scope.sh and go-closure-changed.sh. Run from the repo root.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd); fail=0
tmp=$(mktemp -d "${TMPDIR:-/tmp}/scope-test.XXXXXX"); trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/repo/free/a" "$tmp/repo/free/b" "$tmp/repo/scripts/conformance"; cd "$tmp/repo"
# scope <event> <files...> -- <script args>; prints the outputs on one line.
scope() {
  local ev=$1; shift; local files=(); while [ "$1" != -- ]; do files+=("$1"); shift; done; shift
  printf '%s\n' "${files[@]}" >"$tmp/files"
  GITHUB_EVENT_NAME=$ev SCOPE_FILES=$tmp/files GITHUB_OUTPUT=$tmp/out bash "$here/changed-scope.sh" "$@" 2>/dev/null >/dev/null
  tr '\n' ' ' <"$tmp/out"; rm -f "$tmp/out"
}
check() { # name got want
  if [ "$2" = "$3" ]; then echo "ok   $1"; else echo "FAIL $1: got '$2' want '$3'"; fail=1; fi
}
check one-plugin   "$(scope pull_request free/a/main.go free/a/x/y.go -- --flag build='^scripts/conformance/')" "all=false plugins=a plugin_count=1 build=false "
check two-plugins  "$(scope pull_request free/a/m.go free/b/m.go README.md -- )" "all=false plugins=a,b plugin_count=2 "
check deleted-dir  "$(scope pull_request free/gone/m.go -- )" "all=false plugins= plugin_count=0 "
check flag-hit     "$(scope pull_request scripts/conformance/check.sh -- --flag build='^scripts/conformance/')" "all=false plugins= plugin_count=0 build=true "
check global-hit   "$(scope pull_request free/a/m.go sdk/x.go -- --global '^sdk/' --flag f='^zzz')" "all=true plugins= plugin_count=0 f=true "
check push-all     "$(scope push free/a/m.go -- --flag f='^zzz')" "all=true plugins= plugin_count=0 f=true "
check dispatch-all "$(scope workflow_dispatch -- )" "all=true plugins= plugin_count=0 "
check top-free     "$(scope pull_request free/stray.txt -- )" "all=true plugins= plugin_count=0 "
check self-change  "$(scope pull_request .github/scripts/changed-scope.sh -- )" "all=true plugins= plugin_count=0 "
check weird-name    "$(scope pull_request 'free/a;rm -rf x/m.go' -- )" "all=true plugins= plugin_count=0 "
exit $fail
