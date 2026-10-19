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
check outside-docs  "$(scope pull_request free/a/m.go README.md registry.json -- --outside-free-global)" "all=false plugins=a plugin_count=1 "
check outside-code  "$(scope pull_request free/a/m.go scripts/ci/x.sh -- --outside-free-global)" "all=true plugins= plugin_count=0 "
check outside-flag  "$(scope pull_request free/a/m.go scripts/ci/x.sh -- )" "all=false plugins=a plugin_count=1 "
# Real git: a merge commit whose second parent renames a file out of free/a and adds an unusually named file.
git init -q . && git config user.email t@t && git config user.name t && git checkout -qb main
mkdir -p free/a free/b && echo x > free/a/util.go && echo k > free/a/keep.go && echo y > free/b/keep.go && git add -A && git commit -qm base
git checkout -qb pr && git mv free/a/util.go archive.go && printf z > "free/b/caf\303\251.go" && git add -A && git commit -qm change
git checkout -q main && echo w > unrelated.txt && git add -A && git commit -qm moved && git merge -q --no-ff pr -m merge
got=$(GITHUB_EVENT_NAME=pull_request GITHUB_OUTPUT=$tmp/out2 bash "$here/changed-scope.sh" 2>/dev/null; tr '\n' ' ' <"$tmp/out2")
check git-rename-quote "$got" "all=false plugins=a,b plugin_count=2 "
exit $fail
