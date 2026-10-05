#!/usr/bin/env bash
# matrix.sh [--root DIR] [--json] [--fixtures]: print the plugins that publish a public image.
# Rule (EPIC D12): a FREE plugin whose plugin.json has manifest_version 2, service.kind "compose" and a
# Dockerfile in its directory. Unconverted (v1), cli, library and licensed plugins are skipped by rule.
# Output: one slug (plugin.json "name") per line, sorted; --json prints a JSON array instead.
# Exit: 0 ok, 1 an eligible plugin has an unusable name or --fixtures failed, 2 usage.
set -eu
. "$(dirname "$0")/lib.sh"
img_need jq
root=$IMG_ROOT; json=0; fixtures=0
while [ $# -gt 0 ]; do
  case $1 in
    --root) root=$2; shift 2 ;;
    --json) json=1; shift ;;
    --fixtures) fixtures=1; shift ;;
    *) echo "usage: matrix.sh [--root DIR] [--json] [--fixtures]" >&2; exit 2 ;;
  esac
done

list() {
  local d slug bad=0 out=""
  for d in "$1"/free/*/; do
    [ -f "${d}plugin.json" ] && [ -f "${d}Dockerfile" ] || continue
    jq -e '.manifest_version == 2 and .service.kind == "compose" and ((.license // "free") != "licensed")' "${d}plugin.json" >/dev/null 2>&1 || continue
    slug=$(jq -r '.name // empty' "${d}plugin.json")
    if ! img_slug_ok "$slug"; then echo "matrix: ${d}plugin.json: name '$slug' is not a valid image slug" >&2; bad=1; continue; fi
    out="$out$slug
"
  done
  printf '%s' "$out" | sort
  return $bad
}

if [ "$fixtures" = 1 ]; then
  t=$(img_tmp); trap 'rm -rf "$t"' EXIT; rc=0
  mk() { # dir json [nodocker]
    mkdir -p "$t/free/$1"; printf '%s\n' "$2" > "$t/free/$1/plugin.json"; [ "${3:-}" = nodocker ] || printf 'FROM scratch\n' > "$t/free/$1/Dockerfile"
  }
  mk zeta '{"name":"zeta","manifest_version":2,"service":{"kind":"compose"}}'
  mk alpha '{"name":"alpha","manifest_version":2,"license":"free","service":{"kind":"compose"}}'
  mk legacy '{"name":"legacy","version":"1.0.0"}'
  mk clionly '{"name":"clionly","manifest_version":2,"service":{"kind":"cli"}}'
  mk nodocker '{"name":"nodocker","manifest_version":2,"service":{"kind":"compose"}}' nodocker
  mk paid '{"name":"paid","manifest_version":2,"license":"licensed","service":{"kind":"compose"}}'
  got=$(list "$t" | tr '\n' ' ')
  if [ "$got" = "alpha zeta " ]; then echo "pass  matrix selects exactly the eligible plugins ($got)"; else echo "FAIL  matrix selected '$got', want 'alpha zeta '"; rc=1; fi
  rm "$t/free/alpha/Dockerfile"
  got=$(list "$t" | tr '\n' ' ')
  if [ "$got" = "zeta " ]; then echo "pass  a plugin without a Dockerfile is dropped"; else echo "FAIL  dropping a Dockerfile left '$got'"; rc=1; fi
  mk 'bad' '{"name":"Bad Name","manifest_version":2,"service":{"kind":"compose"}}'
  if list "$t" >/dev/null 2>&1; then echo "FAIL  an invalid slug was accepted"; rc=1; else echo "pass  an invalid slug fails"; fi
  rm -rf "$t/free"; mkdir -p "$t/free"
  if [ -z "$(list "$t")" ]; then echo "pass  an empty tree prints nothing"; else echo "FAIL  empty tree printed output"; rc=1; fi
  exit $rc
fi

if [ "$json" = 1 ]; then list "$root" | jq -R . | jq -sc .; else list "$root"; fi
