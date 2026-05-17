# backup

PostgreSQL backup and restore automation with scheduling, retention policies, and optional cloud storage upload.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install backup
nself build
nself start
```

## Overview

The `backup` plugin provides automated PostgreSQL backup and restore. It runs a Go service that manages scheduled backups, stores them locally or uploads to S3-compatible cloud storage, applies retention policies to prune old backups, and exposes an HTTP API for triggering and monitoring backup jobs.

Backups use `pg_dump` in custom format, producing `.dump` files that restore via `pg_restore`. The scheduler supports standard cron expressions. Pruning happens after each successful backup cycle.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string (`postgres://user:pass@host:5432/db`) |
| `PORT` | No | `3050` | HTTP server port |
| `BACKUP_STORAGE_PATH` | No | `/tmp/nself-backups` | Local directory for backup files |
| `BACKUP_PG_DUMP_PATH` | No | `pg_dump` (in PATH) | Full path to `pg_dump` binary |
| `BACKUP_PG_RESTORE_PATH` | No | `pg_restore` (in PATH) | Full path to `pg_restore` binary |
| `PLUGIN_INTERNAL_SECRET` | No | — | Shared secret for `X-Plugin-Secret` header authentication |
| `BACKUP_SCHEDULE` | No | `0 2 * * *` | Cron expression for scheduled backups (default: 2 AM daily) |
| `BACKUP_RETENTION_DAYS` | No | `30` | Number of days to retain backup files before pruning |
| `BACKUP_MAX_FILES` | No | `30` | Maximum backup file count — oldest are pruned once exceeded |
| `BACKUP_COMPRESS` | No | `true` | Gzip-compress backup files after creation |

## HTTP API

All endpoints require the `X-Plugin-Secret` header (set to `PLUGIN_INTERNAL_SECRET`).

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check — returns `{"status":"ok"}` |
| `POST` | `/backup` | Trigger an immediate backup job |
| `GET` | `/backups` | List all backup records with metadata |
| `GET` | `/backups/{id}` | Get details for a specific backup job |
| `POST` | `/restore` | Trigger a restore from a named backup |
| `DELETE` | `/backups/{id}` | Delete a backup record (file is pruned) |
| `GET` | `/schedules` | List configured backup schedules |
| `POST` | `/schedules` | Create or update a backup schedule |

### Trigger a backup

```bash
curl -X POST http://127.0.0.1:3050/backup \
  -H "X-Plugin-Secret: $PLUGIN_INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"label": "pre-migration"}'
```

### Restore from a backup

```bash
curl -X POST http://127.0.0.1:3050/restore \
  -H "X-Plugin-Secret: $PLUGIN_INTERNAL_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"backup_id": "bkp_2026_05_17_020000"}'
```

## Database Tables

| Table | Purpose |
|---|---|
| `np_backup_jobs` | Backup job records — timestamps, file path, size, status, duration |
| `np_backup_schedules` | Scheduled backup configuration — cron expression, retention policy, enabled flag |

## Usage

```bash
# Initialize the plugin database schema
nself plugin run backup init

# Start the backup automation server
nself plugin run backup server

# Trigger an immediate backup
nself run backup backup --label "manual"

# List all backups
nself run backup list

# Restore from a specific backup
nself run backup restore --id bkp_2026_05_17_020000
```

## Port

The plugin binds to `127.0.0.1:3050`. It is never exposed directly — access via Nginx proxy or localhost.

## Multi-App Isolation

Backup jobs are scoped per app via `source_account_id`. Each app in a multi-app deployment has its own job history and schedules.

## Security

- Port 3050 binds to `127.0.0.1` — never exposed to the network directly.
- All API calls require `X-Plugin-Secret` header.
- Backup files are stored locally at `BACKUP_STORAGE_PATH`; ensure the path is not web-accessible.
- No license required — backup is a core infrastructure feature free for all nSelf users.

## See also

- [plugin-jobs](plugin-jobs.md) — background job queue
- [nSelf CLI: nself plugin](cmd-plugin.md) — plugin management commands
