#!/usr/bin/env bash
# validate-free-manifests.sh: every free/<slug>/plugin.json loads through the cli manifestv2 loader (D-0520).
# Usage: validate-free-manifests.sh [--work DIR]
# Builds tools/catalog at the cli commit pinned in scripts/cli-tools.version (scripts/conformance/lib.sh;
# CLI_TOOLS_SRC overrides the source) and runs it in -partial mode over free/ with empty releases and bundles,
# so every manifest is read through internal/plugin/manifestv2 (enums, np_ tables, duplicate min version keys)
# and nothing is written to the repo. Passes when the tool reports no problem, exits 0 and loaded every plugin
# directory. A planted-defect control runs first: a copy of a real manifest with an invalid language must be
# refused and its valid twin loaded, so the check cannot pass by reading nothing.
# Exit: 0 pass, 1 a manifest failed or a count disagrees, 9 setup failed.
set -u
HERE=$(cd "$(dirname "$0")" && pwd); WT=$(cd "$HERE/../.." && pwd)
work=${TMPDIR:-/tmp}/manifests-v2; rc=0
while [ $# -gt 0 ]; do case $1 in
  --work) work=$2; shift;; *) echo "validate-free-manifests.sh: unknown argument $1" >&2; exit 2;; esac; shift; done
rm -rf "$work"; mkdir -p "$work/out" "$work/ctl"
export CONF_WORKDIR=${CONF_WORKDIR:-$work/conf}
# shellcheck source=../../scripts/conformance/lib.sh
. "$WT/scripts/conformance/lib.sh"
cat=$(cli_tool catalog) || { echo "FAIL: cannot build tools/catalog at the pinned cli commit"; exit 9; }
printf '{"schema_version":1,"releases":{}}\n' > "$work/releases.json"
printf '{"schema_version":"2.0.0","bundles":{}}\n' > "$work/bundles.json"
# scan <tree>: run the loader over one plugin tree; output in $work/scan.txt, exit code returned.
scan() { "$cat" -mode free -plugins "$1" -releases "$work/releases.json" -bundles "$work/bundles.json" \
  -out "$work/out" -partial > "$work/scan.txt" 2>&1; }
loaded() { grep -c '^catalog: note: .* has no row in releases.json' "$work/scan.txt"; }
problems() { grep -c '^catalog: problem: ' "$work/scan.txt"; }

# Control: one valid twin, one planted defect.
src=$WT/free/monitoring/plugin.json
mkdir -p "$work/ctl/ctl-good" "$work/ctl/ctl-bad"
jq '.name = "ctl-good" | .language = "go"' "$src" > "$work/ctl/ctl-good/plugin.json" || exit 9
jq '.name = "ctl-bad" | .language = "config"' "$src" > "$work/ctl/ctl-bad/plugin.json" || exit 9
scan "$work/ctl"; c=$?
if [ "$c" -eq 2 ] && [ "$(problems)" -eq 1 ] && grep -q 'problem: .*ctl-bad/plugin.json: .*language' "$work/scan.txt" \
  && [ "$(loaded)" -eq 1 ]; then echo "control: planted defect refused, valid twin loaded"
else echo "FAIL: control (exit $c, problems $(problems), loaded $(loaded))"; sed -n 1,20p "$work/scan.txt"; exit 1; fi

# Real tree.
want=$(find "$WT/free" -mindepth 1 -maxdepth 1 -type d ! -name '_*' ! -name '.*' | wc -l | tr -d ' ')
scan "$WT/free"; c=$?
n=$(loaded); p=$(problems)
echo "free manifests: $want directories, $n loaded, $p problem(s), exit $c"
[ "$p" -eq 0 ] || { grep '^catalog: problem: ' "$work/scan.txt"; rc=1; }
[ "$c" -eq 0 ] || { echo "FAIL: catalog exit $c"; rc=1; }
[ "$n" -eq "$want" ] || { echo "FAIL: loaded $n of $want plugin directories"; rc=1; }
[ "$rc" -eq 0 ] && echo "PASS: every free manifest loads through manifestv2"
exit $rc
