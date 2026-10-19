#!/usr/bin/env bash
# changed-scope.sh — decide what a pull request can break, so CI runs only the legs it needs.
#
# Purpose: a PR that touches free/ci should not rebuild the other plugins, run four docker build shards,
#   or start a macOS runner. This lists the files the PR changed and turns them into job inputs.
# Usage:   changed-scope.sh [--global REGEX]... [--flag NAME=REGEX]...
#   --global REGEX   a changed file matching it means "run everything" (all=true).
#   --flag NAME=REGEX  output NAME=true when all=true or any changed file matches REGEX.
# Inputs:  checkout with fetch-depth >= 2 (a pull_request run checks out the merge commit; its first parent
#   is the base tip, so `HEAD^1..HEAD` is exactly what the PR adds). GITHUB_EVENT_NAME, GITHUB_OUTPUT.
#   SCOPE_FILES=<file> replaces the diff with a list of paths (tests). SCOPE_LIST_OUT=<file> keeps the changed-file
#   list for a later step (go-closure-changed.sh).
# Outputs (to GITHUB_OUTPUT and stdout):
#   all=true        not a pull_request, the diff is unavailable, the list is huge, a --global match, a file
#                   directly under free/, or this script changed. Always the safe answer: run everything.
#   plugins=a,b     free/<name> directories the PR touched that still exist (empty when all=true).
#   plugin_count=N
#   <flag>=true|false
# Constraints: fail open. Any doubt sets all=true; this script never lowers coverage on push, schedule or dispatch.
set -euo pipefail

globals=(); flags=()
while [ $# -gt 0 ]; do
  case $1 in
    --global) globals+=("$2"); shift 2;;
    --flag) flags+=("$2"); shift 2;;
    *) echo "changed-scope: unknown argument $1" >&2; exit 2;;
  esac
done

event=${GITHUB_EVENT_NAME:-pull_request}
out=${GITHUB_OUTPUT:-/dev/stdout}
list=$(mktemp "${TMPDIR:-/tmp}/changed-scope.XXXXXX"); trap 'rm -f "$list"' EXIT
all=false; reason=

if [ "$event" != pull_request ]; then
  all=true; reason="event is $event"
elif [ -n "${SCOPE_FILES:-}" ]; then
  cat "$SCOPE_FILES" >"$list"
elif git rev-parse -q --verify 'HEAD^2' >/dev/null 2>&1 && git diff --name-only 'HEAD^1' HEAD >"$list" 2>/dev/null; then
  :
else
  all=true; reason="no merge commit to diff (use fetch-depth: 2)"
fi

if [ "$all" = false ]; then
  n=$(grep -c . "$list" || true)
  if [ "$n" -ge 2500 ]; then all=true; reason="$n changed files"; fi
  if grep -qE '^\.github/scripts/changed-scope\.sh$' "$list"; then all=true; reason="changed-scope.sh changed"; fi
  if grep -qE '^free/[^/]+$' "$list"; then all=true; reason="a file sits directly under free/"; fi
  # Names end up in command lines downstream; a directory name outside the plain set is never trusted.
  if sed -nE 's#^free/([^/]+)/.*#\1#p' "$list" | grep -qvE '^[A-Za-z0-9][A-Za-z0-9._-]*$'; then all=true; reason="unusual plugin directory name"; fi
  for g in "${globals[@]+"${globals[@]}"}"; do
    if grep -qE "$g" "$list"; then all=true; reason="global match: $g"; break; fi
  done
fi

[ -z "${SCOPE_LIST_OUT:-}" ] || cp "$list" "$SCOPE_LIST_OUT"

plugins=
if [ "$all" = false ]; then
  while IFS= read -r name; do
    [ -n "$name" ] && [ -d "free/$name" ] && plugins+="${plugins:+,}$name"
  done < <(sed -nE 's#^free/([^/]+)/.*#\1#p' "$list" | sort -u)
fi
count=0; [ -z "$plugins" ] || count=$(printf '%s' "$plugins" | tr ',' '\n' | grep -c .)

{
  echo "all=$all"
  echo "plugins=$plugins"
  echo "plugin_count=$count"
  for f in "${flags[@]+"${flags[@]}"}"; do
    name=${f%%=*}; re=${f#*=}
    if [ "$all" = true ] || grep -qE "$re" "$list"; then echo "$name=true"; else echo "$name=false"; fi
  done
} >>"$out"

echo "changed-scope: all=$all${reason:+ ($reason)} plugins=${plugins:-none}" >&2
