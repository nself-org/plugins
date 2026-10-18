#!/usr/bin/env bash
# plugin-yaml-licence.sh: a free/<slug>/plugin.yaml must not contradict plugin.json on licence (D-0957).
# Usage: plugin-yaml-licence.sh [--root DIR]   (DIR holds <slug>/plugin.json and <slug>/plugin.yaml; default free/)
# plugin.json is the manifest the registry and the cli read. A leftover plugin.yaml that still says
# licenseType pro / requires_license true / requiredEntitlements [pro] for a free plugin misleads readers
# and tools. Compared keys: isCommercial, licenseType, requires_license, tier (when the yaml has them) and the
# number of requiredEntitlements. KNOWN_DRIFT lists slugs with the same defect that are tracked elsewhere; it
# may only shrink. A planted-defect control runs first so the check cannot pass by reading nothing.
# Exit: 0 pass, 1 drift found or the control failed, 2 usage.
set -u
HERE=$(cd "$(dirname "$0")" && pwd); WT=$(cd "$HERE/../.." && pwd)
root=$WT/free
while [ $# -gt 0 ]; do case $1 in
  --root) root=$2; shift;; *) echo "plugin-yaml-licence.sh: unknown argument $1" >&2; exit 2;; esac; shift; done
KNOWN_DRIFT=" nself-eval-gate nself-sync nself-vault payments warehouse "

# yaml_scalar <file> <key>: value of a top-level "key: value" line, comment and quotes stripped.
yaml_scalar() { sed -n "s/^$2:[[:space:]]*\([^#]*\).*/\1/p" "$1" | head -1 | tr -d "\"' " ; }
# yaml_list_len <file> <key>: items of a top-level block list under "key:".
yaml_list_len() { awk -v k="$1:" '$0 ~ "^" k {on=1; next} on && /^[[:space:]]+-[[:space:]]/ {n++; next} on {exit} END {print n+0}' "$2" ; }

# check_dir <root>: print one line per drift; return 1 when any.
check_dir() {
  local d slug y j key want got bad=0
  for y in "$1"/*/plugin.yaml; do
    [ -e "$y" ] || continue
    d=$(dirname "$y"); slug=$(basename "$d"); j=$d/plugin.json
    [ -f "$j" ] || continue
    for key in isCommercial licenseType requires_license tier; do
      got=$(yaml_scalar "$y" "$key"); [ -n "$got" ] || continue
      # plugin.json may omit the two booleans; then they follow its licenseType (anything but free is commercial).
      want=$(jq -r --arg k "$key" 'if has($k) then .[$k] | tostring elif $k == "isCommercial" or $k == "requires_license" then ((.licenseType // .tier // "free") != "free") | tostring else "" end' "$j")
      [ "$got" = "$want" ] || { echo "$slug: plugin.yaml $key=$got, plugin.json $key=$want"; bad=1; }
    done
    got=$(yaml_list_len requiredEntitlements "$y"); want=$(jq -r '(.requiredEntitlements // []) | length' "$j")
    [ "$got" = "$want" ] || { echo "$slug: plugin.yaml requiredEntitlements has $got item(s), plugin.json has $want"; bad=1; }
  done
  return $bad
}

# Control: a planted pro yaml next to a free json must be reported, its matching twin must pass.
ctl=$(mktemp -d "${TMPDIR:-/tmp}/yaml-licence.XXXXXX") || exit 1
trap 'rm -rf "$ctl"' EXIT
mkdir -p "$ctl/bad" "$ctl/good"
for s in bad good; do
  printf '{"name":"%s","isCommercial":false,"licenseType":"free","requires_license":false,"requiredEntitlements":[],"tier":"free"}\n' "$s" > "$ctl/$s/plugin.json"
done
printf 'name: bad\nisCommercial: true\nlicenseType: pro\nrequiredEntitlements:\n  - pro\nrequires_license: true\ntier: pro\n' > "$ctl/bad/plugin.yaml"
printf 'name: good\nisCommercial: false\nlicenseType: free\nrequires_license: false\ntier: free\n' > "$ctl/good/plugin.yaml"
out=$(check_dir "$ctl"); c=$?
if [ "$c" -ne 0 ] && echo "$out" | grep -q '^bad: ' && ! echo "$out" | grep -q '^good: '; then
  echo "control: planted drift reported, matching twin passed"
else echo "FAIL: control (exit $c)"; echo "$out"; exit 1; fi

# Real tree.
rc=0
out=$(check_dir "$root") ; : "$out"
while IFS= read -r line; do
  [ -n "$line" ] || continue
  slug=${line%%:*}
  case "$KNOWN_DRIFT" in *" $slug "*) echo "known drift (tracked): $line" ;; *) echo "FAIL: $line"; rc=1 ;; esac
done <<EOF
$out
EOF
[ "$rc" -eq 0 ] && echo "PASS: no untracked plugin.yaml licence drift under $root"
exit $rc
