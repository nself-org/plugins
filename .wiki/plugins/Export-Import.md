# Export-Import Plugin

Bulk data export/import with multiple format support, migration tools, backup/restore, and cross-platform data transfer

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Export-Import plugin provides comprehensive data portability solutions for the nself platform. It enables bulk data exports, imports from various sources, cross-platform migrations, automated backups, and point-in-time restores with support for multiple formats and storage backends.

### Key Features

- **Data Export** - Export data to JSON, CSV, SQL, XML, and custom formats
- **Data Import** - Import from multiple formats with validation and conflict resolution
- **Platform Migration** - Migrate data from Slack, Discord, Teams, Mattermost, and more
- **Automated Backups** - Schedule full and incremental backups with retention policies
- **Point-in-Time Restore** - Restore data from any backup snapshot
- **Transform Templates** - Create reusable data transformation pipelines
- **Audit Trail** - Complete audit log of all data transfer operations
- **Multi-Storage Support** - Local, S3, GCS, Azure, and MinIO storage backends
- **Compression & Encryption** - Configurable compression (gzip, zstd) and encryption
- **Multi-Account Support** - `source_account_id` isolation for multi-tenant deployments

### Supported Platforms

| Platform | Import | Export | Migration | Status |
|----------|--------|--------|-----------|--------|
| JSON | ✓ | ✓ | N/A | Supported |
| CSV | ✓ | ✓ | N/A | Supported |
| SQL | ✓ | ✓ | N/A | Supported |
| XML | ✓ | ✓ | N/A | Supported |
| Slack | ✓ | N/A | ✓ | Supported |
| Discord | ✓ | N/A | ✓ | Supported |
| Microsoft Teams | ✓ | N/A | ✓ | Supported |
| Mattermost | ✓ | N/A | ✓ | Supported |
| Rocket.Chat | ✓ | N/A | ✓ | Supported |
| Telegram | ✓ | N/A | ✓ | Supported |

---

## Quick Start

```bash
# Install the plugin
nself plugin install export-import

# Set required environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"
export EI_PLUGIN_PORT=3717

# Initialize database schema
nself plugin export-import init

# Start the server
nself plugin export-import server --port 3717

# Check status
nself plugin export-import status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `EI_PLUGIN_PORT` | No | `3717` | HTTP server port |
| `EI_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `EXPORT_STORAGE_PATH` | No | `/var/nself/exports` | Path for export files |
| `EXPORT_MAX_FILE_SIZE_GB` | No | `10` | Maximum export file size (GB) |
| `EXPORT_RETENTION_DAYS` | No | `30` | Export file retention days |
| `IMPORT_MAX_FILE_SIZE_GB` | No | `10` | Maximum import file size (GB) |
| `IMPORT_TEMP_PATH` | No | `/tmp/nself-imports` | Temporary import directory |
| `BACKUP_STORAGE_BACKEND` | No | `local` | Backup storage backend (local, s3, gcs, azure, minio) |
| `BACKUP_RETENTION_DAYS` | No | `90` | Backup retention days |
| `MIGRATION_BATCH_SIZE` | No | `100` | Migration batch size |
| `MIGRATION_RATE_LIMIT_MS` | No | `100` | Migration rate limit (milliseconds) |
| `EXPORT_IMPORT_QUEUE_CONCURRENCY` | No | `5` | Queue concurrency limit |
| `EXPORT_IMPORT_QUEUE_TIMEOUT_MINUTES` | No | `120` | Queue timeout (minutes) |
| `COMPRESSION_LEVEL` | No | `6` | Compression level (0-9) |
| `COMPRESSION_ALGORITHM` | No | `gzip` | Compression algorithm (gzip, zstd) |
| `EI_API_KEY` | No | - | API key for authentication |
| `EI_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `EI_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=nself
POSTGRES_PASSWORD=secure_password
POSTGRES_SSL=false

# Server
EI_PLUGIN_PORT=3717
EI_PLUGIN_HOST=0.0.0.0

# Export Configuration
EXPORT_STORAGE_PATH=/var/nself/exports
EXPORT_MAX_FILE_SIZE_GB=10
EXPORT_RETENTION_DAYS=30

# Import Configuration
IMPORT_MAX_FILE_SIZE_GB=10
IMPORT_TEMP_PATH=/tmp/nself-imports

# Backup Configuration
BACKUP_STORAGE_BACKEND=s3
BACKUP_RETENTION_DAYS=90

# Migration Configuration
MIGRATION_BATCH_SIZE=100
MIGRATION_RATE_LIMIT_MS=100

# Queue Configuration
EXPORT_IMPORT_QUEUE_CONCURRENCY=5
EXPORT_IMPORT_QUEUE_TIMEOUT_MINUTES=120

# Compression
COMPRESSION_LEVEL=6
COMPRESSION_ALGORITHM=gzip

# Security
EI_API_KEY=your_api_key_here
EI_RATE_LIMIT_MAX=100
EI_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin export-import init

# Start the server
nself plugin export-import server
nself plugin export-import server --port 3717 --host 0.0.0.0

# Check status and statistics
nself plugin export-import status
```

### Export Commands

```bash
# List export jobs
nself plugin export-import export list
nself plugin export-import export list --status completed
nself plugin export-import export list --limit 50

# Show export job details
nself plugin export-import export info <job-id>

# Cancel running export
nself plugin export-import export cancel <job-id>

# Delete export job
nself plugin export-import export delete <job-id>
```

### Import Commands

```bash
# List import jobs
nself plugin export-import import list
nself plugin export-import import list --status running
nself plugin export-import import list --limit 50

# Show import job details
nself plugin export-import import info <job-id>

# Cancel running import
nself plugin export-import import cancel <job-id>
```

### Migration Commands

```bash
# List migration jobs
nself plugin export-import migrate list
nself plugin export-import migrate list --limit 50

# Show supported platforms
nself plugin export-import migrate platforms

# Show migration job details
nself plugin export-import migrate info <job-id>
```

### Backup Commands

```bash
# List backup snapshots
nself plugin export-import backup list
nself plugin export-import backup list --limit 50

# Show backup snapshot details
nself plugin export-import backup info <snapshot-id>

# Verify backup integrity
nself plugin export-import backup verify <snapshot-id>

# Delete backup snapshot
nself plugin export-import backup delete <snapshot-id>

# Cleanup expired backups
nself plugin export-import backup cleanup
```

### Restore Commands

```bash
# List restore jobs
nself plugin export-import restore list
nself plugin export-import restore list --limit 50

# Show restore job details
nself plugin export-import restore info <job-id>
```

### Audit Commands

```bash
# View audit log
nself plugin export-import audit list
nself plugin export-import audit list --type export
nself plugin export-import audit list --limit 100
```

### Transform Commands

```bash
# List transformation templates
nself plugin export-import transform list
nself plugin export-import transform list --limit 50

# Delete transformation template
nself plugin export-import transform delete <template-id>
```

---

## REST API

### Base URL

```
http://localhost:3717
```

### Health & Status

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "export-import",
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### GET /ready
Readiness check with database connectivity.

**Response:**
```json
{
  "ready": true,
  "plugin": "export-import",
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### GET /live
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "export-import",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 52428800,
    "heapTotal": 20971520,
    "heapUsed": 15728640,
    "external": 1048576
  },
  "stats": {
    "export_jobs": { "total": 45, "pending": 2, "running": 1, "completed": 40, "failed": 2 },
    "import_jobs": { "total": 30, "pending": 1, "running": 0, "completed": 28, "failed": 1 },
    "migration_jobs": { "total": 5, "pending": 0, "running": 0, "completed": 5, "failed": 0 },
    "backup_snapshots": { "total": 120, "verified": 115, "expired": 10 },
    "restore_jobs": { "total": 3, "pending": 0, "running": 0, "completed": 3, "failed": 0 },
    "transform_templates": { "total": 15, "public_count": 8 },
    "audit_entries": 500
  },
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### GET /v1/status
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "export-import",
  "version": "1.0.0",
  "status": "running",
  "stats": { /* same as /live */ },
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

### Export Endpoints

#### POST /api/export/create
Create a new export job.

**Request Body:**
```json
{
  "user_id": "user123",
  "name": "User Data Export",
  "description": "Export all user data for backup",
  "export_type": "full",
  "format": "json",
  "scope": {
    "tables": ["users", "messages", "channels"],
    "include_files": true
  },
  "filters": {
    "date_range": {
      "start": "2026-01-01",
      "end": "2026-02-01"
    }
  },
  "compression": "gzip",
  "encryption_enabled": false
}
```

**Response:**
```json
{
  "id": "uuid",
  "source_account_id": "primary",
  "user_id": "user123",
  "name": "User Data Export",
  "description": "Export all user data for backup",
  "export_type": "full",
  "format": "json",
  "scope": { /* ... */ },
  "filters": { /* ... */ },
  "compression": "gzip",
  "encryption_enabled": false,
  "status": "pending",
  "progress_percentage": 0,
  "total_records": 0,
  "exported_records": 0,
  "output_path": null,
  "output_size_bytes": null,
  "checksum": null,
  "error_message": null,
  "metadata": {},
  "started_at": null,
  "completed_at": null,
  "expires_at": "2026-03-13T12:00:00.000Z",
  "created_at": "2026-02-11T12:00:00.000Z",
  "updated_at": "2026-02-11T12:00:00.000Z"
}
```

#### GET /api/export/jobs
List export jobs.

**Query Parameters:**
- `limit` - Number of records (default: 100)
- `offset` - Offset for pagination (default: 0)
- `status` - Filter by status (pending, running, completed, failed, cancelled)

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "name": "User Data Export",
      "status": "completed",
      "progress_percentage": 100,
      "exported_records": 50000,
      "total_records": 50000,
      "created_at": "2026-02-11T12:00:00.000Z"
    }
  ],
  "limit": 100,
  "offset": 0
}
```

#### GET /api/export/jobs/:id
Get export job details.

#### GET /api/export/jobs/:id/download
Get download information for completed export.

**Response:**
```json
{
  "download_url": "/var/nself/exports/export-uuid.json.gz",
  "checksum": "sha256:abc123...",
  "size_bytes": 5242880
}
```

#### DELETE /api/export/jobs/:id
Cancel or delete an export job.

**Response:**
```json
{
  "success": true,
  "action": "cancelled"
}
```

#### POST /api/export/estimate
Estimate export size.

**Request Body:**
```json
{
  "scope": {
    "tables": ["users", "messages"],
    "filters": { /* ... */ }
  }
}
```

**Response:**
```json
{
  "estimated_size_bytes": 5242880,
  "estimated_records": 50000,
  "scope": { /* ... */ }
}
```

#### GET /api/export/templates
List export templates.

### Import Endpoints

#### POST /api/import/create
Create a new import job.

**Request Body:**
```json
{
  "user_id": "user123",
  "name": "Slack Import",
  "description": "Import Slack workspace data",
  "import_type": "platform_migration",
  "source_format": "slack",
  "source_path": "/tmp/slack-export.zip",
  "source_size_bytes": 10485760,
  "mapping_rules": {
    "channels": "channels",
    "messages": "messages",
    "users": "users"
  },
  "conflict_resolution": "skip",
  "validation_mode": "strict",
  "dry_run": false
}
```

**Response:**
```json
{
  "id": "uuid",
  "source_account_id": "primary",
  "user_id": "user123",
  "name": "Slack Import",
  "status": "pending",
  "progress_percentage": 0,
  "total_records": 0,
  "imported_records": 0,
  "skipped_records": 0,
  "failed_records": 0,
  "created_at": "2026-02-11T12:00:00.000Z"
}
```

#### POST /api/import/validate
Validate import file.

**Response:**
```json
{
  "valid": true,
  "errors": [],
  "warnings": []
}
```

#### POST /api/import/upload
Upload import file.

**Response:**
```json
{
  "path": "/tmp/uploaded-file",
  "size_bytes": 10485760
}
```

#### GET /api/import/jobs
List import jobs.

**Query Parameters:**
- `limit` - Number of records
- `offset` - Pagination offset
- `status` - Filter by status

#### GET /api/import/jobs/:id
Get import job details.

#### POST /api/import/jobs/:id/start
Start a pending import job.

#### DELETE /api/import/jobs/:id
Cancel or delete an import job.

#### GET /api/import/mappings/:format
Get default field mappings for a format.

**Response:**
```json
{
  "format": "slack",
  "mappings": {
    "channels": "channels",
    "messages": "messages",
    "users": "users",
    "files": "files"
  }
}
```

### Migration Endpoints

#### POST /api/migrate/analyze
Analyze source platform data.

**Response:**
```json
{
  "platform_status": "connected",
  "available_data": {
    "channels": 150,
    "messages": 50000,
    "users": 500,
    "files": 2000
  },
  "estimated_duration_minutes": 60
}
```

#### POST /api/migrate/create
Create a migration job.

**Request Body:**
```json
{
  "user_id": "user123",
  "name": "Slack to nself Migration",
  "source_platform": "slack",
  "source_credentials": {
    "api_token": "xoxb-...",
    "team_id": "T12345"
  },
  "destination_scope": {
    "workspace_id": "workspace456"
  },
  "migration_plan": {
    "migrate_channels": true,
    "migrate_messages": true,
    "migrate_files": true,
    "migrate_users": true
  }
}
```

#### GET /api/migrate/jobs
List migration jobs.

#### GET /api/migrate/jobs/:id
Get migration job details.

#### POST /api/migrate/jobs/:id/start
Start a migration job.

#### DELETE /api/migrate/jobs/:id
Cancel or delete a migration job.

#### GET /api/migrate/platforms
List supported migration platforms.

**Response:**
```json
{
  "platforms": [
    { "id": "slack", "name": "Slack", "status": "supported" },
    { "id": "discord", "name": "Discord", "status": "supported" },
    { "id": "teams", "name": "Microsoft Teams", "status": "supported" }
  ]
}
```

### Backup Endpoints

#### POST /api/backup/create
Create a backup snapshot.

**Request Body:**
```json
{
  "name": "Daily Backup 2026-02-11",
  "description": "Automated daily backup",
  "backup_type": "full",
  "scope": {
    "all_tables": true
  },
  "compression": "gzip",
  "encryption_enabled": true,
  "storage_backend": "s3",
  "retention_days": 90
}
```

**Response:**
```json
{
  "id": "uuid",
  "source_account_id": "primary",
  "name": "Daily Backup 2026-02-11",
  "backup_type": "full",
  "storage_backend": "s3",
  "storage_path": "/backups/1707652800000-full",
  "retention_days": 90,
  "expires_at": "2026-05-12T12:00:00.000Z",
  "created_at": "2026-02-11T12:00:00.000Z"
}
```

#### GET /api/backup/snapshots
List backup snapshots.

#### GET /api/backup/snapshots/:id
Get backup snapshot details.

#### POST /api/backup/snapshots/:id/verify
Verify backup integrity.

#### DELETE /api/backup/snapshots/:id
Delete a backup snapshot.

#### GET /api/backup/schedule
Get backup schedule configuration.

**Response:**
```json
{
  "enabled": true,
  "frequency": "daily",
  "time": "02:00",
  "backup_type": "incremental",
  "retention_days": 90,
  "storage_backend": "s3"
}
```

#### PUT /api/backup/schedule
Update backup schedule.

**Request Body:**
```json
{
  "enabled": true,
  "frequency": "daily",
  "time": "02:00",
  "backup_type": "incremental",
  "retention_days": 90,
  "storage_backend": "s3"
}
```

### Restore Endpoints

#### POST /api/restore/create
Create a restore job.

**Request Body:**
```json
{
  "user_id": "user123",
  "snapshot_id": "snapshot-uuid",
  "restore_type": "full",
  "target_scope": {
    "all_tables": true
  },
  "restore_point": null,
  "conflict_resolution": "replace"
}
```

#### POST /api/restore/preview
Preview restore operation.

**Request Body:**
```json
{
  "snapshot_id": "snapshot-uuid",
  "target_scope": {
    "tables": ["users", "messages"]
  }
}
```

**Response:**
```json
{
  "snapshot_id": "snapshot-uuid",
  "snapshot_name": "Daily Backup 2026-02-11",
  "target_scope": { /* ... */ },
  "estimated_items": 50000,
  "conflicts": []
}
```

#### GET /api/restore/jobs
List restore jobs.

#### GET /api/restore/jobs/:id
Get restore job details.

#### POST /api/restore/jobs/:id/start
Start a restore job.

#### DELETE /api/restore/jobs/:id
Cancel or delete a restore job.

### Transform Template Endpoints

#### GET /api/transform/templates
List transformation templates.

**Query Parameters:**
- `limit` - Number of records
- `offset` - Pagination offset
- `source_format` - Filter by source format
- `target_format` - Filter by target format

#### POST /api/transform/templates
Create a transformation template.

**Request Body:**
```json
{
  "user_id": "user123",
  "name": "Slack to nself Transform",
  "description": "Transform Slack export to nself format",
  "source_format": "slack",
  "target_format": "nself",
  "transformations": {
    "field_mappings": {
      "user": "user_id",
      "channel": "channel_id"
    },
    "value_transformations": []
  },
  "is_public": false
}
```

#### GET /api/transform/templates/:id
Get transformation template details.

#### PUT /api/transform/templates/:id
Update transformation template.

#### DELETE /api/transform/templates/:id
Delete transformation template.

#### POST /api/transform/apply
Apply transformation template to data.

**Request Body:**
```json
{
  "template_id": "template-uuid",
  "data": { /* data to transform */ }
}
```

### Audit Endpoints

#### GET /api/audit/transfers
List audit entries.

**Query Parameters:**
- `limit` - Number of records
- `offset` - Pagination offset
- `job_type` - Filter by job type (export, import, migration, backup, restore)
- `user_id` - Filter by user ID

#### GET /api/audit/transfers/:id
Get audit entry details.

#### GET /api/audit/export
Export audit log.

**Query Parameters:**
- `limit` - Number of records
- `job_type` - Filter by job type

---

## Webhook Events

The Export-Import plugin does not emit webhook events. All operations are tracked in the audit log accessible via the `/api/audit/transfers` endpoint.

---

## Database Schema

### ei_export_jobs

Stores export job records.

```sql
CREATE TABLE ei_export_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  export_type VARCHAR(50) NOT NULL,
  format VARCHAR(50) NOT NULL,
  scope JSONB NOT NULL DEFAULT '{}',
  filters JSONB DEFAULT '{}',
  compression VARCHAR(20),
  encryption_enabled BOOLEAN DEFAULT false,
  encryption_key_id UUID,
  status VARCHAR(50) DEFAULT 'pending',
  progress_percentage INTEGER DEFAULT 0,
  total_records INTEGER DEFAULT 0,
  exported_records INTEGER DEFAULT 0,
  output_path TEXT,
  output_size_bytes BIGINT,
  checksum VARCHAR(64),
  error_message TEXT,
  metadata JSONB DEFAULT '{}',
  started_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  expires_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ei_export_jobs_source_account ON ei_export_jobs(source_account_id);
CREATE INDEX idx_ei_export_jobs_user ON ei_export_jobs(user_id);
CREATE INDEX idx_ei_export_jobs_status ON ei_export_jobs(status);
CREATE INDEX idx_ei_export_jobs_created ON ei_export_jobs(created_at DESC);
CREATE INDEX idx_ei_export_jobs_expires ON ei_export_jobs(expires_at) WHERE expires_at IS NOT NULL;
```

**Columns:**
| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | gen_random_uuid() | Primary key |
| `source_account_id` | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| `user_id` | VARCHAR(255) | No | - | User who created the export |
| `name` | VARCHAR(255) | No | - | Export job name |
| `description` | TEXT | Yes | - | Export description |
| `export_type` | VARCHAR(50) | No | - | Export type (full, incremental, selective) |
| `format` | VARCHAR(50) | No | - | Export format (json, csv, sql, xml) |
| `scope` | JSONB | No | {} | Export scope (tables, filters) |
| `filters` | JSONB | Yes | {} | Additional filters |
| `compression` | VARCHAR(20) | Yes | - | Compression algorithm |
| `encryption_enabled` | BOOLEAN | No | false | Encryption flag |
| `encryption_key_id` | UUID | Yes | - | Encryption key ID |
| `status` | VARCHAR(50) | No | 'pending' | Job status |
| `progress_percentage` | INTEGER | No | 0 | Progress percentage |
| `total_records` | INTEGER | No | 0 | Total records to export |
| `exported_records` | INTEGER | No | 0 | Records exported |
| `output_path` | TEXT | Yes | - | Export file path |
| `output_size_bytes` | BIGINT | Yes | - | Output file size |
| `checksum` | VARCHAR(64) | Yes | - | File checksum |
| `error_message` | TEXT | Yes | - | Error message if failed |
| `metadata` | JSONB | No | {} | Additional metadata |
| `started_at` | TIMESTAMPTZ | Yes | - | Job start time |
| `completed_at` | TIMESTAMPTZ | Yes | - | Job completion time |
| `expires_at` | TIMESTAMPTZ | Yes | - | File expiration time |
| `created_at` | TIMESTAMPTZ | No | NOW() | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | No | NOW() | Last update timestamp |

### ei_import_jobs

Stores import job records.

```sql
CREATE TABLE ei_import_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  import_type VARCHAR(50) NOT NULL,
  source_format VARCHAR(50) NOT NULL,
  source_path TEXT NOT NULL,
  source_size_bytes BIGINT,
  mapping_rules JSONB DEFAULT '{}',
  conflict_resolution VARCHAR(50) DEFAULT 'skip',
  validation_mode VARCHAR(50) DEFAULT 'strict',
  dry_run BOOLEAN DEFAULT false,
  status VARCHAR(50) DEFAULT 'pending',
  progress_percentage INTEGER DEFAULT 0,
  total_records INTEGER DEFAULT 0,
  imported_records INTEGER DEFAULT 0,
  skipped_records INTEGER DEFAULT 0,
  failed_records INTEGER DEFAULT 0,
  validation_errors JSONB DEFAULT '[]',
  error_message TEXT,
  metadata JSONB DEFAULT '{}',
  started_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ei_import_jobs_source_account ON ei_import_jobs(source_account_id);
CREATE INDEX idx_ei_import_jobs_user ON ei_import_jobs(user_id);
CREATE INDEX idx_ei_import_jobs_status ON ei_import_jobs(status);
CREATE INDEX idx_ei_import_jobs_created ON ei_import_jobs(created_at DESC);
```

### ei_migration_jobs

Stores cross-platform migration jobs.

```sql
CREATE TABLE ei_migration_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  name VARCHAR(255) NOT NULL,
  source_platform VARCHAR(50) NOT NULL,
  source_credentials JSONB NOT NULL DEFAULT '{}',
  destination_scope JSONB NOT NULL DEFAULT '{}',
  migration_plan JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(50) DEFAULT 'pending',
  phase VARCHAR(50),
  progress_percentage INTEGER DEFAULT 0,
  estimated_duration_minutes INTEGER,
  total_items INTEGER DEFAULT 0,
  migrated_items INTEGER DEFAULT 0,
  failed_items INTEGER DEFAULT 0,
  warnings JSONB DEFAULT '[]',
  error_message TEXT,
  metadata JSONB DEFAULT '{}',
  started_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ei_migration_jobs_source_account ON ei_migration_jobs(source_account_id);
CREATE INDEX idx_ei_migration_jobs_user ON ei_migration_jobs(user_id);
CREATE INDEX idx_ei_migration_jobs_platform ON ei_migration_jobs(source_platform);
CREATE INDEX idx_ei_migration_jobs_status ON ei_migration_jobs(status);
CREATE INDEX idx_ei_migration_jobs_created ON ei_migration_jobs(created_at DESC);
```

### ei_backup_snapshots

Stores backup snapshots.

```sql
CREATE TABLE ei_backup_snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  backup_type VARCHAR(50) NOT NULL,
  base_snapshot_id UUID REFERENCES ei_backup_snapshots(id) ON DELETE SET NULL,
  scope JSONB NOT NULL DEFAULT '{}',
  compression VARCHAR(20) NOT NULL DEFAULT 'gzip',
  encryption_enabled BOOLEAN DEFAULT false,
  encryption_key_id UUID,
  storage_backend VARCHAR(50) NOT NULL DEFAULT 'local',
  storage_path TEXT NOT NULL,
  total_size_bytes BIGINT,
  compressed_size_bytes BIGINT,
  checksum VARCHAR(64),
  verification_status VARCHAR(50),
  verified_at TIMESTAMP WITH TIME ZONE,
  retention_days INTEGER DEFAULT 30,
  expires_at TIMESTAMP WITH TIME ZONE,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ei_snapshots_source_account ON ei_backup_snapshots(source_account_id);
CREATE INDEX idx_ei_snapshots_type ON ei_backup_snapshots(backup_type);
CREATE INDEX idx_ei_snapshots_created ON ei_backup_snapshots(created_at DESC);
CREATE INDEX idx_ei_snapshots_expires ON ei_backup_snapshots(expires_at) WHERE expires_at IS NOT NULL;
```

### ei_restore_jobs

Stores restore operations.

```sql
CREATE TABLE ei_restore_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  snapshot_id UUID NOT NULL REFERENCES ei_backup_snapshots(id) ON DELETE CASCADE,
  restore_type VARCHAR(50) NOT NULL,
  target_scope JSONB NOT NULL DEFAULT '{}',
  restore_point TIMESTAMP WITH TIME ZONE,
  conflict_resolution VARCHAR(50) DEFAULT 'skip',
  status VARCHAR(50) DEFAULT 'pending',
  progress_percentage INTEGER DEFAULT 0,
  total_items INTEGER DEFAULT 0,
  restored_items INTEGER DEFAULT 0,
  failed_items INTEGER DEFAULT 0,
  error_message TEXT,
  metadata JSONB DEFAULT '{}',
  started_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ei_restore_jobs_source_account ON ei_restore_jobs(source_account_id);
CREATE INDEX idx_ei_restore_jobs_user ON ei_restore_jobs(user_id);
CREATE INDEX idx_ei_restore_jobs_snapshot ON ei_restore_jobs(snapshot_id);
CREATE INDEX idx_ei_restore_jobs_status ON ei_restore_jobs(status);
```

### ei_transform_templates

Stores data transformation templates.

```sql
CREATE TABLE ei_transform_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255),
  name VARCHAR(255) NOT NULL,
  description TEXT,
  source_format VARCHAR(50) NOT NULL,
  target_format VARCHAR(50) NOT NULL,
  transformations JSONB NOT NULL DEFAULT '{}',
  is_public BOOLEAN DEFAULT false,
  usage_count INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ei_templates_source_account ON ei_transform_templates(source_account_id);
CREATE INDEX idx_ei_templates_formats ON ei_transform_templates(source_format, target_format);
CREATE INDEX idx_ei_templates_public ON ei_transform_templates(is_public) WHERE is_public = true;
```

### ei_data_transfer_audit

Audit log for all data transfer operations.

```sql
CREATE TABLE ei_data_transfer_audit (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  job_type VARCHAR(50) NOT NULL,
  job_id UUID NOT NULL,
  user_id VARCHAR(255) NOT NULL,
  action VARCHAR(100) NOT NULL,
  records_affected INTEGER,
  data_size_bytes BIGINT,
  ip_address INET,
  user_agent TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_ei_audit_source_account ON ei_data_transfer_audit(source_account_id);
CREATE INDEX idx_ei_audit_job ON ei_data_transfer_audit(job_type, job_id);
CREATE INDEX idx_ei_audit_user ON ei_data_transfer_audit(user_id);
CREATE INDEX idx_ei_audit_created ON ei_data_transfer_audit(created_at DESC);
```

### ei_backup_schedule

Backup schedule configuration.

```sql
CREATE TABLE ei_backup_schedule (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  enabled BOOLEAN DEFAULT false,
  frequency VARCHAR(20) NOT NULL DEFAULT 'daily',
  time VARCHAR(10) NOT NULL DEFAULT '02:00',
  backup_type VARCHAR(50) NOT NULL DEFAULT 'incremental',
  retention_days INTEGER DEFAULT 90,
  storage_backend VARCHAR(50) NOT NULL DEFAULT 'local',
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id)
);

CREATE INDEX idx_ei_backup_schedule_source_account ON ei_backup_schedule(source_account_id);
```

---

## Examples

### Example 1: Export User Data to JSON

```bash
# Via API
curl -X POST http://localhost:3717/api/export/create \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "admin",
    "name": "User Export 2026-02",
    "export_type": "selective",
    "format": "json",
    "scope": {
      "tables": ["users", "user_profiles"],
      "include_related": true
    },
    "compression": "gzip"
  }'

# Check export status
curl http://localhost:3717/api/export/jobs/<job-id>

# Download when complete
curl http://localhost:3717/api/export/jobs/<job-id>/download
```

### Example 2: Import Data from CSV

```bash
# Upload file first
curl -X POST http://localhost:3717/api/import/upload \
  -F "file=@users.csv"

# Create import job
curl -X POST http://localhost:3717/api/import/create \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "admin",
    "name": "User Import from CSV",
    "import_type": "bulk_load",
    "source_format": "csv",
    "source_path": "/tmp/users.csv",
    "mapping_rules": {
      "email": "email",
      "name": "full_name",
      "phone": "phone_number"
    },
    "conflict_resolution": "replace",
    "validation_mode": "strict"
  }'

# Start import
curl -X POST http://localhost:3717/api/import/jobs/<job-id>/start
```

### Example 3: Migrate from Slack

```bash
# Create migration job
curl -X POST http://localhost:3717/api/migrate/create \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "admin",
    "name": "Slack to nself Migration",
    "source_platform": "slack",
    "source_credentials": {
      "api_token": "xoxb-your-slack-token",
      "team_id": "T12345678"
    },
    "destination_scope": {
      "workspace_id": "workspace123"
    },
    "migration_plan": {
      "migrate_channels": true,
      "migrate_messages": true,
      "migrate_files": true,
      "migrate_users": true,
      "date_range": {
        "start": "2025-01-01",
        "end": "2026-02-01"
      }
    }
  }'

# Start migration
curl -X POST http://localhost:3717/api/migrate/jobs/<job-id>/start

# Monitor progress
curl http://localhost:3717/api/migrate/jobs/<job-id>
```

### Example 4: Automated Backup Schedule

```bash
# Configure daily backups
curl -X PUT http://localhost:3717/api/backup/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "frequency": "daily",
    "time": "02:00",
    "backup_type": "incremental",
    "retention_days": 90,
    "storage_backend": "s3"
  }'

# Manual backup
curl -X POST http://localhost:3717/api/backup/create \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Manual Backup Before Update",
    "backup_type": "full",
    "scope": { "all_tables": true },
    "compression": "gzip",
    "encryption_enabled": true,
    "storage_backend": "s3",
    "retention_days": 30
  }'
```

### Example 5: Point-in-Time Restore

```bash
# List available snapshots
curl http://localhost:3717/api/backup/snapshots

# Preview restore
curl -X POST http://localhost:3717/api/restore/preview \
  -H "Content-Type: application/json" \
  -d '{
    "snapshot_id": "snapshot-uuid",
    "target_scope": {
      "tables": ["users", "messages"]
    }
  }'

# Create restore job
curl -X POST http://localhost:3717/api/restore/create \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "admin",
    "snapshot_id": "snapshot-uuid",
    "restore_type": "selective",
    "target_scope": {
      "tables": ["users"]
    },
    "conflict_resolution": "replace"
  }'

# Start restore
curl -X POST http://localhost:3717/api/restore/jobs/<job-id>/start
```

### Example 6: SQL Queries for Audit Log

```sql
-- View recent export activity
SELECT
  e.name,
  e.user_id,
  e.status,
  e.exported_records,
  e.output_size_bytes,
  e.created_at,
  e.completed_at
FROM ei_export_jobs e
WHERE e.source_account_id = 'primary'
  AND e.created_at > NOW() - INTERVAL '30 days'
ORDER BY e.created_at DESC;

-- View migration success rate
SELECT
  source_platform,
  COUNT(*) AS total_migrations,
  COUNT(*) FILTER (WHERE status = 'completed') AS successful,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed,
  ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'completed') / COUNT(*), 2) AS success_rate
FROM ei_migration_jobs
WHERE source_account_id = 'primary'
GROUP BY source_platform;

-- View backup storage usage
SELECT
  storage_backend,
  COUNT(*) AS snapshot_count,
  SUM(total_size_bytes) AS total_bytes,
  SUM(compressed_size_bytes) AS compressed_bytes,
  ROUND(100.0 * SUM(compressed_size_bytes) / NULLIF(SUM(total_size_bytes), 0), 2) AS compression_ratio
FROM ei_backup_snapshots
WHERE source_account_id = 'primary'
  AND expires_at IS NULL OR expires_at > NOW()
GROUP BY storage_backend;

-- View audit trail for user
SELECT
  a.job_type,
  a.action,
  a.records_affected,
  a.data_size_bytes,
  a.created_at
FROM ei_data_transfer_audit a
WHERE a.source_account_id = 'primary'
  AND a.user_id = 'user123'
ORDER BY a.created_at DESC
LIMIT 100;
```

---

## Troubleshooting

### Common Issues

#### "Database connection failed"

```
Error: Connection refused
```

**Solutions:**
1. Verify PostgreSQL is running
2. Check `DATABASE_URL` or individual `POSTGRES_*` variables
3. Test connection: `psql $DATABASE_URL -c "SELECT 1"`

```bash
# Check PostgreSQL status
pg_isready -h localhost -p 5432

# Test connection
psql "postgresql://user:pass@localhost:5432/nself" -c "SELECT version();"
```

#### "Export file too large"

```
Error: Export exceeds EXPORT_MAX_FILE_SIZE_GB
```

**Solutions:**
1. Increase `EXPORT_MAX_FILE_SIZE_GB` limit
2. Use selective export with filters
3. Split export into multiple jobs

```bash
# Increase limit
export EXPORT_MAX_FILE_SIZE_GB=20

# Use filters to reduce size
curl -X POST http://localhost:3717/api/export/create \
  -d '{
    "scope": { "tables": ["users"] },
    "filters": { "date_range": { "start": "2026-01-01" } }
  }'
```

#### "Import validation failed"

```
Error: Validation errors found in import file
```

**Solutions:**
1. Check validation errors in import job details
2. Use `validation_mode: "lenient"` for less strict validation
3. Fix data format issues in source file

```bash
# Check validation errors
curl http://localhost:3717/api/import/jobs/<job-id>

# Use lenient mode
curl -X POST http://localhost:3717/api/import/create \
  -d '{ "validation_mode": "lenient" }'
```

#### "Backup storage backend unavailable"

```
Error: Failed to connect to S3
```

**Solutions:**
1. Verify storage backend credentials (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
2. Check network connectivity to storage backend
3. Test with local backend first

```bash
# Switch to local backend temporarily
export BACKUP_STORAGE_BACKEND=local

# Test S3 access
aws s3 ls s3://your-bucket/
```

#### "Migration authentication failed"

```
Error: Invalid Slack API token
```

**Solutions:**
1. Verify API token is valid and not expired
2. Check required scopes/permissions for migration
3. Regenerate token from source platform

```bash
# Test Slack token
curl -H "Authorization: Bearer xoxb-your-token" \
  https://slack.com/api/auth.test
```

#### "Restore conflict resolution failed"

```
Error: Conflicts detected during restore
```

**Solutions:**
1. Use `conflict_resolution: "replace"` to overwrite existing data
2. Use `conflict_resolution: "skip"` to skip conflicts
3. Preview restore first to identify conflicts

```bash
# Preview to see conflicts
curl -X POST http://localhost:3717/api/restore/preview \
  -d '{ "snapshot_id": "uuid", "target_scope": {...} }'

# Use replace strategy
curl -X POST http://localhost:3717/api/restore/create \
  -d '{ "conflict_resolution": "replace" }'
```

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug nself plugin export-import server
```

### Health Checks

```bash
# Check plugin health
curl http://localhost:3717/health

# Check database connectivity
curl http://localhost:3717/ready

# Check detailed status
curl http://localhost:3717/v1/status
```

---

*Last Updated: February 11, 2026*
*Plugin Version: 1.0.0*
*nself Version: 0.4.8+*
