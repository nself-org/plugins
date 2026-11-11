#!/usr/bin/env bash
# test-nself-ci-template-parity.sh
#
# Purpose: P7-WIRE-06. The shipped template free/ci/templates/nself-ci.yml and the plugins repo's own
#   .github/workflows/nself-ci.yml (instantiated from it) must stay the same workflow. They may differ only
#   in what an instance is told to fill in: comments, the pinned env values (CI_VERSION, CI_RELEASE_URL,
#   CI_RELEASE_SHA256 and any extra pin such as GITLEAKS_SHA256), and the setup steps between the checkout
#   and "Install the pinned nself-ci". Any other drift (for example the NSELF_CI_TRUST step) fails.
# Inputs: optional TEMPLATE and INSTANCE paths (default: the two files of this checkout).
# Outputs: "ok:" or "FAIL:" with a unified diff of the normalised files; exit 0 equal, 1 differ, 2 usage.
# Constraints: bash 3.2 compatible; awk, sed and diff only; no network.
set -u

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
template="${1:-$repo/free/ci/templates/nself-ci.yml}"
instance="${2:-$repo/.github/workflows/nself-ci.yml}"
if [ "$#" -gt 2 ] || [ ! -f "$template" ] || [ ! -f "$instance" ]; then
  echo "usage: $0 [TEMPLATE INSTANCE] (both files must exist)" >&2
  exit 2
fi

# normalise FILE: drop comments and blank lines, blank the pinned env values, drop extra pins and the
# instance's setup steps, so only the contract of the workflow is left.
normalise() {
  sed -E -e 's/^[[:space:]]*#.*$//' -e 's/[[:space:]]+#[[:space:]].*$//' -e 's/[[:space:]]+$//' "$1" |
    awk '
      /^$/ { next }
      /^env:$/ { inenv = 1; print; next }
      inenv && /^[^ ]/ { inenv = 0 }
      inenv {
        if ($1 ~ /^(CI_VERSION|CI_RELEASE_URL|CI_RELEASE_SHA256):$/) { print "  " $1 " \"\""; next }
        if ($1 ~ /_SHA256:$/) { next }
      }
      /^[[:space:]]+fetch-depth: 0$/ { print; skip = 1; next }
      skip && /^[[:space:]]+- name: Install the pinned nself-ci$/ { skip = 0 }
      skip { next }
      { print }
    '
}

tmp="$(mktemp -d "${TMPDIR:-/tmp}/nself-ci-parity.XXXXXX")" || exit 1
trap 'rm -rf "$tmp"' EXIT
normalise "$template" > "$tmp/template"
normalise "$instance" > "$tmp/instance"

if ! grep -q 'NSELF_CI_TRUST:' "$tmp/template"; then
  echo "FAIL: $template has no NSELF_CI_TRUST step" >&2
  exit 1
fi
if diff -u "$tmp/template" "$tmp/instance" > "$tmp/diff"; then
  echo "ok: the template and the instantiated workflow agree except for pinned values"
  exit 0
fi
echo "FAIL: $template and $instance differ beyond pinned values:" >&2
cat "$tmp/diff" >&2
exit 1
