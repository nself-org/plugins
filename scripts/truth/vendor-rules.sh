#!/usr/bin/env bash
# Vendor the nself-org truth rules into another repository (P7-TRUTH-04, Epic TRUTH D5).
# Purpose: a repo that enforces the truth gate keeps byte copies of .github/truth/org.yaml and
#   .github/truth/claims.yaml, so its own .github/truth.yaml can `include` them without a network
#   fetch (the check reads files only, Epic CI D21). This script writes those copies and checks them.
# Usage:   vendor-rules.sh <target-dir>          write <target-dir>/.github/truth/{org,claims}.yaml
#          vendor-rules.sh --check <target-dir>  exit 1 when a copy is missing or its body differs
# Format:  each copy is one header line, then the rule file's bytes unchanged:
#            # VENDORED FROM nself-org/plugins@<sha> — DO NOT HAND EDIT
#          <sha> is HEAD of this checkout. --check ignores the header line (the sha moves with every
#          release), so a copy only fails when its body no longer equals this checkout's rules.
# Outputs: one line per file on stdout; problems on stderr.
# Exit:    0 ok, 1 a copy is missing or differs (--check), 2 usage or a source file is missing.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../.." && pwd)
files=(org.yaml claims.yaml)
header_prefix='# VENDORED FROM nself-org/plugins@'

usage() {
	echo "usage: vendor-rules.sh [--check] <target-dir>" >&2
	exit 2
}

check=0
if [ "${1:-}" = "--check" ]; then
	check=1
	shift
fi
[ $# -eq 1 ] || usage
case "$1" in -*) usage ;; esac
target=$1
[ -d "$target" ] || { echo "vendor-rules: $target is not a directory" >&2; exit 2; }

for f in "${files[@]}"; do
	[ -f "$root/.github/truth/$f" ] || { echo "vendor-rules: $root/.github/truth/$f is missing" >&2; exit 2; }
done

if [ "$check" -eq 1 ]; then
	bad=0
	for f in "${files[@]}"; do
		copy="$target/.github/truth/$f"
		if [ ! -f "$copy" ]; then
			echo "vendor-rules: $copy is missing" >&2
			bad=1
			continue
		fi
		# The header line must be exactly the one this script writes (a lone CR would hide YAML from the comparison).
		first=$(head -n 1 "$copy")
		if ! [[ $first =~ ^"$header_prefix"[0-9a-f]{40}" — DO NOT HAND EDIT"$ ]]; then
			echo "vendor-rules: $copy has no valid VENDORED header line" >&2
			bad=1
			continue
		fi
		if tail -n +2 "$copy" | cmp -s - "$root/.github/truth/$f"; then
			echo "ok $f"
		else
			echo "vendor-rules: $copy differs from nself-org/plugins .github/truth/$f" >&2
			bad=1
		fi
	done
	exit "$bad"
fi

sha=$(git -C "$root" rev-parse HEAD 2>/dev/null) || { echo "vendor-rules: $root is not a git checkout" >&2; exit 2; }
mkdir -p "$target/.github/truth"
for f in "${files[@]}"; do
	dest="$target/.github/truth/$f"
	tmp=$(mktemp "$dest.XXXXXX")
	trap 'rm -f "$tmp"' EXIT
	{
		printf '%s%s — DO NOT HAND EDIT\n' "$header_prefix" "$sha"
		cat "$root/.github/truth/$f"
	} >"$tmp"
	chmod 644 "$tmp"
	mv "$tmp" "$dest"
	echo "wrote $f"
done
