#!/usr/bin/env bash
# Rule migrations (ADR 0010): a plugin with migrations/*.sql declares migrations.apply boot, and a Go
# plugin calls migrate.Apply at boot: a real call in non-test source, not a comment and not a _test.go file.
# A non-Go plugin implements the contract itself.
set -u
. "$(dirname "$0")/../lib.sh"
dir=$1; name=$(plugin_name "$dir"); pj=$dir/plugin.json
ls "$dir"/migrations/*.sql >/dev/null 2>&1 || exit 0
rc=0
[ "$(jq -r '.migrations.apply // ""' "$pj")" = boot ] || { fail migrations "$name" "migrations/ has SQL files but the manifest lacks migrations.apply: boot"; rc=1; }
# code_only <file>: the Go file without // and /* */ comments.
code_only() { awk 'BEGIN { c = 0 } { l = $0; out = ""
    while (length(l) > 0) {
      if (c) { i = index(l, "*/"); if (i == 0) { l = ""; break } l = substr(l, i + 2); c = 0; continue }
      a = index(l, "//"); b = index(l, "/*")
      if (a > 0 && (b == 0 || a < b)) { out = out substr(l, 1, a - 1); l = ""; break }
      if (b > 0) { out = out substr(l, 1, b - 1); l = substr(l, b + 2); c = 1; continue }
      out = out l; l = "" }
    print out }' "$1"; }
if [ "$(jq -r '.language // ""' "$pj")" = go ]; then
  found=0
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    code_only "$f" | grep -qE 'migrate\.Apply\(' && { found=1; break; }
  done <<EOF
$(find "$dir" \( -name vendor -o -name node_modules -o -name third_party \) -prune -o -name '*.go' ! -name '*_test.go' -type f -print)
EOF
  [ $found -eq 1 ] || { fail migrations "$name" "migrations/ present but no migrate.Apply call in non-test Go source (comments and _test.go do not count)"; rc=1; }
fi
exit $rc
