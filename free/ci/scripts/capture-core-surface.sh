#!/usr/bin/env bash
# capture-core-surface.sh NSELF_BIN: record the core `nself ci` help surface (P7-CANON-09).
# Purpose: the nself-ci binary prints the same --help text as core `nself ci`; this script captures
# that text from a built core binary so it is never typed by hand.
# Inputs:  NSELF_BIN, a core nself built from cli origin/main; the plugin proxy binary need not exist
#          for --help (cobra answers it).
# Outputs: testdata/core-surface/{ci,ci-build,ci-forgejo,ci-serve}.help.txt, help-json-ci.json (the
#          `help --json ci` subtree), and byte copies of the four help files in internal/surface/help/
#          (embedded by the binary; a unit test fails if the two copies drift).
# Exit:    0 captured, 1 a capture was empty or the tool failed, 2 usage.
set -euo pipefail
bin=${1:-}
[ -n "$bin" ] && [ -x "$bin" ] || { echo "usage: capture-core-surface.sh NSELF_BIN" >&2; exit 2; }
here=$(cd "$(dirname "$0")/.." && pwd)
td=$here/testdata/core-surface
eh=$here/internal/surface/help
mkdir -p "$td" "$eh"
cap() { # cap <outfile-base> <args...>
  local base=$1; shift
  "$bin" "$@" > "$td/$base.help.txt" 2>/dev/null
  [ -s "$td/$base.help.txt" ] || { echo "capture-core-surface: empty help for: $*" >&2; exit 1; }
  cp "$td/$base.help.txt" "$eh/$base.txt"
}
cap ci ci --help
cap ci-build ci build --help
cap ci-forgejo ci forgejo --help
cap ci-serve ci serve --help
"$bin" help --json ci | python3 -c '
import json, sys
d = json.load(sys.stdin)["data"]
keep = [c for c in d["commands"] if c["path"] == "nself ci" or c["path"].startswith("nself ci ")]
if not keep:
    sys.exit("capture-core-surface: no ci commands in help --json")
json.dump({"schema_version": d["schema_version"], "commands": keep}, sys.stdout, indent=2, sort_keys=True)
print()
' > "$td/help-json-ci.json"
echo "captured: $(ls "$td" | tr '\n' ' ')"
