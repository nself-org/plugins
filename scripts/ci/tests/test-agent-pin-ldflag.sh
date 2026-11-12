#!/usr/bin/env bash
# shellcheck disable=SC2319,SC2016 # $? after a test is the point; the patterns are literal
# test-agent-pin-ldflag.sh
#
# Purpose: P7-WIRE-12. Both release build paths give the ci plugin -X main.agentPinnedVersion=<tag version>, and no
#   other plugin: plugin_ldflags in scripts/build-tarballs.sh, and the inline copy in
#   .github/workflows/release-tarballs.yml, which must agree. The workflow also checks the built binary.
# Inputs: none (reads the two files from this checkout).
# Outputs: "ok:"/"FAIL:" per case, final summary; exit 1 on any failure.
# Constraints: bash 3.2 compatible; no go, no network. The Go side (the built binary reports the pin and `nodes add`
#   takes the Release source) is TestWire12PinnedBuildReportsPin in free/ci/cmd.
set -u

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
script="$repo/scripts/build-tarballs.sh"
wf="$repo/.github/workflows/release-tarballs.yml"
fail=0

check() { # check NAME CONDITION-RESULT DETAIL
  if [ "$2" -eq 0 ]; then echo "ok: $1"; else echo "FAIL: $1: $3" >&2; fail=1; fi
}

# shellcheck source=/dev/null
. "$script" || { echo "FAIL: cannot source $script" >&2; exit 1; }
set +e +u # the script's main sets -euo pipefail only when run; sourcing must not leave them on

got="$(plugin_ldflags ci 1.2.3 2>&1)"
[ "$got" = "-s -w -X main.agentPinnedVersion=1.2.3" ]; check "build-tarballs.sh pins the ci plugin" $? "$got"
got="$(plugin_ldflags notify 1.2.3 2>&1)"
[ "$got" = "-s -w" ]; check "build-tarballs.sh leaves other plugins unpinned" $? "$got"

grep -q 'go build -trimpath -ldflags="\$ldflags"' "$script"; check "build_platform_tarballs builds with plugin_ldflags" $? "no -ldflags=\"\$ldflags\""
! grep -q 'ldflags="-s -w" *\\$' "$script"; check "build-tarballs.sh has no hard-coded -s -w build left" $? "found one"

# The workflow's rule: the two lines that compute ldflags in build_plugin, run for each plugin and compared.
rule="$(grep -E '^ *local ldflags="-s -w"$|^ *\[ "\$plugin_name" = "ci" \] && ldflags=' "$wf" | sed 's/^ *local //')"
[ "$(printf '%s\n' "$rule" | wc -l | tr -d ' ')" -eq 2 ]; check "the workflow computes ldflags in two lines" $? "$rule"
for p in ci notify; do
  wfgot="$(plugin_name="$p" VERSION=1.2.3 bash -c "$rule"'
printf "%s\n" "$ldflags"' 2>&1)"
  [ "$wfgot" = "$(plugin_ldflags "$p" 1.2.3)" ]; check "workflow and build-tarballs.sh agree for $p" $? "workflow: $wfgot"
done
grep -q 'go build -trimpath -ldflags="\$ldflags"' "$wf"; check "the workflow builds with its ldflags" $? "no -ldflags=\"\$ldflags\""
! grep -q 'ldflags="-s -w" *\\$' "$wf"; check "the workflow has no hard-coded -s -w build left" $? "found one"

grep -q 'name: Verify the ci plugin build carries the agent pin' "$wf"; check "the release checks the built binary's pin" $? "step missing"
grep -q 'select(.id == "CI-DN013")' "$wf"; check "the release check reads CI-DN013" $? "no CI-DN013 probe"

if [ "$fail" -ne 0 ]; then echo "test-agent-pin-ldflag: FAILED" >&2; exit 1; fi
echo "test-agent-pin-ldflag: all passed"
