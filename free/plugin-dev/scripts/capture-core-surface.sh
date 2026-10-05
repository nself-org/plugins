#!/usr/bin/env bash
# capture-core-surface.sh NSELF_BIN: record the core `nself plugin <author command>` help (P7-CANON-18).
# Purpose: the nself-plugin-dev binary prints the same --help text as core; this script captures that
# text from a built core binary so it is never typed by hand.
# Inputs:  NSELF_BIN, a core nself built from cli origin/main.
# Outputs: testdata/core-surface/<cmd>.help.txt and byte copies in internal/surface/help/<cmd>.txt
#          (embedded by the binary; a unit test fails if the two copies drift), for init, new, dev,
#          debug, link, unlink, test; the deprecation notice core prints for `new` goes to stderr and
#          is not part of the help text.
# Exit:    0 captured, 1 a capture was empty, 2 usage.
set -euo pipefail
bin=${1:-}
[ -n "$bin" ] && [ -x "$bin" ] || { echo "usage: capture-core-surface.sh NSELF_BIN" >&2; exit 2; }
here=$(cd "$(dirname "$0")/.." && pwd)
td=$here/testdata/core-surface
eh=$here/internal/surface/help
mkdir -p "$td" "$eh"
home=$(mktemp -d); trap 'rm -rf "$home"' EXIT
for c in init new dev debug link unlink test; do
  HOME=$home NO_COLOR=1 "$bin" plugin "$c" --help > "$td/$c.help.txt" 2>/dev/null </dev/null
  [ -s "$td/$c.help.txt" ] || { echo "capture-core-surface: empty help for: plugin $c" >&2; exit 1; }
  cp "$td/$c.help.txt" "$eh/$c.txt"
done
echo "captured: $(ls "$td" | tr '\n' ' ')"
