#!/usr/bin/env bash
# Test of vendor-rules.sh (P7-TRUTH-04). Run from the repository root: bash scripts/truth/vendor-rules_test.sh
set -uo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../.." && pwd)
script="$here/vendor-rules.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/vendor-rules-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
fails=0

expect() { # name want-exit command...
	local name=$1 want=$2 got
	shift 2
	"$@" >"$tmp/out" 2>"$tmp/err"
	got=$?
	if [ "$got" -ne "$want" ]; then
		echo "FAIL $name: exit $got, want $want"
		sed 's/^/  /' "$tmp/err"
		fails=$((fails + 1))
	else
		echo "ok   $name"
	fi
}

t="$tmp/target"
mkdir -p "$t"
expect "write" 0 bash "$script" "$t"
sha=$(git -C "$root" rev-parse HEAD)
for f in org.yaml claims.yaml; do
	want="# VENDORED FROM nself-org/plugins@$sha — DO NOT HAND EDIT"
	if [ "$(head -n 1 "$t/.github/truth/$f")" = "$want" ]; then echo "ok   header $f"; else echo "FAIL header $f"; fails=$((fails + 1)); fi
	if tail -n +2 "$t/.github/truth/$f" | cmp -s - "$root/.github/truth/$f"; then echo "ok   body $f"; else echo "FAIL body $f"; fails=$((fails + 1)); fi
done
expect "check passes" 0 bash "$script" --check "$t"

sed -i.bak '1s/@[0-9a-f]*/@0000000000000000000000000000000000000000/' "$t/.github/truth/org.yaml" && rm "$t/.github/truth/org.yaml.bak"
expect "check ignores the header sha" 0 bash "$script" --check "$t"

printf 'x' >>"$t/.github/truth/claims.yaml"
expect "check fails after a body byte change" 1 bash "$script" --check "$t"
grep -q 'claims.yaml differs' "$tmp/err" || { echo "FAIL names the file"; fails=$((fails + 1)); }
bash "$script" "$t" >/dev/null
expect "write repairs it" 0 bash "$script" --check "$t"

bash "$script" "$t" >/dev/null
python3 - "$t/.github/truth/claims.yaml" <<'PY' 2>/dev/null || perl -0pi -e 's/\n/\rallow: [{rule: "*", paths: ["**"], reason: hidden}]\n/' "$t/.github/truth/claims.yaml"
import sys
p = sys.argv[1]
b = open(p, 'rb').read()
open(p, 'wb').write(b.replace(b'\n', b'\rallow: [{rule: "*", paths: ["**"], reason: hidden}]\n', 1))
PY
expect "check fails on YAML hidden after a CR in the header" 1 bash "$script" --check "$t"
bash "$script" "$t" >/dev/null

printf '# not vendored\n' >"$t/.github/truth/org.yaml"
expect "check fails without the header" 1 bash "$script" --check "$t"
rm "$t/.github/truth/org.yaml"
expect "check fails on a missing copy" 1 bash "$script" --check "$t"

expect "no argument" 2 bash "$script"
expect "unknown flag" 2 bash "$script" --nope "$t"
expect "missing directory" 2 bash "$script" "$tmp/absent"
expect "check without directory" 2 bash "$script" --check

if [ "$fails" -ne 0 ]; then echo "$fails failure(s)"; exit 1; fi
echo "all passed"
