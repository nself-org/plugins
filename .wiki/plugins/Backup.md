# Backup Plugin

**Category:** Infrastructure
**Port:** 3013
**Version:** 1.0.0

PostgreSQL backup and restore automation with scheduling, retention policies, multi-provider storage (local, S3, R2, GCS), compression, and encryption.

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Configuration](#configuration)
4. [CLI Commands](#cli-commands)
5. [REST API](#rest-api)
6. [Database Schema](#database-schema)
7. [Backup Types](#backup-types)
8. [Storage Providers](#storage-providers)
9. [Scheduling](#scheduling)
10. [Encryption](#encryption)
11. [Restore Procedures](#restore-procedures)
12. [Examples](#examples)
13. [Troubleshooting](#troubleshooting)

---

## Overview

The Backup plugin provides enterprise-grade PostgreSQL backup automation with:

- **Automated Scheduling** - Cron-based backup schedules
- **Multiple Backup Types** - Full, incremental, schema-only, data-only
- **Multi-Provider Storage** - Local filesystem, AWS S3, Cloudflare R2, Google Cloud Storage
- **Compression** - gzip, zstd, or no compression
- **Encryption** - AES-256 encryption for backups at rest
- **Retention Policies** - Automatic cleanup of old backups
- **Restore Management** - Track and manage restore operations
- **Concurrent Backups** - Run multiple backups in parallel

### Use Cases

- **Disaster Recovery** - Automated daily backups to cloud storage
- **Point-in-Time Recovery** - Incremental backups for granular restore
- **Database Migrations** - Schema-only backups for migration workflows
- **Development Snapshots** - Quick local backups for development testing
- **Compliance** - Long-term retention for regulatory requirements

---

## Quick Start

### 1. Install Dependencies

```bash
cd plugins/backup/ts
npm install
npm run build
```

### 2. Set Required Environment Variables

```bash
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export BACKUP_STORAGE_PATH="/data/backups"
```

### 3. Initialize Database Schema

```bash
npm run cli -- init
```

### 4. Create a Backup Schedule

```bash
npm run cli -- create-schedule \
  --name "daily-full-backup" \
  --cron "0 2 * * *" \
  --type full \
  --provider local \
  --retention 30
```

### 5. Start Server

```bash
npm run dev
```

The server will start on port 3013 and execute backups according to schedules.

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_PLUGIN_PORT` | `3013` | HTTP server port |
| `BACKUP_STORAGE_PATH` | `/tmp/nself-backups` | Local storage directory |
| `BACKUP_S3_ENDPOINT` | - | S3-compatible endpoint URL |
| `BACKUP_S3_BUCKET` | - | S3 bucket name |
| `BACKUP_S3_ACCESS_KEY` | - | S3 access key ID |
| `BACKUP_S3_SECRET_KEY` | - | S3 secret access key |
| `BACKUP_S3_REGION` | `us-east-1` | S3 region |
| `BACKUP_ENCRYPTION_KEY` | - | 32-character encryption key (AES-256) |
| `BACKUP_DEFAULT_RETENTION_DAYS` | `30` | Default backup retention (days) |
| `BACKUP_MAX_CONCURRENT` | `2` | Maximum concurrent backup operations |
| `BACKUP_PG_DUMP_PATH` | `pg_dump` | Path to pg_dump binary |
| `BACKUP_API_KEY` | - | API key for authentication |
| `BACKUP_RATE_LIMIT_MAX` | `100` | Max requests per window |
| `BACKUP_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (milliseconds) |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Storage
BACKUP_STORAGE_PATH=/data/backups

# S3 (optional)
BACKUP_S3_ENDPOINT=https://s3.amazonaws.com
BACKUP_S3_BUCKET=nself-backups
BACKUP_S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
BACKUP_S3_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
BACKUP_S3_REGION=us-east-1

# Encryption
BACKUP_ENCRYPTION_KEY=$(openssl rand -hex 32)

# Retention
BACKUP_DEFAULT_RETENTION_DAYS=90
BACKUP_MAX_CONCURRENT=3

# Server
BACKUP_PLUGIN_PORT=3013
```

---

## CLI Commands

### Initialize Database

```bash
npm run cli -- init
```

Initializes the backup plugin schema (4 tables).

### Create Backup Schedule

```bash
npm run cli -- create-schedule \
  --name "daily-full-backup" \
  --cron "0 2 * * *" \
  --type full \
  --provider local \
  --retention 30 \
  --compression gzip \
  --encryption true
```

**Options:**
- `--name`: Schedule name (required)
- `--cron`: Cron schedule expression (required)
- `--type`: Backup type (full, incremental, schema_only, data_only)
- `--provider`: Storage provider (local, s3, r2, gcs)
- `--retention`: Retention period in days
- `--compression`: Compression type (none, gzip, zstd)
- `--encryption`: Enable encryption (true/false)

### List Backup Schedules

```bash
npm run cli -- list-schedules
```

### Run Backup Immediately

```bash
npm run cli -- run-backup --schedule-id <uuid>

# Or run ad-hoc backup
npm run cli -- run-backup \
  --type full \
  --provider local \
  --compression gzip
```

### List Backup Artifacts

```bash
npm run cli -- list-backups

# Filter by schedule
npm run cli -- list-backups --schedule-id <uuid>

# Filter by date range
npm run cli -- list-backups --start-date 2026-01-01 --end-date 2026-01-31
```

### Restore from Backup

```bash
npm run cli -- restore \
  --artifact-id <uuid> \
  --target-database nself_restored \
  --create-database true
```

**Options:**
- `--artifact-id`: Backup artifact UUID (required)
- `--target-database`: Target database name (default: original database)
- `--create-database`: Create database if not exists (true/false)
- `--no-owner`: Restore without owner (true/false)
- `--clean`: Drop objects before restore (true/false)

### Download Backup

```bash
npm run cli -- download --artifact-id <uuid> --output backup.dump
```

### Show Status

```bash
npm run cli -- status
```

Shows backup statistics, active schedules, recent backups, storage usage.

---

## REST API

### Health & Status

#### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "backup",
  "timestamp": "2026-02-11T12:00:00Z",
  "version": "1.0.0"
}
```

#### GET /ready

Readiness check (includes database connectivity and pg_dump availability).

**Response:**
```json
{
  "ready": true,
  "database": "ok",
  "pgDump": "ok",
  "storage": "ok",
  "timestamp": "2026-02-11T12:00:00Z"
}
```

#### GET /live

Liveness check with detailed stats.

**Response:**
```json
{
  "alive": true,
  "uptime": 3600,
  "memory": {
    "used": 50000000,
    "total": 100000000
  },
  "stats": {
    "totalSchedules": 5,
    "activeSchedules": 4,
    "totalBackups": 150,
    "last24Hours": 4,
    "totalSize": 1073741824,
    "runningBackups": 1,
    "failedBackups24h": 0
  }
}
```

### Backup Schedules

#### POST /v1/schedules

Create a new backup schedule.

**Request:**
```json
{
  "name": "daily-full-backup",
  "scheduleCron": "0 2 * * *",
  "backupType": "full",
  "targetProvider": "s3",
  "targetConfig": {
    "bucket": "nself-backups",
    "prefix": "prod/"
  },
  "compression": "gzip",
  "encryptionEnabled": true,
  "retentionDays": 90,
  "maxBackups": 30,
  "enabled": true
}
```

**Response:**
```json
{
  "id": "schedule_uuid",
  "name": "daily-full-backup",
  "scheduleCron": "0 2 * * *",
  "backupType": "full",
  "targetProvider": "s3",
  "nextRunAt": "2026-02-12T02:00:00Z",
  "createdAt": "2026-02-11T12:00:00Z"
}
```

#### GET /v1/schedules

List all backup schedules.

**Query Parameters:**
- `enabled`: Filter by enabled status (true/false)
- `provider`: Filter by storage provider

**Response:**
```json
{
  "schedules": [
    {
      "id": "uuid",
      "name": "daily-full-backup",
      "scheduleCron": "0 2 * * *",
      "backupType": "full",
      "targetProvider": "s3",
      "enabled": true,
      "lastRunAt": "2026-02-11T02:00:00Z",
      "nextRunAt": "2026-02-12T02:00:00Z",
      "createdAt": "2026-02-10T12:00:00Z"
    }
  ],
  "total": 1
}
```

#### GET /v1/schedules/:id

Get a specific schedule.

#### PATCH /v1/schedules/:id

Update a schedule.

**Request:**
```json
{
  "enabled": false,
  "retentionDays": 60
}
```

#### DELETE /v1/schedules/:id

Delete a schedule.

### Backup Artifacts

#### POST /v1/backups

Run a backup immediately.

**Request:**
```json
{
  "scheduleId": "schedule_uuid",
  "backupType": "full",
  "targetProvider": "s3",
  "compression": "gzip",
  "encryptionEnabled": true
}
```

**Response:**
```json
{
  "id": "np_backup_uuid",
  "status": "running",
  "startedAt": "2026-02-11T12:00:00Z"
}
```

#### GET /v1/backups

List all backup artifacts.

**Query Parameters:**
- `scheduleId`: Filter by schedule
- `status`: Filter by status (running, completed, failed)
- `startDate`: Filter by start date
- `endDate`: Filter by end date
- `limit`: Max results (default: 50)
- `offset`: Offset for pagination

**Response:**
```json
{
  "backups": [
    {
      "id": "uuid",
      "scheduleId": "schedule_uuid",
      "backupType": "full",
      "status": "completed",
      "fileSize": 1073741824,
      "filePath": "s3://nself-backups/prod/np_backup_20260211_020000.dump.gz.enc",
      "compression": "gzip",
      "encrypted": true,
      "startedAt": "2026-02-11T02:00:00Z",
      "completedAt": "2026-02-11T02:15:00Z",
      "duration": 900
    }
  ],
  "total": 150,
  "limit": 50,
  "offset": 0
}
```

#### GET /v1/backups/:id

Get backup artifact details.

#### GET /v1/backups/:id/download

Download a backup artifact.

**Query Parameters:**
- `decrypt`: Decrypt file before download (true/false)

**Response:** Binary stream of backup file

#### DELETE /v1/backups/:id

Delete a backup artifact.

### Restore Operations

#### POST /v1/restore

Initiate a restore operation.

**Request:**
```json
{
  "artifactId": "np_backup_uuid",
  "targetDatabase": "nself_restored",
  "createDatabase": true,
  "noOwner": true,
  "clean": false
}
```

**Response:**
```json
{
  "id": "restore_uuid",
  "artifactId": "np_backup_uuid",
  "status": "running",
  "startedAt": "2026-02-11T12:00:00Z"
}
```

#### GET /v1/restore

List restore operations.

#### GET /v1/restore/:id

Get restore operation status.

**Response:**
```json
{
  "id": "restore_uuid",
  "artifactId": "np_backup_uuid",
  "targetDatabase": "nself_restored",
  "status": "completed",
  "startedAt": "2026-02-11T12:00:00Z",
  "completedAt": "2026-02-11T12:10:00Z",
  "duration": 600,
  "errorMessage": null
}
```

---

## Database Schema

### np_backup_schedules

Backup schedule definitions.

```sql
CREATE TABLE np_backup_schedules (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  schedule_cron VARCHAR(128) NOT NULL,
  np_backup_type VARCHAR(32) DEFAULT 'full' CHECK (np_backup_type IN ('full', 'incremental', 'schema_only', 'data_only')),
  target_provider VARCHAR(32) DEFAULT 'local' CHECK (target_provider IN ('local', 's3', 'r2', 'gcs')),
  target_config JSONB DEFAULT '{}',
  include_tables TEXT[] DEFAULT '{}',
  exclude_tables TEXT[] DEFAULT '{}',
  compression VARCHAR(16) DEFAULT 'gzip' CHECK (compression IN ('none', 'gzip', 'zstd')),
  encryption_enabled BOOLEAN DEFAULT false,
  encryption_key_id VARCHAR(255),
  retention_days INTEGER DEFAULT 30,
  max_backups INTEGER DEFAULT 10,
  enabled BOOLEAN DEFAULT true,
  last_run_at TIMESTAMP WITH TIME ZONE,
  next_run_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_backup_schedules_source_account ON np_backup_schedules(source_account_id);
CREATE INDEX idx_backup_schedules_enabled ON np_backup_schedules(enabled) WHERE enabled = true;
CREATE INDEX idx_backup_schedules_next_run ON np_backup_schedules(next_run_at) WHERE enabled = true;
```

### np_backup_artifacts

Backup file records.

```sql
CREATE TABLE np_backup_artifacts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  schedule_id UUID REFERENCES np_backup_schedules(id) ON DELETE SET NULL,
  np_backup_type VARCHAR(32) NOT NULL,
  status VARCHAR(32) DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed')),
  target_provider VARCHAR(32) NOT NULL,
  np_fileproc_path TEXT NOT NULL,
  np_fileproc_size BIGINT,
  compression VARCHAR(16),
  encrypted BOOLEAN DEFAULT false,
  checksum VARCHAR(64),
  started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  completed_at TIMESTAMP WITH TIME ZONE,
  duration_seconds INTEGER,
  error_message TEXT,
  metadata JSONB DEFAULT '{}',
  expires_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_backup_artifacts_source_account ON np_backup_artifacts(source_account_id);
CREATE INDEX idx_backup_artifacts_schedule ON np_backup_artifacts(schedule_id);
CREATE INDEX idx_backup_artifacts_status ON np_backup_artifacts(status);
CREATE INDEX idx_backup_artifacts_started ON np_backup_artifacts(started_at DESC);
CREATE INDEX idx_backup_artifacts_expires ON np_backup_artifacts(expires_at) WHERE expires_at IS NOT NULL;
```

### np_backup_restore_jobs

Restore operation tracking.

```sql
CREATE TABLE np_backup_restore_jobs (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  artifact_id UUID REFERENCES np_backup_artifacts(id) ON DELETE CASCADE,
  target_database VARCHAR(255) NOT NULL,
  status VARCHAR(32) DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed')),
  options JSONB DEFAULT '{}',
  started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  completed_at TIMESTAMP WITH TIME ZONE,
  duration_seconds INTEGER,
  error_message TEXT,
  restored_by VARCHAR(255)
);

CREATE INDEX idx_backup_restore_jobs_source_account ON np_backup_restore_jobs(source_account_id);
CREATE INDEX idx_backup_restore_jobs_artifact ON np_backup_restore_jobs(artifact_id);
CREATE INDEX idx_backup_restore_jobs_status ON np_backup_restore_jobs(status);
```

### np_backup_webhook_events

Webhook event log.

```sql
CREATE TABLE np_backup_webhook_events (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(255) NOT NULL,
  payload JSONB NOT NULL,
  delivered BOOLEAN DEFAULT false,
  delivered_at TIMESTAMP WITH TIME ZONE,
  delivery_attempts INTEGER DEFAULT 0,
  last_error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_backup_webhook_events_source_account ON np_backup_webhook_events(source_account_id);
CREATE INDEX idx_backup_webhook_events_delivered ON np_backup_webhook_events(delivered);
```

---

## Backup Types

### Full Backup

Complete database dump including schema and data.

```bash
npm run cli -- run-backup --type full
```

**Use Cases:**
- Initial backup
- Weekly/monthly snapshots
- Pre-migration backups

**Size:** Largest
**Restore Time:** Fastest (single file)

### Incremental Backup

Only data changed since last backup (uses Write-Ahead Log).

```bash
npm run cli -- run-backup --type incremental
```

**Use Cases:**
- Frequent backups (hourly/daily)
- Point-in-time recovery
- Minimal storage overhead

**Size:** Smallest
**Restore Time:** Slower (requires base + incremental chain)

### Schema-Only Backup

Database structure without data.

```bash
npm run cli -- run-backup --type schema_only
```

**Use Cases:**
- Development environment setup
- Schema versioning
- Migration workflows

**Size:** Very small
**Restore Time:** Very fast

### Data-Only Backup

Data without schema definitions.

```bash
npm run cli -- run-backup --type data_only
```

**Use Cases:**
- Seeding test environments
- Data migration between versions
- Bulk data exports

**Size:** Medium
**Restore Time:** Fast (if schema exists)

---

## Storage Providers

### Local Filesystem

Store backups on local disk.

**Configuration:**
```bash
BACKUP_STORAGE_PATH=/data/backups
```

**Pros:**
- Fast backup/restore
- No network dependency
- No additional cost

**Cons:**
- Limited by disk space
- No off-site redundancy
- Single point of failure

### AWS S3

Store backups in Amazon S3.

**Configuration:**
```bash
BACKUP_S3_ENDPOINT=https://s3.amazonaws.com
BACKUP_S3_BUCKET=nself-backups
BACKUP_S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
BACKUP_S3_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
BACKUP_S3_REGION=us-east-1
```

**Pros:**
- Unlimited storage
- High durability (99.999999999%)
- Lifecycle policies
- Versioning support

**Cons:**
- Network transfer time
- Cost per GB stored
- API rate limits

### Cloudflare R2

S3-compatible object storage with zero egress fees.

**Configuration:**
```bash
BACKUP_S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
BACKUP_S3_BUCKET=nself-backups
BACKUP_S3_ACCESS_KEY=<r2-access-key>
BACKUP_S3_SECRET_KEY=<r2-secret-key>
BACKUP_S3_REGION=auto
```

**Pros:**
- S3-compatible API
- Zero egress costs
- High performance

**Cons:**
- Limited geographic regions
- Newer service

### Google Cloud Storage

Store backups in GCS.

**Configuration:**
```bash
BACKUP_S3_ENDPOINT=https://storage.googleapis.com
BACKUP_S3_BUCKET=nself-backups
BACKUP_S3_ACCESS_KEY=<gcs-access-key>
BACKUP_S3_SECRET_KEY=<gcs-secret-key>
BACKUP_S3_REGION=us-central1
```

**Pros:**
- Deep GCP integration
- Nearline/Coldline tiers for cost savings
- Strong consistency

**Cons:**
- Cost per GB stored
- Network transfer time

---

## Scheduling

Backups are scheduled using cron expressions.

### Cron Syntax

```
┌───────────── minute (0 - 59)
│ ┌───────────── hour (0 - 23)
│ │ ┌───────────── day of month (1 - 31)
│ │ │ ┌───────────── month (1 - 12)
│ │ │ │ ┌───────────── day of week (0 - 6) (Sunday=0)
│ │ │ │ │
│ │ │ │ │
* * * * *
```

### Common Schedules

**Every hour:**
```
0 * * * *
```

**Every day at 2 AM:**
```
0 2 * * *
```

**Every Sunday at 3 AM:**
```
0 3 * * 0
```

**Every 6 hours:**
```
0 */6 * * *
```

**First day of month at midnight:**
```
0 0 1 * *
```

**Weekdays at 6 PM:**
```
0 18 * * 1-5
```

### Example Schedule Configuration

```json
{
  "name": "hourly-incremental",
  "scheduleCron": "0 * * * *",
  "backupType": "incremental",
  "retentionDays": 7
}
```

```json
{
  "name": "daily-full",
  "scheduleCron": "0 2 * * *",
  "backupType": "full",
  "retentionDays": 30
}
```

```json
{
  "name": "weekly-archive",
  "scheduleCron": "0 3 * * 0",
  "backupType": "full",
  "retentionDays": 365
}
```

---

## Encryption

Backups can be encrypted at rest using AES-256-GCM.

### Setup Encryption Key

```bash
# Generate a 32-character key
export BACKUP_ENCRYPTION_KEY=$(openssl rand -hex 32)
```

**IMPORTANT:** Store this key securely. Encrypted backups cannot be restored without it.

### Enable Encryption

**Via CLI:**
```bash
npm run cli -- create-schedule \
  --name "encrypted-backup" \
  --encryption true
```

**Via API:**
```json
{
  "name": "encrypted-backup",
  "encryptionEnabled": true
}
```

### Encryption Process

1. Backup file is created (e.g., `backup.dump.gz`)
2. AES-256-GCM encryption applied
3. Encrypted file saved with `.enc` extension (e.g., `backup.dump.gz.enc`)
4. Original unencrypted file deleted

### Restore Encrypted Backup

```bash
npm run cli -- restore \
  --artifact-id <uuid> \
  --target-database nself_restored
```

Decryption happens automatically if `BACKUP_ENCRYPTION_KEY` is set.

### Key Rotation

To rotate encryption keys:

1. Generate new key: `BACKUP_ENCRYPTION_KEY_NEW=$(openssl rand -hex 32)`
2. Re-encrypt existing backups (not automated yet)
3. Update environment variable
4. New backups use new key

---

## Restore Procedures

### Full Restore

Restore complete database from full backup.

```bash
npm run cli -- restore \
  --artifact-id <np_backup_uuid> \
  --target-database nself \
  --clean true
```

**Options:**
- `--clean`: Drop all objects before restore
- `--create-database`: Create database if not exists
- `--no-owner`: Skip ownership commands

### Point-in-Time Restore

Restore to specific point in time using incremental backups.

```bash
# 1. Restore base (full) backup
npm run cli -- restore --artifact-id <full_backup_uuid>

# 2. Apply incremental backups in order
npm run cli -- restore --artifact-id <incremental_1_uuid>
npm run cli -- restore --artifact-id <incremental_2_uuid>
```

### Schema-Only Restore

Restore schema to new database.

```bash
npm run cli -- restore \
  --artifact-id <schema_backup_uuid> \
  --target-database nself_dev \
  --create-database true
```

### Selective Table Restore

Restore specific tables only.

```bash
# Use pg_restore with table flag
pg_restore \
  --dbname=nself \
  --table=users \
  --table=orders \
  backup.dump
```

### Cross-Environment Restore

Restore production backup to staging.

```bash
# Download production backup
npm run cli -- download --artifact-id <prod_backup_uuid> --output prod.dump

# Restore to staging database
pg_restore --dbname=nself_staging --clean prod.dump
```

---

## Examples

### Example 1: Daily Backups with 30-Day Retention

```bash
npm run cli -- create-schedule \
  --name "daily-backup" \
  --cron "0 2 * * *" \
  --type full \
  --provider s3 \
  --compression gzip \
  --encryption true \
  --retention 30
```

### Example 2: Hourly Incrementals + Daily Fulls

**Hourly incremental:**
```bash
npm run cli -- create-schedule \
  --name "hourly-incremental" \
  --cron "0 * * * *" \
  --type incremental \
  --provider s3 \
  --retention 7
```

**Daily full:**
```bash
npm run cli -- create-schedule \
  --name "daily-full" \
  --cron "0 2 * * *" \
  --type full \
  --provider s3 \
  --retention 30
```

### Example 3: Query Recent Backups

```sql
SELECT
  id,
  np_backup_type,
  status,
  np_fileproc_size / 1024 / 1024 AS size_mb,
  duration_seconds,
  started_at,
  completed_at
FROM np_backup_artifacts
WHERE source_account_id = 'primary'
  AND status = 'completed'
  AND started_at > NOW() - INTERVAL '7 days'
ORDER BY started_at DESC
LIMIT 10;
```

### Example 4: Calculate Storage Usage

```sql
SELECT
  target_provider,
  COUNT(*) AS np_backup_count,
  SUM(np_fileproc_size) / 1024 / 1024 / 1024 AS total_gb
FROM np_backup_artifacts
WHERE source_account_id = 'primary'
  AND status = 'completed'
GROUP BY target_provider;
```

### Example 5: Failed Backup Analysis

```sql
SELECT
  b.schedule_id,
  s.name AS schedule_name,
  COUNT(*) AS failure_count,
  MAX(b.started_at) AS last_failure,
  b.error_message
FROM np_backup_artifacts b
JOIN np_backup_schedules s ON b.schedule_id = s.id
WHERE b.source_account_id = 'primary'
  AND b.status = 'failed'
  AND b.started_at > NOW() - INTERVAL '30 days'
GROUP BY b.schedule_id, s.name, b.error_message
ORDER BY failure_count DESC;
```

---

## Troubleshooting

### Backup Fails with "pg_dump: command not found"

**Error:**
```
Error: pg_dump: command not found
```

**Solution:**
```bash
# Verify pg_dump is installed
which pg_dump

# If not found, install PostgreSQL client tools
# Ubuntu/Debian:
sudo apt-get install postgresql-client

# macOS:
brew install postgresql

# Or set custom path
export BACKUP_PG_DUMP_PATH=/usr/local/bin/pg_dump
```

### S3 Upload Fails with "Access Denied"

**Error:**
```
Error: Access denied uploading to S3
```

**Solution:**
1. Verify S3 credentials are correct
2. Check IAM policy includes PutObject permission:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:DeleteObject"
      ],
      "Resource": "arn:aws:s3:::nself-backups/*"
    }
  ]
}
```

### Restore Fails with "Database does not exist"

**Error:**
```
Error: database "nself_restored" does not exist
```

**Solution:**
```bash
# Create database first
createdb nself_restored

# Or use --create-database flag
npm run cli -- restore \
  --artifact-id <uuid> \
  --target-database nself_restored \
  --create-database true
```

### Backup Schedule Not Running

**Issue:** Scheduled backups not executing at expected times.

**Solution:**
```bash
# Check schedule status
npm run cli -- list-schedules

# Verify enabled=true
# Verify next_run_at is in the future

# Check server logs
npm run dev

# Verify cron expression is valid
# Test at https://crontab.guru
```

### Disk Space Issues

**Error:**
```
Error: No space left on device
```

**Solution:**
1. Check disk usage:
```bash
df -h
du -sh /data/backups/*
```

2. Clean up old backups:
```bash
# Manually delete old backups
find /data/backups -name "*.dump*" -mtime +90 -delete

# Or adjust retention policies
npm run cli -- update-schedule <uuid> --retention 30
```

3. Use compression:
```bash
# Update schedule to use gzip
npm run cli -- update-schedule <uuid> --compression gzip
```

### Encryption Key Lost

**Issue:** Cannot restore encrypted backups because key is lost.

**Solution:**
Unfortunately, encrypted backups cannot be restored without the original encryption key. This is by design for security.

**Prevention:**
1. Store encryption key in secure vault (1Password, AWS Secrets Manager)
2. Keep encrypted backup of key itself
3. Test restore procedures regularly

### Concurrent Backup Limit Reached

**Error:**
```
Error: Maximum concurrent backups reached (2)
```

**Solution:**
```bash
# Increase concurrent limit
export BACKUP_MAX_CONCURRENT=5

# Or stagger backup schedules to avoid overlap
```

---

## Best Practices

1. **3-2-1 Rule** - Keep 3 copies of data, on 2 different media, with 1 offsite
2. **Test Restores** - Regularly test restore procedures (monthly minimum)
3. **Monitor Failures** - Set up alerts for failed backups
4. **Encrypt Sensitive Data** - Always encrypt backups containing PII
5. **Automate Everything** - Use schedules, don't rely on manual backups
6. **Document Recovery Procedures** - Keep runbooks for disaster recovery
7. **Use Incremental Backups** - Reduce storage costs and backup time
8. **Verify Checksums** - Validate backup integrity after creation
9. **Separate Retention Policies** - Keep daily (30d), weekly (90d), monthly (365d)
10. **Budget for Storage** - Plan for long-term retention costs

---

## Performance Considerations

### Large Database Optimization

For databases > 100GB:

1. **Use parallel dump:**
```bash
pg_dump --format=directory --jobs=4 --file=np_backup_dir
```

2. **Split by table:**
```bash
# Backup large tables separately
pg_dump --table=large_table1 --format=custom --file=large1.dump
```

3. **Use compression:**
```bash
# zstd provides better compression ratio than gzip
BACKUP_COMPRESSION=zstd
```

4. **Increase concurrent backups:**
```bash
BACKUP_MAX_CONCURRENT=5
```

### Network Transfer Optimization

For cloud storage:

1. **Use regional endpoints:**
```bash
# Same region as database
BACKUP_S3_REGION=us-east-1
```

2. **Multipart uploads:** (automatic for files > 100MB)

3. **Direct network path:** Avoid routing through NAT/proxy

### Backup Window Optimization

To minimize impact on production:

1. Schedule during low-traffic periods (2-4 AM)
2. Use incremental backups for frequent snapshots
3. Monitor database load during backups
4. Consider read replicas for backup source

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Backup
- **Issues**: https://github.com/acamarata/nself-plugins/issues
- **License**: Source-Available

---

**Last Updated**: February 11, 2026
**Plugin Version**: 1.0.0
**nself Version**: 0.4.8+
