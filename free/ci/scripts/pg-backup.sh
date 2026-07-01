#!/usr/bin/env bash
# pg-backup.sh — nightly ops-postgres dump for sentry boxes (standard §12.5).
#
# Purpose: the observability DB (uptime history, incidents, error events) had NO
#          backup layer at all. This dumps it nightly with rotation. Pair with a
#          weekly provider snapshot (e.g. Hetzner) as the second layer.
# Inputs:  env — POSTGRES_CONTAINER (default ops-postgres), PG_USER (postgres),
#          BACKUP_DIR (/opt/nself-ops/backups), KEEP (7), REPORT_DIR
#          (/opt/nself-ops/errors — failure reports only).
# Outputs: ${BACKUP_DIR}/opsdb-<UTC ts>.dump.gz (pg_dump custom format, gzipped);
#          an MD report to REPORT_DIR ONLY on failure.
# Restore: gunzip -c opsdb-<ts>.dump.gz | docker exec -i ops-postgres \
#            pg_restore -U postgres -d postgres --clean --if-exists
# Cron:    17 3 * * * /opt/nself-ops/pg-backup.sh >> /opt/nself-ops/errors/pg-backup.log 2>&1
set -euo pipefail

POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-ops-postgres}"
PG_USER="${PG_USER:-postgres}"
BACKUP_DIR="${BACKUP_DIR:-/opt/nself-ops/backups}"
KEEP="${KEEP:-7}"
REPORT_DIR="${REPORT_DIR:-/opt/nself-ops/errors}"

ts_iso() { date -u +%FT%TZ; }
ts_file() { date -u +%Y%m%d-%H%M%S; }
hash6() { printf '%s' "$1" | md5sum 2>/dev/null | cut -c1-6 || printf '%s' "$1" | md5 | cut -c1-6; }

emit_failure() {
  local detail="$1"
  local key="pg-backup:$(ts_file)"
  local f="${REPORT_DIR}/$(ts_file)-$(hash6 "$key")-pg-backup-failed.md"
  mkdir -p "$REPORT_DIR"
  {
    echo "---"
    echo "id: $key"
    echo "created_at: $(ts_iso)"
    echo "title: \"pg-backup FAILED on sentry box\""
    echo "severity: critical"
    echo "source: pg-backup"
    echo "---"
    echo
    echo "# pg-backup FAILED"
    echo
    echo '```'
    echo "$detail"
    echo '```'
    echo
    echo "The observability DB has NO fresh backup. Fix before the next incident."
  } > "$f"
  echo "[pg-backup] FAILED — report written: $f" >&2
}

main() {
  mkdir -p "$BACKUP_DIR"
  local out="${BACKUP_DIR}/opsdb-$(ts_file).dump"
  if ! docker exec "$POSTGRES_CONTAINER" pg_dump -U "$PG_USER" --format=custom postgres > "$out" 2>/tmp/pg-backup.err; then
    emit_failure "$(cat /tmp/pg-backup.err 2>/dev/null || echo 'pg_dump failed with no stderr')"
    rm -f "$out"
    exit 1
  fi
  if [ ! -s "$out" ]; then
    emit_failure "pg_dump produced an EMPTY file ($out) — treated as failure."
    rm -f "$out"
    exit 1
  fi
  gzip -f "$out"
  echo "[pg-backup] wrote ${out}.gz ($(du -h "${out}.gz" | cut -f1))"

  # Rotate: keep newest $KEEP dumps.
  ls -1t "${BACKUP_DIR}"/opsdb-*.dump.gz 2>/dev/null | tail -n +$((KEEP + 1)) | while read -r old; do
    rm -f "$old"
    echo "[pg-backup] rotated out $old"
  done
  echo "[pg-backup] done at $(ts_iso) (keeping newest ${KEEP})"
}

main
