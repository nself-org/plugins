#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir "$T/assets"
printf 'built source' > "$T/assets/demo-1.0.0.tar.gz"
(cd "$T/assets" && shasum -a 256 *.tar.gz) > "$T/built.txt"
for scenario in built empty replaced renamed; do
 cp "$T/built.txt" "$T/list.txt"
 cp "$T/assets/demo-1.0.0.tar.gz" "$T/original"
 case "$scenario" in
 empty) : > "$T/list.txt" ;;
 replaced) printf 'foreign source' > "$T/assets/demo-1.0.0.tar.gz" ;;
 renamed) sed 's/demo/other/' "$T/built.txt" > "$T/list.txt" ;;
 esac
 rc=0
 bash "$HERE/platform-built-check.sh" "$T/list.txt" "$T/assets" >/dev/null 2>&1 || rc=$?
 cp "$T/original" "$T/assets/demo-1.0.0.tar.gz"
 if { [ "$scenario" = built ] && [ "$rc" != 0 ]; } || { [ "$scenario" != built ] && [ "$rc" = 0 ]; }; then
  echo "FAIL source built check: $scenario (exit $rc)" >&2
  exit 1
 fi
 echo "PASS source built check: $scenario"
done
