#!/usr/bin/env bash
# go-closure-changed.sh — did a PR change anything a set of Go packages is built from?
#
# Purpose: gate an expensive Go test leg on the packages it really runs, found with `go list -deps` instead of
#   a hand-kept path list that would go stale the day someone adds an import.
# Usage:   go-closure-changed.sh <module-dir> <changed-files-list> <package-pattern>...
#   Patterns are relative to the module (./internal/gates/...); ones whose directory is missing are skipped.
#   CLOSURE_EXTRA="dir1 dir2"  extra repo-relative directories that also count (fixtures).
#   CLOSURE_TAGS  build tags to include (default integration,security,darwin_integration).
# Output:  true or false on stdout. Changes to the module's go.mod, go.sum or vendor/ always count.
# Constraints: fail open. A `go list` failure, no usable package or an unreadable list prints true.
set -uo pipefail

mod=${1:?module dir}; files=${2:?changed files list}; shift 2
[ -r "$files" ] || { echo true; exit 0; }
pats=()
for p in "$@"; do
  d=${p%/...}
  [ -d "$mod/$d" ] && pats+=("$p")
done
[ ${#pats[@]} -gt 0 ] || { echo true; exit 0; }

modflag=; [ -d "$mod/vendor" ] && modflag=-mod=vendor
root=$(cd "$mod" && git rev-parse --show-toplevel)
# Union over the platforms CI builds on and the tags its tests use, so darwin-only or integration-only files count.
dirs=
for goos in linux darwin; do
  d=$(cd "$mod" && GOOS=$goos CGO_ENABLED=0 go list $modflag -tags "${CLOSURE_TAGS:-integration,security,darwin_integration}" -test -deps -f '{{if not .Standard}}{{.Dir}}{{end}}' "${pats[@]}" 2>/dev/null) || { echo true; exit 0; }
  dirs+="$d"$'\n'
done
[ -n "$dirs" ] || { echo true; exit 0; }

top=$(cd "$root" && pwd -P)
modrel=$(cd "$mod" && pwd -P); modrel=${modrel#"$top"/}
prefixes=$(printf '%s\n' "$dirs" | sed "s#^$top/##" | grep -v "^$modrel/vendor/" | sort -u)
prefixes+=$'\n'"$modrel/go.mod"$'\n'"$modrel/go.sum"$'\n'"$modrel/vendor"
for x in ${CLOSURE_EXTRA:-}; do prefixes+=$'\n'"$x"; done

# A file counts when it is a prefix directory's content or one of the named files.
while IFS= read -r f; do
  [ -n "$f" ] || continue
  while IFS= read -r p; do
    [ -n "$p" ] || continue
    if [ "$f" = "$p" ] || [ "${f#"$p"/}" != "$f" ]; then echo true; exit 0; fi
  done <<<"$prefixes"
done <"$files"
echo false
