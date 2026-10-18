#!/usr/bin/env bash
# pins-match-lock.sh CLI_DIR: every image in docker-compose.plugin.yml is pinned and equals a lock entry (P7-PLUG-58, ADR 0030).
# Pinned: <repository>:<tag>@sha256:<64 hex>, never :latest. "Lock" is the cli image lock
# (CLI_DIR/internal/compose/images.lock.json, P7-LIVE-17) plus the plugins image lock (images.json, images.d/monitoring.json),
# which holds the entries the cli lock does not carry (alloy; tempo, which the cli lock still pins to latest).
# A reference matches when repository, version and index digest are all equal to one lock entry.
# Exit: 0 all match, 1 a mismatch (named), 2 usage or a missing input.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
frag=$here/../docker-compose.plugin.yml
[ $# -eq 1 ] || { echo "usage: pins-match-lock.sh CLI_DIR" >&2; exit 2; }
cli_lock=$1/internal/compose/images.lock.json
plugins_lock=$root/images.json
command -v jq >/dev/null 2>&1 || { echo "pins-match-lock: jq is required" >&2; exit 2; }
for f in "$frag" "$cli_lock" "$plugins_lock"; do [ -f "$f" ] || { echo "pins-match-lock: missing $f" >&2; exit 2; }; done

# One "repository:version@digest" per lock entry. Repositories are compared fully qualified (docker.io/...).
locked=$( { jq -r '.images[] | "\(.repository):\(.version)@\(.index_digest)"' "$cli_lock"
            jq -r '.images | to_entries[] | .value.image' "$plugins_lock"; } | sort -u )

refs=$(sed -n 's/^[[:space:]]*image:[[:space:]]*//p' "$frag" | tr -d '"'"'" | sort -u)
[ -n "$refs" ] || { echo "pins-match-lock: no image lines in $frag" >&2; exit 2; }

rc=0; n=0
for ref in $refs; do
  n=$((n + 1))
  case $ref in
    *:latest|*:latest@*) echo "FAIL $ref: :latest is not a pin"; rc=1; continue ;;
  esac
  if ! printf '%s' "$ref" | grep -Eq '^[a-z0-9][^@[:space:]]*:[^@:/[:space:]]+@sha256:[0-9a-f]{64}$'; then
    echo "FAIL $ref: not <repository>:<tag>@sha256:<64 hex>"; rc=1; continue
  fi
  if printf '%s\n' "$locked" | grep -Fxq -- "$ref"; then echo "ok   $ref"; else echo "FAIL $ref: no lock entry with this repository, version and digest"; rc=1; fi
done
# The ticket names ten images.
[ "$n" -eq 10 ] || { echo "FAIL expected 10 distinct images, found $n"; rc=1; }
[ $rc -eq 0 ] && echo "pins-match-lock: $n images pinned and equal to the lock"
exit $rc
