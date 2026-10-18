#!/usr/bin/env bash
# Purpose: keep the published forgejo wiki page honest after the in-stack runner was removed (P7-TRUST-21, D-0945).
#   Every envVars name in plugin.json must be a row of the wiki Configuration table, no row may name a variable the
#   manifest lacks, and the wiki page and the network-literal lint must not present the removed runner as current.
# Inputs: none. Reads free/forgejo/plugin.json, .github/wiki/plugins/Forgejo.md and scripts/lint-compose-network-literals.sh.
# Outputs: one line per failed check on stderr; exit 0 when all hold, 1 on any failure, 2 when jq is missing.
# Constraints: bash 3.2 compatible; read-only; no network.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
manifest="$repo/free/forgejo/plugin.json"
wiki="$repo/.github/wiki/plugins/Forgejo.md"
lint="$repo/scripts/lint-compose-network-literals.sh"

command -v jq >/dev/null 2>&1 || { echo "wiki-env-docs: jq is required" >&2; exit 2; }
fail=0
bad() { echo "wiki-env-docs: $*" >&2; fail=1; }

# Rows of the "## Configuration" table: the backticked first column.
table="$(awk '/^## Configuration/{on=1;next} /^## /{on=0} on && /^\| `/{gsub(/^\| `|`.*$/,"");print}' "$wiki" | sort)"
want="$(jq -r '.envVars[].name' "$manifest" | sort)"

[ "$table" = "$want" ] || bad "wiki Configuration table does not match plugin.json envVars
--- wiki
$table
--- manifest
$want"

if grep -n 'FORGEJO_RUNNER' "$wiki" >&2; then bad "$wiki still lists FORGEJO_RUNNER_* variables"; fi
if grep -n 'forgejo_runner_net' "$lint" >&2; then bad "$lint still cites forgejo_runner_net, a network the plugin no longer has"; fi
if grep -n -i 'plus a Forgejo Actions runner\|Forgejo Actions runner\.' "$wiki" >&2; then bad "$wiki still describes an in-stack runner"; fi

exit "$fail"
