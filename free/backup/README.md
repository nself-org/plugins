# backup

PostgreSQL backup and restore automation with scheduling.

## Overview

The `backup` plugin provides automated PostgreSQL backup and restore capabilities with configurable scheduling, pruning policies, and optional cloud storage upload.

## Installation

```bash
nself plugin install backup
```

## Configuration

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `PORT` | No | Server port (default: 3050) |
| `BACKUP_STORAGE_PATH` | No | Local path for backup files (default: `/tmp/nself-backups`) |
| `BACKUP_PG_DUMP_PATH` | No | Path to `pg_dump` binary |
| `BACKUP_PG_RESTORE_PATH` | No | Path to `pg_restore` binary |
| `PLUGIN_INTERNAL_SECRET` | No | Internal API secret |

## Usage

```bash
# Initialize database schema
nself plugin run backup init

# Start backup automation server
nself plugin run backup server
```

## Database Tables

- `np_backup_jobs` — Backup job records
- `np_backup_schedules` — Scheduled backup configuration

## License

MIT
