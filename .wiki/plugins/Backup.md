# Backup Plugin

Automated PostgreSQL backup and restore system with cron scheduling, multiple storage targets, compression, encryption, retention policies, and artifact management.

| Property | Value |
|----------|-------|
| **Port** | `3013` |
| **Category** | `infrastructure` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run backup init
nself plugin run backup server
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_PLUGIN_PORT` | `3013` | Server port |
| `BACKUP_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `BACKUP_STORAGE_PATH` | `./backups` | Local storage directory |
| `BACKUP_S3_BUCKET` | - | S3 bucket name |
| `BACKUP_S3_REGION` | - | S3 region |
| `BACKUP_S3_ACCESS_KEY` | - | S3 access key |
| `BACKUP_S3_SECRET_KEY` | - | S3 secret key |
| `BACKUP_S3_ENDPOINT` | - | Custom S3 endpoint (R2, MinIO) |
| `BACKUP_ENCRYPTION_KEY` | - | AES-256 encryption key for backup files |
| `BACKUP_DEFAULT_RETENTION_DAYS` | `30` | Days to retain backup artifacts |
| `BACKUP_MAX_CONCURRENT` | `2` | Maximum concurrent backup jobs |
| `BACKUP_PG_DUMP_PATH` | `pg_dump` | Path to `pg_dump` binary |
| `BACKUP_PG_RESTORE_PATH` | `pg_restore` | Path to `pg_restore` binary |
| `BACKUP_API_KEY` | - | API key for authentication |
| `BACKUP_RATE_LIMIT_MAX` | `200` | Max requests per window |
| `BACKUP_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (ms) |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (4 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`, `-h`/`--host`) |
| `status` | Show schedule/artifact/restore counts |
| `create-schedule` | Create backup schedule (`-n`/`--name`, `-c`/`--cron`, `-t`/`--type`, `--include`, `--exclude`, `--compression`, `--retention`, `--max-backups`) |
| `list-schedules` | List backup schedules (`-l`/`--limit`) |
| `run-backup` | Run backup immediately (`-s`/`--schedule`, `-t`/`--type`, `--include`, `--exclude`, `--compression`) |
| `list-backups` | List backup artifacts (`-l`/`--limit`, `-s`/`--status`) |
| `restore` | Restore from backup (`-a`/`--artifact`, `-d`/`--database`, `-t`/`--tables`, `-m`/`--mode`, `-c`/`--conflict`) |
| `download` | Download backup artifact (`-a`/`--artifact`, `-o`/`--output`) |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |
| `GET` | `/live` | Liveness with memory/uptime |
| `GET` | `/v1/status` | Plugin status with schedule/artifact/restore counts |

### Schedules

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/schedules` | Create schedule (body: `name`, `cron_expression`, `backup_type?`, `target_provider?`, `compression?`, `encryption?`, `include_tables?`, `exclude_tables?`, `retention_days?`, `max_backups?`, `metadata?`) |
| `GET` | `/v1/schedules` | List schedules (query: `limit?`, `offset?`) |
| `GET` | `/v1/schedules/:id` | Get schedule details |
| `PUT` | `/v1/schedules/:id` | Update schedule |
| `DELETE` | `/v1/schedules/:id` | Delete schedule |
| `POST` | `/v1/schedules/:id/run` | Trigger immediate backup for schedule |

### Artifacts

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/artifacts` | List backup artifacts (query: `limit?`, `offset?`, `status?`, `schedule_id?`) |
| `GET` | `/v1/artifacts/:id` | Get artifact details |
| `DELETE` | `/v1/artifacts/:id` | Delete artifact (removes file and record) |
| `GET` | `/v1/artifacts/:id/download` | Download backup file (streams file content) |

### Restore

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/restore` | Start restore job (body: `artifact_id`, `target_database_url?`, `tables?`, `restore_mode?`, `conflict_strategy?`) |
| `GET` | `/v1/restore/:id` | Get restore job status |
| `POST` | `/v1/restore/:id/cancel` | Cancel running restore job |

---

## Backup Types

| Type | Description |
|------|-------------|
| `full` | Full database dump (all data and schema) |
| `incremental` | Incremental backup (changes since last full) |
| `schema_only` | Schema definitions only (no data) |
| `data_only` | Data only (no schema definitions) |

---

## Storage Targets

| Provider | Description | Config Required |
|----------|-------------|-----------------|
| `local` | Local filesystem | `BACKUP_STORAGE_PATH` |
| `s3` | Amazon S3 | `BACKUP_S3_BUCKET`, `BACKUP_S3_REGION`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` |
| `r2` | Cloudflare R2 | `BACKUP_S3_BUCKET`, `BACKUP_S3_ENDPOINT`, `BACKUP_S3_ACCESS_KEY`, `BACKUP_S3_SECRET_KEY` |
| `gcs` | Google Cloud Storage | (via S3-compatible endpoint) |

---

## Compression

| Method | Description |
|--------|-------------|
| `none` | No compression |
| `gzip` | Standard gzip compression |
| `zstd` | Zstandard compression (faster, better ratio) |

---

## Restore Modes

| Mode | Description |
|------|-------------|
| `merge` | Merge restored data with existing data |
| `replace` | Drop and recreate tables before restore |
| `dry_run` | Validate backup without modifying the database |

### Conflict Strategy

| Strategy | Description |
|----------|-------------|
| `skip` | Skip rows that conflict with existing data |
| `overwrite` | Overwrite existing rows on conflict |
| `error` | Abort restore on any conflict |

---

## Cron Scheduling

The scheduler uses standard cron expressions (parsed by `cron-parser`):

```
┌──────── minute (0-59)
│ ┌────── hour (0-23)
│ │ ┌──── day of month (1-31)
│ │ │ ┌── month (1-12)
│ │ │ │ ┌ day of week (0-7, 0 and 7 = Sunday)
│ │ │ │ │
* * * * *
```

Examples:

| Expression | Description |
|------------|-------------|
| `0 2 * * *` | Every day at 2:00 AM |
| `0 */6 * * *` | Every 6 hours |
| `0 0 * * 0` | Every Sunday at midnight |
| `0 3 1 * *` | First day of every month at 3:00 AM |
| `30 1 * * 1-5` | Weekdays at 1:30 AM |

The scheduler checks for due schedules every 60 seconds. When a schedule is due, it triggers a backup in the background. The `max_concurrent` setting limits parallel backup execution.

---

## Backup Process

1. **pg_dump execution** -- Runs `pg_dump` with `--format=custom` and appropriate flags (`--schema-only`, `--data-only`, `--table`, `--exclude-table`)
2. **Compression** -- Pipes output through gzip or zstd if configured
3. **Checksum** -- Computes SHA-256 hash of the output file
4. **Row counts** -- Queries table row counts for verification metadata
5. **Artifact record** -- Creates a database record with file path, size, checksum, compression method, and row counts
6. **Retention enforcement** -- Removes artifacts older than `retention_days` or exceeding `max_backups`

---

## Database Schema

### `backup_schedules`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Schedule ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Schedule name |
| `cron_expression` | `VARCHAR(128)` | Cron schedule string |
| `backup_type` | `VARCHAR(50)` | `full`, `incremental`, `schema_only`, `data_only` |
| `target_provider` | `VARCHAR(50)` | `local`, `s3`, `r2`, `gcs` |
| `target_config` | `JSONB` | Provider-specific config (bucket, path, etc.) |
| `compression` | `VARCHAR(20)` | `none`, `gzip`, `zstd` |
| `encryption` | `BOOLEAN` | Whether to encrypt backup files |
| `include_tables` | `TEXT[]` | Tables to include (null = all) |
| `exclude_tables` | `TEXT[]` | Tables to exclude |
| `retention_days` | `INTEGER` | Days to retain artifacts |
| `max_backups` | `INTEGER` | Maximum artifacts to keep |
| `is_active` | `BOOLEAN` | Whether schedule is enabled |
| `last_run_at` | `TIMESTAMPTZ` | Last execution time |
| `next_run_at` | `TIMESTAMPTZ` | Next scheduled execution |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `backup_artifacts`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Artifact ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `schedule_id` | `UUID` (FK) | References `backup_schedules` (nullable for ad-hoc) |
| `backup_type` | `VARCHAR(50)` | Backup type used |
| `status` | `VARCHAR(20)` | `pending`, `running`, `completed`, `failed`, `expired` |
| `file_path` | `TEXT` | Full path to backup file |
| `file_size` | `BIGINT` | File size in bytes |
| `checksum` | `VARCHAR(128)` | SHA-256 checksum |
| `compression` | `VARCHAR(20)` | Compression method used |
| `encrypted` | `BOOLEAN` | Whether file is encrypted |
| `table_count` | `INTEGER` | Number of tables in backup |
| `row_counts` | `JSONB` | Per-table row counts |
| `duration_ms` | `INTEGER` | Backup execution time |
| `error_message` | `TEXT` | Error details (if failed) |
| `expires_at` | `TIMESTAMPTZ` | Retention expiration |
| `started_at` | `TIMESTAMPTZ` | Execution start time |
| `completed_at` | `TIMESTAMPTZ` | Execution completion time |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `backup_restore_jobs`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Restore job ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `artifact_id` | `UUID` (FK) | References `backup_artifacts` |
| `target_database_url` | `TEXT` | Target database (encrypted) |
| `tables` | `TEXT[]` | Specific tables to restore (null = all) |
| `restore_mode` | `VARCHAR(20)` | `merge`, `replace`, `dry_run` |
| `conflict_strategy` | `VARCHAR(20)` | `skip`, `overwrite`, `error` |
| `status` | `VARCHAR(20)` | `pending`, `running`, `completed`, `failed`, `cancelled` |
| `tables_restored` | `INTEGER` | Number of tables restored |
| `rows_restored` | `BIGINT` | Total rows restored |
| `duration_ms` | `INTEGER` | Restore execution time |
| `error_message` | `TEXT` | Error details (if failed) |
| `started_at` | `TIMESTAMPTZ` | Execution start time |
| `completed_at` | `TIMESTAMPTZ` | Execution completion time |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `backup_webhook_events`

Standard webhook event tracking table with `id`, `source_account_id`, `event_type`, `payload` (JSONB), `processed`, `processed_at`, `error`, `created_at`.

---

## Troubleshooting

**"pg_dump not found"** -- Set `BACKUP_PG_DUMP_PATH` to the full path of the `pg_dump` binary. On macOS with Homebrew: `/opt/homebrew/bin/pg_dump`.

**Backup fails with permission error** -- Ensure `BACKUP_STORAGE_PATH` exists and is writable. For S3, verify the access key has `s3:PutObject` and `s3:GetObject` permissions.

**Restore fails midway** -- Use `dry_run` restore mode first to validate. Check the restore job status via `GET /v1/restore/:id` for error details.

**Schedule not triggering** -- Verify the schedule is `is_active: true`. Check `next_run_at` to confirm the next execution time. The scheduler checks every 60 seconds.

**Artifacts accumulating** -- Set `retention_days` and/or `max_backups` on the schedule. Run the cleanup manually or rely on the scheduler to enforce retention after each backup.

**Concurrent backup limit reached** -- Only `BACKUP_MAX_CONCURRENT` backups run simultaneously. Additional triggers queue until a slot opens.
