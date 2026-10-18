#!/usr/bin/env bash
# lint-boot-migration-depends.sh
#
# Purpose: a plugin that applies its migrations at boot (ADR 0010, boot_migrations.go) exits
#   fatally when Postgres is not reachable, so its compose fragment must wait for a healthy
#   Postgres. A bare `depends_on: [postgres]` only waits for the container to start; a cold
#   start then crash-loops once (D-0962).
# Inputs: optional repo root (default: the repo this script lives in).
# Outputs: one line per offending fragment on stderr; exit 1 on any, else "ok".
# Constraints: bash 3.2 compatible; no docker, no network.
set -u

root="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
fail=0

for boot in "$root"/free/*/boot_migrations.go "$root"/free/*/cmd/boot_migrations.go; do
  [ -f "$boot" ] || continue
  dir="${boot%/boot_migrations.go}"
  dir="${dir%/cmd}"
  frag="$dir/docker-compose.plugin.yml"
  [ -f "$frag" ] || continue
  # postgres must be a map key under depends_on whose next lines carry the health condition.
  if ! grep -A2 -E '^[[:space:]]+postgres:[[:space:]]*$' "$frag" | grep -q 'condition:[[:space:]]*service_healthy'; then
    echo "FAIL: ${frag#"$root"/}: applies boot migrations but does not depend on postgres with condition: service_healthy" >&2
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then exit 1; fi
echo "lint-boot-migration-depends: ok"
