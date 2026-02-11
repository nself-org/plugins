# Data Operations Plugin

Comprehensive data operations platform with GDPR-compliant export/deletion, bulk import/export, cross-platform migration, backup/restore, and data portability.

---

## Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [SQL Query Examples](#sql-query-examples)
- [Use Cases](#use-cases)
- [GDPR Compliance](#gdpr-compliance)
- [Data Migration](#data-migration)
- [Backup & Restore](#backup--restore)
- [Data Transformation](#data-transformation)
- [Security & Verification](#security--verification)
- [Troubleshooting](#troubleshooting)
- [Performance](#performance)
- [Integration Examples](#integration-examples)

---

## Overview

The Data Operations plugin is a unified data management platform that provides comprehensive tools for data export, import, migration, backup, and compliance operations. Built for developers who need robust data portability, GDPR compliance, and cross-platform migration capabilities.

### What It Does

- **GDPR Compliance** - Right to be forgotten, data export, deletion requests
- **Bulk Operations** - Import/export large datasets with multiple format support (JSON, CSV, SQL)
- **Cross-Platform Migration** - Move data between different systems and platforms
- **Backup & Restore** - Automated backup scheduling and point-in-time restore
- **Data Transformation** - Templates for transforming data during migration
- **Audit Trail** - Complete audit log of all data operations
- **Verification Codes** - Secure sensitive operations with verification codes

### Why Use This Plugin

1. **Compliance First** - Built-in GDPR, CCPA, and data privacy compliance
2. **Multi-Format Support** - JSON, CSV, SQL, and custom formats
3. **Safe Operations** - Verification codes for destructive operations
4. **Audit Everything** - Complete audit trail for compliance and debugging
5. **Flexible Storage** - Local filesystem or cloud storage backends
6. **Plugin Registry** - Register other nself plugins for coordinated operations

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                  Data Operations Platform                    │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │  Export  │  │  Import  │  │ Deletion │  │ Migration│    │
│  │  Engine  │  │  Engine  │  │  Engine  │  │  Engine  │    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘    │
│       │             │             │             │           │
│  ┌────┴─────────────┴─────────────┴─────────────┴──────┐   │
│  │           Plugin Registry & Coordination            │   │
│  │    - Register external plugins                      │   │
│  │    - Coordinate cross-plugin operations             │   │
│  │    - Track dependencies                             │   │
│  └────────────────────┬─────────────────────────────────┘   │
│                       │                                     │
│  ┌────────────────────┴─────────────────────────────────┐   │
│  │         Storage Backends & Transformation           │   │
│  │    - Local filesystem                                │   │
│  │    - S3-compatible storage                           │   │
│  │    - Data transformation templates                   │   │
│  │    - Compression (gzip, brotli, zstd)               │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Audit & Compliance Layer                │   │
│  │    - All operations logged                           │   │
│  │    - Verification codes for sensitive ops            │   │
│  │    - Retention policies                              │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Key Features

### GDPR & Privacy Compliance

- **Right to Access** - Export all user data in machine-readable format
- **Right to Erasure** - "Right to be forgotten" with verification codes
- **Audit Trail** - Complete log of all data operations for compliance
- **Data Portability** - Export data in multiple formats (JSON, CSV, SQL)
- **Consent Management** - Track deletion requests and completion
- **Cooldown Periods** - Configurable cooldown before deletion executes

### Bulk Data Operations

- **Multi-Format Import** - JSON, CSV, SQL dump files
- **Multi-Format Export** - Choose output format per operation
- **Large File Support** - Handle files up to 10GB with streaming
- **Batch Processing** - Process records in configurable batch sizes
- **Progress Tracking** - Real-time progress updates for long operations
- **Error Recovery** - Resume failed imports from last checkpoint

### Cross-Platform Migration

- **Plugin-to-Plugin Migration** - Move data between nself plugins
- **External System Migration** - Import from third-party systems
- **Schema Mapping** - Define field mappings between systems
- **Transformation Templates** - Apply transformations during migration
- **Incremental Migration** - Migrate data in batches with rate limiting
- **Validation** - Verify data integrity after migration

### Backup & Restore

- **Automated Backups** - Schedule periodic backups
- **Full & Incremental** - Choose backup strategy
- **Point-in-Time Restore** - Restore to specific snapshot
- **Multiple Storage Backends** - Local, S3-compatible storage
- **Compression** - Reduce backup size with gzip/brotli/zstd
- **Retention Policies** - Automatic cleanup of old backups

### Data Transformation

- **Template System** - Define reusable transformation templates
- **Field Mapping** - Map fields between different schemas
- **Data Validation** - Validate data during transformation
- **Custom Scripts** - Execute custom transformation logic
- **Batch Processing** - Transform large datasets efficiently

---

## Quick Start

```bash
# Install the plugin
nself plugin install data-operations

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "DATAOPS_STORAGE_PATH=/tmp/nself-data-operations" >> .env

# Initialize database schema
nself plugin data-operations init

# Start the server
nself plugin data-operations server --port 3306

# Export user data (GDPR request)
nself plugin data-operations export \
  --user-id user_123 \
  --format json \
  --email user@example.com

# Create a backup
nself plugin data-operations backup create \
  --name "daily-backup" \
  --description "Daily automated backup"

# Import data from CSV
nself plugin data-operations import \
  --file /path/to/data.csv \
  --format csv \
  --table users

# Request data deletion (GDPR)
nself plugin data-operations delete \
  --user-id user_123 \
  --reason "User requested deletion"

# Verify and complete deletion
nself plugin data-operations delete verify \
  --request-id req_abc123 \
  --code 123456
```

---

## Installation

### Prerequisites

- **nself CLI**: Version 0.4.8 or higher
- **PostgreSQL**: Version 12 or higher
- **Node.js**: Version 18 or higher (if building from source)
- **Disk Space**: Adequate space for exports/backups (configurable location)

### Install via nself CLI

```bash
# Install latest version
nself plugin install data-operations

# Install specific version
nself plugin install data-operations@1.0.0

# Verify installation
nself plugin list | grep data-operations
```

### Manual Installation

```bash
# Clone repository
git clone https://github.com/acamarata/nself-plugins.git
cd nself-plugins/plugins/data-operations

# Build TypeScript implementation
cd ts
npm install
npm run build

# Initialize database
nself plugin data-operations init
```

### Post-Installation Setup

```bash
# 1. Create storage directories
mkdir -p /tmp/nself-data-operations/exports
mkdir -p /tmp/nself-data-operations/imports
mkdir -p /tmp/nself-data-operations/backups

# 2. Set permissions (production)
chmod 700 /tmp/nself-data-operations
chown app:app /tmp/nself-data-operations

# 3. Configure environment variables
cat > .env.data-operations <<EOF
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
DATAOPS_PLUGIN_PORT=3306
DATAOPS_STORAGE_PATH=/tmp/nself-data-operations
DATAOPS_TEMP_PATH=/tmp/nself-imports
DATAOPS_DOWNLOAD_EXPIRY_HOURS=24
DATAOPS_DELETION_COOLDOWN_HOURS=24
DATAOPS_MAX_EXPORT_SIZE_MB=500
DATAOPS_EXPORT_RETENTION_DAYS=30
DATAOPS_BACKUP_RETENTION_DAYS=90
EOF

# 4. Load environment
source .env.data-operations

# 5. Initialize schema
nself plugin data-operations init

# 6. Verify setup
nself plugin data-operations status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `DATAOPS_PLUGIN_PORT` | No | `3306` | HTTP server port |
| `DATAOPS_APP_IDS` | No | `primary` | Comma-separated app IDs for multi-app isolation |
| `DATAOPS_STORAGE_PATH` | No | `/tmp/nself-data-operations` | Base path for exports, backups, and temp files |
| `DATAOPS_TEMP_PATH` | No | `/tmp/nself-imports` | Temporary path for import processing |
| `DATAOPS_DOWNLOAD_EXPIRY_HOURS` | No | `24` | Hours before export download links expire |
| `DATAOPS_DELETION_COOLDOWN_HOURS` | No | `24` | Hours before deletion request executes |
| `DATAOPS_MAX_EXPORT_SIZE_MB` | No | `500` | Maximum export size in MB |
| `DATAOPS_MAX_FILE_SIZE_GB` | No | `10` | Maximum import file size in GB |
| `DATAOPS_VERIFICATION_CODE_LENGTH` | No | `6` | Length of verification codes |
| `DATAOPS_EXPORT_RETENTION_DAYS` | No | `30` | Days to retain completed exports |
| `DATAOPS_IMPORT_MAX_FILE_SIZE_GB` | No | `10` | Maximum import file size in GB |
| `DATAOPS_BACKUP_STORAGE_BACKEND` | No | `local` | Storage backend: `local`, `s3`, `gcs` |
| `DATAOPS_BACKUP_RETENTION_DAYS` | No | `90` | Days to retain backup snapshots |
| `DATAOPS_MIGRATION_BATCH_SIZE` | No | `100` | Records per batch during migration |
| `DATAOPS_MIGRATION_RATE_LIMIT_MS` | No | `0` | Milliseconds between migration batches (0 = no limit) |
| `DATAOPS_QUEUE_CONCURRENCY` | No | `5` | Number of concurrent job processors |
| `DATAOPS_QUEUE_TIMEOUT_MINUTES` | No | `60` | Maximum job execution time |
| `DATAOPS_COMPRESSION_LEVEL` | No | `6` | Compression level (1-9 for gzip) |
| `DATAOPS_COMPRESSION_ALGORITHM` | No | `gzip` | Compression: `gzip`, `brotli`, `zstd`, `none` |
| `DATAOPS_API_KEY` | No | - | API key for authenticated endpoints |
| `DATAOPS_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `DATAOPS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Server
DATAOPS_PLUGIN_PORT=3306

# Storage Configuration
DATAOPS_STORAGE_PATH=/var/lib/nself/data-operations
DATAOPS_TEMP_PATH=/tmp/nself-imports
DATAOPS_BACKUP_STORAGE_BACKEND=local

# GDPR & Compliance
DATAOPS_DOWNLOAD_EXPIRY_HOURS=24
DATAOPS_DELETION_COOLDOWN_HOURS=24
DATAOPS_VERIFICATION_CODE_LENGTH=6
DATAOPS_EXPORT_RETENTION_DAYS=30

# Performance
DATAOPS_MAX_EXPORT_SIZE_MB=1000
DATAOPS_MAX_FILE_SIZE_GB=10
DATAOPS_MIGRATION_BATCH_SIZE=100
DATAOPS_QUEUE_CONCURRENCY=5
DATAOPS_COMPRESSION_ALGORITHM=gzip
DATAOPS_COMPRESSION_LEVEL=6

# Backup
DATAOPS_BACKUP_RETENTION_DAYS=90

# Security
DATAOPS_API_KEY=your_secret_api_key_here
DATAOPS_RATE_LIMIT_MAX=100
DATAOPS_RATE_LIMIT_WINDOW_MS=60000

# Multi-App Isolation (optional)
DATAOPS_APP_IDS=app1,app2,app3
```

### Production Configuration

```bash
# Use production-grade storage
DATAOPS_STORAGE_PATH=/var/lib/nself/data-operations
DATAOPS_BACKUP_STORAGE_BACKEND=s3

# S3 Configuration (if using S3 backend)
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_S3_BUCKET=nself-backups
AWS_S3_REGION=us-east-1

# Stricter limits for production
DATAOPS_DOWNLOAD_EXPIRY_HOURS=6
DATAOPS_DELETION_COOLDOWN_HOURS=72
DATAOPS_MAX_EXPORT_SIZE_MB=2000

# Higher performance settings
DATAOPS_QUEUE_CONCURRENCY=10
DATAOPS_COMPRESSION_ALGORITHM=zstd
DATAOPS_COMPRESSION_LEVEL=3

# Longer retention for compliance
DATAOPS_EXPORT_RETENTION_DAYS=90
DATAOPS_BACKUP_RETENTION_DAYS=365
```

### Storage Backend Configuration

#### Local Storage

```bash
DATAOPS_BACKUP_STORAGE_BACKEND=local
DATAOPS_STORAGE_PATH=/var/lib/nself/data-operations
```

#### S3-Compatible Storage

```bash
DATAOPS_BACKUP_STORAGE_BACKEND=s3
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_S3_BUCKET=nself-backups
AWS_S3_REGION=us-east-1
AWS_S3_ENDPOINT=https://s3.amazonaws.com  # Optional: for S3-compatible services
```

#### Google Cloud Storage

```bash
DATAOPS_BACKUP_STORAGE_BACKEND=gcs
GCS_PROJECT_ID=your-project-id
GCS_BUCKET=nself-backups
GCS_KEYFILE=/path/to/service-account.json
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin data-operations init

# Start HTTP server
nself plugin data-operations server
nself plugin data-operations server --port 3306
nself plugin data-operations server --host 0.0.0.0 --port 3306

# Check overall status
nself plugin data-operations status

# View detailed statistics
nself plugin data-operations stats
```

### Export Commands

```bash
# Create export request (GDPR right to access)
nself plugin data-operations export \
  --user-id user_123 \
  --format json \
  --email user@example.com \
  --reason "User requested data export"

# Export with specific plugins
nself plugin data-operations export \
  --user-id user_123 \
  --format json \
  --plugins stripe,github,shopify \
  --email user@example.com

# Export to CSV
nself plugin data-operations export \
  --user-id user_123 \
  --format csv \
  --email user@example.com

# Export to SQL dump
nself plugin data-operations export \
  --user-id user_123 \
  --format sql \
  --email user@example.com

# List export requests
nself plugin data-operations export list

# List by status
nself plugin data-operations export list --status completed
nself plugin data-operations export list --status pending
nself plugin data-operations export list --status failed

# Get export request details
nself plugin data-operations export get req_abc123

# Download export file
nself plugin data-operations export download req_abc123 --output /path/to/export.json

# Cancel pending export
nself plugin data-operations export cancel req_abc123
```

### Import Commands

```bash
# Import from JSON file
nself plugin data-operations import \
  --file /path/to/data.json \
  --format json \
  --table users

# Import from CSV
nself plugin data-operations import \
  --file /path/to/data.csv \
  --format csv \
  --table users \
  --delimiter "," \
  --has-header

# Import from SQL dump
nself plugin data-operations import \
  --file /path/to/dump.sql \
  --format sql \
  --database target_db

# Import with transformation template
nself plugin data-operations import \
  --file /path/to/data.json \
  --format json \
  --table users \
  --template user-transform

# Import with dry-run (validation only)
nself plugin data-operations import \
  --file /path/to/data.json \
  --format json \
  --table users \
  --dry-run

# List import requests
nself plugin data-operations import list

# Get import details
nself plugin data-operations import get req_abc123

# Retry failed import
nself plugin data-operations import retry req_abc123

# Cancel import
nself plugin data-operations import cancel req_abc123
```

### Deletion Commands (GDPR)

```bash
# Request data deletion (right to be forgotten)
nself plugin data-operations delete \
  --user-id user_123 \
  --reason "User requested deletion" \
  --email user@example.com

# Request deletion for specific plugins
nself plugin data-operations delete \
  --user-id user_123 \
  --plugins stripe,github \
  --reason "Partial deletion" \
  --email user@example.com

# Verify deletion request with code
nself plugin data-operations delete verify \
  --request-id req_abc123 \
  --code 123456

# List deletion requests
nself plugin data-operations delete list

# List by status
nself plugin data-operations delete list --status pending
nself plugin data-operations delete list --status verified
nself plugin data-operations delete list --status completed

# Get deletion request details
nself plugin data-operations delete get req_abc123

# Cancel deletion request (before execution)
nself plugin data-operations delete cancel req_abc123
```

### Migration Commands

```bash
# Create migration job
nself plugin data-operations migrate \
  --source-plugin stripe \
  --target-plugin custom-billing \
  --description "Migrate Stripe data to custom billing"

# Create migration with transformation
nself plugin data-operations migrate \
  --source-plugin stripe \
  --target-plugin custom-billing \
  --template stripe-to-billing \
  --batch-size 100

# Create migration with mapping
nself plugin data-operations migrate \
  --source-plugin stripe \
  --target-plugin custom-billing \
  --mapping '{"stripe_customers": "billing_users", "stripe_subscriptions": "billing_plans"}'

# Start migration job
nself plugin data-operations migrate start job_abc123

# Pause migration
nself plugin data-operations migrate pause job_abc123

# Resume migration
nself plugin data-operations migrate resume job_abc123

# List migration jobs
nself plugin data-operations migrate list

# Get migration status
nself plugin data-operations migrate get job_abc123

# Cancel migration
nself plugin data-operations migrate cancel job_abc123
```

### Backup Commands

```bash
# Create manual backup
nself plugin data-operations backup create \
  --name "pre-migration-backup" \
  --description "Backup before data migration"

# Create backup for specific plugins
nself plugin data-operations backup create \
  --name "plugin-backup" \
  --plugins stripe,github

# Create compressed backup
nself plugin data-operations backup create \
  --name "compressed-backup" \
  --compression gzip \
  --compression-level 9

# List backups
nself plugin data-operations backup list

# List by date range
nself plugin data-operations backup list \
  --since "2024-01-01" \
  --until "2024-12-31"

# Get backup details
nself plugin data-operations backup get snap_abc123

# Download backup
nself plugin data-operations backup download snap_abc123 \
  --output /path/to/backup.tar.gz

# Delete old backup
nself plugin data-operations backup delete snap_abc123

# Schedule automatic backups
nself plugin data-operations backup schedule \
  --name "daily-backup" \
  --cron "0 2 * * *" \
  --retention-days 30

# List backup schedules
nself plugin data-operations backup schedule list

# Delete backup schedule
nself plugin data-operations backup schedule delete sched_abc123
```

### Restore Commands

```bash
# Restore from backup snapshot
nself plugin data-operations restore \
  --snapshot snap_abc123 \
  --target-database nself

# Restore specific plugins only
nself plugin data-operations restore \
  --snapshot snap_abc123 \
  --plugins stripe,github

# Restore to specific point in time
nself plugin data-operations restore \
  --snapshot snap_abc123 \
  --timestamp "2024-01-15 10:30:00"

# Dry-run restore (validation only)
nself plugin data-operations restore \
  --snapshot snap_abc123 \
  --dry-run

# List restore jobs
nself plugin data-operations restore list

# Get restore job status
nself plugin data-operations restore get job_abc123

# Cancel restore job
nself plugin data-operations restore cancel job_abc123
```

### Transform Template Commands

```bash
# Create transformation template
nself plugin data-operations transform create \
  --name "stripe-to-billing" \
  --description "Transform Stripe data to billing schema" \
  --template-file /path/to/template.json

# List templates
nself plugin data-operations transform list

# Get template details
nself plugin data-operations transform get stripe-to-billing

# Update template
nself plugin data-operations transform update stripe-to-billing \
  --template-file /path/to/updated-template.json

# Delete template
nself plugin data-operations transform delete stripe-to-billing

# Test template (dry-run)
nself plugin data-operations transform test stripe-to-billing \
  --sample-file /path/to/sample-data.json
```

### Plugin Registry Commands

```bash
# Register external plugin for data operations
nself plugin data-operations plugins register \
  --name stripe \
  --version 1.0.0 \
  --tables stripe_customers,stripe_subscriptions,stripe_invoices \
  --user-id-field customer_id

# List registered plugins
nself plugin data-operations plugins list

# Get plugin details
nself plugin data-operations plugins get stripe

# Unregister plugin
nself plugin data-operations plugins unregister stripe

# Update plugin registration
nself plugin data-operations plugins update stripe \
  --tables stripe_customers,stripe_subscriptions,stripe_invoices,stripe_charges
```

### Audit Commands

```bash
# View audit log
nself plugin data-operations audit list

# View audit log by user
nself plugin data-operations audit list --user-id user_123

# View audit log by operation
nself plugin data-operations audit list --operation export
nself plugin data-operations audit list --operation delete

# View audit log by date range
nself plugin data-operations audit list \
  --since "2024-01-01" \
  --until "2024-12-31"

# Get audit entry details
nself plugin data-operations audit get audit_abc123

# Export audit log
nself plugin data-operations audit export \
  --format csv \
  --output /path/to/audit.csv
```

---

## REST API

The plugin exposes a REST API when running the server.

### Base URL

```
http://localhost:3306
```

### Authentication

Most endpoints require API key authentication:

```bash
# Set API key in environment
export DATAOPS_API_KEY=your_secret_key

# Use in requests
curl -H "Authorization: Bearer your_secret_key" \
  http://localhost:3306/api/exports
```

### Endpoints

#### Health & Status

```http
GET /health
```
Returns server health status.

```http
GET /api/status
```
Returns overall statistics and system status.

**Example Response:**
```json
{
  "status": "ok",
  "uptime": 86400,
  "stats": {
    "exports": { "pending": 2, "completed": 150, "failed": 1 },
    "imports": { "pending": 0, "completed": 45, "failed": 0 },
    "deletions": { "pending": 1, "completed": 10 },
    "migrations": { "running": 0, "completed": 5 },
    "backups": { "total": 30, "size_gb": 15.4 }
  }
}
```

#### Export Endpoints

```http
POST /api/exports
Content-Type: application/json

{
  "user_id": "user_123",
  "format": "json",
  "email": "user@example.com",
  "plugins": ["stripe", "github"],
  "reason": "User requested data export"
}
```
Create a new export request.

```http
GET /api/exports
```
List all export requests. Query params: `status`, `user_id`, `limit`, `offset`.

```http
GET /api/exports/:id
```
Get export request details.

```http
GET /api/exports/:id/download
```
Download export file. Returns signed download URL or file stream.

```http
POST /api/exports/:id/cancel
```
Cancel pending export.

#### Import Endpoints

```http
POST /api/imports
Content-Type: multipart/form-data

file: [binary]
format: json
table: users
template: user-transform
```
Create a new import request.

```http
GET /api/imports
```
List all import requests. Query params: `status`, `limit`, `offset`.

```http
GET /api/imports/:id
```
Get import request details.

```http
POST /api/imports/:id/retry
```
Retry failed import.

```http
POST /api/imports/:id/cancel
```
Cancel import.

#### Deletion Endpoints

```http
POST /api/deletions
Content-Type: application/json

{
  "user_id": "user_123",
  "email": "user@example.com",
  "plugins": ["stripe", "github"],
  "reason": "User requested deletion"
}
```
Create deletion request. Returns verification code.

```http
POST /api/deletions/:id/verify
Content-Type: application/json

{
  "code": "123456"
}
```
Verify deletion request with code.

```http
GET /api/deletions
```
List deletion requests. Query params: `status`, `user_id`, `limit`, `offset`.

```http
GET /api/deletions/:id
```
Get deletion request details.

```http
POST /api/deletions/:id/cancel
```
Cancel deletion request (before execution).

#### Migration Endpoints

```http
POST /api/migrations
Content-Type: application/json

{
  "source_plugin": "stripe",
  "target_plugin": "custom-billing",
  "description": "Migrate Stripe to custom billing",
  "template": "stripe-to-billing",
  "mapping": {
    "stripe_customers": "billing_users"
  },
  "batch_size": 100
}
```
Create migration job.

```http
POST /api/migrations/:id/start
```
Start migration job.

```http
POST /api/migrations/:id/pause
```
Pause running migration.

```http
POST /api/migrations/:id/resume
```
Resume paused migration.

```http
GET /api/migrations
```
List migration jobs.

```http
GET /api/migrations/:id
```
Get migration status and progress.

```http
POST /api/migrations/:id/cancel
```
Cancel migration.

#### Backup Endpoints

```http
POST /api/backups
Content-Type: application/json

{
  "name": "manual-backup",
  "description": "Pre-migration backup",
  "plugins": ["stripe", "github"],
  "compression": "gzip",
  "compression_level": 6
}
```
Create backup snapshot.

```http
GET /api/backups
```
List backup snapshots. Query params: `since`, `until`, `limit`, `offset`.

```http
GET /api/backups/:id
```
Get backup details.

```http
GET /api/backups/:id/download
```
Download backup file.

```http
DELETE /api/backups/:id
```
Delete backup snapshot.

```http
POST /api/backups/schedules
Content-Type: application/json

{
  "name": "daily-backup",
  "cron": "0 2 * * *",
  "retention_days": 30,
  "plugins": ["*"],
  "compression": "gzip"
}
```
Create backup schedule.

```http
GET /api/backups/schedules
```
List backup schedules.

```http
DELETE /api/backups/schedules/:id
```
Delete backup schedule.

#### Restore Endpoints

```http
POST /api/restores
Content-Type: application/json

{
  "snapshot_id": "snap_abc123",
  "target_database": "nself",
  "plugins": ["stripe", "github"],
  "dry_run": false
}
```
Create restore job.

```http
GET /api/restores
```
List restore jobs.

```http
GET /api/restores/:id
```
Get restore job status.

```http
POST /api/restores/:id/cancel
```
Cancel restore job.

#### Transform Template Endpoints

```http
POST /api/transforms
Content-Type: application/json

{
  "name": "stripe-to-billing",
  "description": "Transform Stripe to billing schema",
  "template": {
    "mappings": [
      {
        "source": "stripe_customers",
        "target": "billing_users",
        "fields": {
          "id": "customer_id",
          "email": "user_email"
        }
      }
    ]
  }
}
```
Create transformation template.

```http
GET /api/transforms
```
List templates.

```http
GET /api/transforms/:name
```
Get template details.

```http
PUT /api/transforms/:name
```
Update template.

```http
DELETE /api/transforms/:name
```
Delete template.

```http
POST /api/transforms/:name/test
Content-Type: application/json

{
  "sample_data": [...]
}
```
Test template with sample data.

#### Plugin Registry Endpoints

```http
POST /api/plugins
Content-Type: application/json

{
  "name": "stripe",
  "version": "1.0.0",
  "tables": ["stripe_customers", "stripe_subscriptions"],
  "user_id_field": "customer_id"
}
```
Register plugin.

```http
GET /api/plugins
```
List registered plugins.

```http
GET /api/plugins/:name
```
Get plugin details.

```http
DELETE /api/plugins/:name
```
Unregister plugin.

#### Audit Endpoints

```http
GET /api/audit
```
List audit log entries. Query params: `user_id`, `operation`, `since`, `until`, `limit`, `offset`.

```http
GET /api/audit/:id
```
Get audit entry details.

```http
GET /api/audit/export
```
Export audit log. Query params: `format` (csv, json), `since`, `until`.

---

## Webhook Events

The plugin emits webhook events for async operations. Configure webhook URL in environment:

```bash
DATAOPS_WEBHOOK_URL=https://your-app.com/webhooks/data-operations
DATAOPS_WEBHOOK_SECRET=your_webhook_secret
```

### Event Types

| Event | Description | Payload Fields |
|-------|-------------|----------------|
| `export.completed` | Data export completed successfully | `request_id`, `user_id`, `format`, `file_size`, `download_url` |
| `export.failed` | Data export failed | `request_id`, `user_id`, `error`, `reason` |
| `import.completed` | Data import completed successfully | `request_id`, `table`, `records_imported`, `duration_ms` |
| `import.failed` | Data import failed | `request_id`, `table`, `error`, `records_processed` |
| `deletion.completed` | GDPR deletion completed | `request_id`, `user_id`, `tables_affected`, `records_deleted` |
| `migration.completed` | Data migration completed | `job_id`, `source_plugin`, `target_plugin`, `records_migrated` |
| `backup.completed` | Backup snapshot created | `snapshot_id`, `name`, `size_bytes`, `compression` |
| `restore.completed` | Data restore completed | `job_id`, `snapshot_id`, `tables_restored`, `duration_ms` |

### Webhook Payload Format

```json
{
  "event": "export.completed",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {
    "request_id": "req_abc123",
    "user_id": "user_123",
    "format": "json",
    "file_size": 1048576,
    "download_url": "https://storage.example.com/exports/req_abc123.json",
    "expires_at": "2024-01-16T10:30:00Z"
  },
  "signature": "sha256=..."
}
```

### Webhook Signature Verification

```python
import hmac
import hashlib

def verify_webhook(payload: str, signature: str, secret: str) -> bool:
    """Verify webhook signature"""
    expected = hmac.new(
        secret.encode(),
        payload.encode(),
        hashlib.sha256
    ).hexdigest()

    return hmac.compare_digest(f"sha256={expected}", signature)
```

### Example Webhook Handler

```javascript
const express = require('express');
const crypto = require('crypto');

app.post('/webhooks/data-operations', express.raw({ type: 'application/json' }), (req, res) => {
  const signature = req.headers['x-dataops-signature'];
  const payload = req.body.toString();

  // Verify signature
  const expectedSignature = crypto
    .createHmac('sha256', process.env.DATAOPS_WEBHOOK_SECRET)
    .update(payload)
    .digest('hex');

  if (!crypto.timingSafeEqual(
    Buffer.from(`sha256=${expectedSignature}`),
    Buffer.from(signature)
  )) {
    return res.status(401).send('Invalid signature');
  }

  // Parse and handle event
  const event = JSON.parse(payload);

  switch (event.event) {
    case 'export.completed':
      console.log('Export completed:', event.data.request_id);
      // Send email to user with download link
      break;

    case 'deletion.completed':
      console.log('Deletion completed:', event.data.user_id);
      // Update internal records
      break;

    // ... handle other events
  }

  res.status(200).send('OK');
});
```

---

## Database Schema

### dataops_export_requests

Tracks GDPR data export requests.

```sql
CREATE TABLE dataops_export_requests (
    id VARCHAR(255) PRIMARY KEY,              -- req_xxx
    user_id VARCHAR(255) NOT NULL,            -- User identifier
    status VARCHAR(50) NOT NULL,              -- pending, processing, completed, failed, expired
    format VARCHAR(20) NOT NULL,              -- json, csv, sql
    plugins JSONB DEFAULT '[]',               -- Plugins to export from
    email VARCHAR(255),                        -- Email to notify
    reason TEXT,                               -- User-provided reason
    file_path VARCHAR(2048),                   -- Path to export file
    file_size_bytes BIGINT,                    -- Export file size
    download_url VARCHAR(2048),                -- Signed download URL
    download_expires_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,                        -- Error if failed
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_dataops_export_requests_user ON dataops_export_requests(user_id);
CREATE INDEX idx_dataops_export_requests_status ON dataops_export_requests(status);
CREATE INDEX idx_dataops_export_requests_created ON dataops_export_requests(created_at DESC);
CREATE INDEX idx_dataops_export_requests_expires ON dataops_export_requests(expires_at);
```

### dataops_import_requests

Tracks bulk import operations.

```sql
CREATE TABLE dataops_import_requests (
    id VARCHAR(255) PRIMARY KEY,              -- req_xxx
    status VARCHAR(50) NOT NULL,              -- pending, processing, completed, failed, canceled
    format VARCHAR(20) NOT NULL,              -- json, csv, sql
    file_path VARCHAR(2048) NOT NULL,         -- Path to import file
    file_size_bytes BIGINT,                   -- Import file size
    target_table VARCHAR(255),                 -- Target table name
    template_name VARCHAR(255),                -- Transformation template
    options JSONB DEFAULT '{}',                -- Import options (delimiter, encoding, etc.)
    records_total INTEGER,                     -- Total records in file
    records_imported INTEGER DEFAULT 0,        -- Successfully imported
    records_failed INTEGER DEFAULT 0,          -- Failed records
    error_message TEXT,                        -- Error if failed
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_dataops_import_requests_status ON dataops_import_requests(status);
CREATE INDEX idx_dataops_import_requests_created ON dataops_import_requests(created_at DESC);
CREATE INDEX idx_dataops_import_requests_table ON dataops_import_requests(target_table);
```

### dataops_deletion_requests

Tracks GDPR deletion requests (right to be forgotten).

```sql
CREATE TABLE dataops_deletion_requests (
    id VARCHAR(255) PRIMARY KEY,              -- req_xxx
    user_id VARCHAR(255) NOT NULL,            -- User to delete
    status VARCHAR(50) NOT NULL,              -- pending, verified, processing, completed, failed, canceled
    verification_code VARCHAR(20),             -- 6-digit verification code
    verification_code_sent_at TIMESTAMP WITH TIME ZONE,
    verification_code_expires_at TIMESTAMP WITH TIME ZONE,
    verified_at TIMESTAMP WITH TIME ZONE,
    plugins JSONB DEFAULT '[]',               -- Plugins to delete from
    email VARCHAR(255),                        -- Email for notifications
    reason TEXT,                               -- User-provided reason
    tables_affected JSONB DEFAULT '[]',        -- Tables deleted from
    records_deleted INTEGER DEFAULT 0,         -- Total records deleted
    error_message TEXT,                        -- Error if failed
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    cooldown_until TIMESTAMP WITH TIME ZONE,   -- Deletion executes after this
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_dataops_deletion_requests_user ON dataops_deletion_requests(user_id);
CREATE INDEX idx_dataops_deletion_requests_status ON dataops_deletion_requests(status);
CREATE INDEX idx_dataops_deletion_requests_created ON dataops_deletion_requests(created_at DESC);
CREATE INDEX idx_dataops_deletion_requests_cooldown ON dataops_deletion_requests(cooldown_until);
```

### dataops_migration_jobs

Tracks cross-platform data migration jobs.

```sql
CREATE TABLE dataops_migration_jobs (
    id VARCHAR(255) PRIMARY KEY,              -- job_xxx
    status VARCHAR(50) NOT NULL,              -- pending, running, paused, completed, failed, canceled
    source_plugin VARCHAR(255) NOT NULL,       -- Source plugin name
    target_plugin VARCHAR(255) NOT NULL,       -- Target plugin name
    description TEXT,                          -- Job description
    template_name VARCHAR(255),                -- Transformation template
    table_mapping JSONB DEFAULT '{}',          -- Source to target table mappings
    batch_size INTEGER DEFAULT 100,            -- Records per batch
    rate_limit_ms INTEGER DEFAULT 0,           -- Delay between batches
    records_total INTEGER,                     -- Total records to migrate
    records_migrated INTEGER DEFAULT 0,        -- Successfully migrated
    records_failed INTEGER DEFAULT 0,          -- Failed records
    current_table VARCHAR(255),                -- Currently processing table
    progress_pct DECIMAL(5,2) DEFAULT 0.00,   -- Progress percentage
    error_message TEXT,                        -- Error if failed
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    paused_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_dataops_migration_jobs_status ON dataops_migration_jobs(status);
CREATE INDEX idx_dataops_migration_jobs_created ON dataops_migration_jobs(created_at DESC);
CREATE INDEX idx_dataops_migration_jobs_source ON dataops_migration_jobs(source_plugin);
CREATE INDEX idx_dataops_migration_jobs_target ON dataops_migration_jobs(target_plugin);
```

### dataops_backup_snapshots

Tracks backup snapshots.

```sql
CREATE TABLE dataops_backup_snapshots (
    id VARCHAR(255) PRIMARY KEY,              -- snap_xxx
    name VARCHAR(255) NOT NULL,                -- Backup name
    description TEXT,                          -- Backup description
    status VARCHAR(50) NOT NULL,              -- pending, creating, completed, failed
    storage_backend VARCHAR(50) NOT NULL,      -- local, s3, gcs
    storage_path VARCHAR(2048),                -- Path/key in storage
    plugins JSONB DEFAULT '[]',               -- Plugins included
    compression VARCHAR(20),                   -- gzip, brotli, zstd, none
    compression_level INTEGER,                 -- Compression level (1-9)
    size_bytes BIGINT,                        -- Backup file size
    size_uncompressed_bytes BIGINT,           -- Uncompressed size
    tables_included JSONB DEFAULT '[]',        -- Tables in backup
    checksum_sha256 VARCHAR(64),              -- SHA-256 checksum
    error_message TEXT,                        -- Error if failed
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    expires_at TIMESTAMP WITH TIME ZONE,       -- Auto-delete after this
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_dataops_backup_snapshots_status ON dataops_backup_snapshots(status);
CREATE INDEX idx_dataops_backup_snapshots_created ON dataops_backup_snapshots(created_at DESC);
CREATE INDEX idx_dataops_backup_snapshots_name ON dataops_backup_snapshots(name);
CREATE INDEX idx_dataops_backup_snapshots_expires ON dataops_backup_snapshots(expires_at);
```

### dataops_restore_jobs

Tracks restore operations.

```sql
CREATE TABLE dataops_restore_jobs (
    id VARCHAR(255) PRIMARY KEY,              -- job_xxx
    snapshot_id VARCHAR(255) NOT NULL,         -- Backup snapshot to restore
    status VARCHAR(50) NOT NULL,              -- pending, running, completed, failed, canceled
    target_database VARCHAR(255),              -- Target database name
    plugins JSONB DEFAULT '[]',               -- Plugins to restore
    dry_run BOOLEAN DEFAULT FALSE,            -- Validation only
    tables_restored JSONB DEFAULT '[]',        -- Tables restored
    records_restored INTEGER DEFAULT 0,        -- Total records restored
    error_message TEXT,                        -- Error if failed
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_dataops_restore_jobs_status ON dataops_restore_jobs(status);
CREATE INDEX idx_dataops_restore_jobs_snapshot ON dataops_restore_jobs(snapshot_id);
CREATE INDEX idx_dataops_restore_jobs_created ON dataops_restore_jobs(created_at DESC);
```

### dataops_transform_templates

Stores data transformation templates.

```sql
CREATE TABLE dataops_transform_templates (
    id VARCHAR(255) PRIMARY KEY,              -- Generated ID
    name VARCHAR(255) UNIQUE NOT NULL,        -- Template name
    description TEXT,                          -- Template description
    template_spec JSONB NOT NULL,             -- Transformation specification
    version INTEGER DEFAULT 1,                 -- Template version
    is_active BOOLEAN DEFAULT TRUE,           -- Active status
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_dataops_transform_templates_name ON dataops_transform_templates(name);
CREATE INDEX idx_dataops_transform_templates_active ON dataops_transform_templates(is_active);
```

### dataops_plugin_registry

Registry of plugins for coordinated operations.

```sql
CREATE TABLE dataops_plugin_registry (
    id VARCHAR(255) PRIMARY KEY,              -- Generated ID
    name VARCHAR(255) UNIQUE NOT NULL,        -- Plugin name
    version VARCHAR(50),                       -- Plugin version
    tables JSONB DEFAULT '[]',                -- Tables owned by plugin
    user_id_field VARCHAR(255),                -- Field name for user ID
    supports_export BOOLEAN DEFAULT TRUE,     -- Supports export operations
    supports_deletion BOOLEAN DEFAULT TRUE,   -- Supports deletion operations
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    registered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_dataops_plugin_registry_name ON dataops_plugin_registry(name);
```

### dataops_transfer_audit

Audit log of all data operations.

```sql
CREATE TABLE dataops_transfer_audit (
    id VARCHAR(255) PRIMARY KEY,              -- audit_xxx
    operation VARCHAR(50) NOT NULL,           -- export, import, delete, migrate, backup, restore
    operation_id VARCHAR(255),                 -- Request/job ID
    user_id VARCHAR(255),                      -- User affected
    status VARCHAR(50) NOT NULL,              -- success, failure
    details JSONB DEFAULT '{}',                -- Operation details
    error_message TEXT,                        -- Error if failed
    ip_address VARCHAR(45),                    -- Client IP
    user_agent TEXT,                           -- Client user agent
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_dataops_transfer_audit_operation ON dataops_transfer_audit(operation);
CREATE INDEX idx_dataops_transfer_audit_user ON dataops_transfer_audit(user_id);
CREATE INDEX idx_dataops_transfer_audit_created ON dataops_transfer_audit(created_at DESC);
CREATE INDEX idx_dataops_transfer_audit_status ON dataops_transfer_audit(status);
```

### dataops_backup_schedule

Automated backup schedules.

```sql
CREATE TABLE dataops_backup_schedule (
    id VARCHAR(255) PRIMARY KEY,              -- sched_xxx
    name VARCHAR(255) NOT NULL,                -- Schedule name
    description TEXT,                          -- Schedule description
    cron_expression VARCHAR(100) NOT NULL,     -- Cron schedule
    is_active BOOLEAN DEFAULT TRUE,           -- Active status
    plugins JSONB DEFAULT '[]',               -- Plugins to backup
    compression VARCHAR(20) DEFAULT 'gzip',   -- Compression algorithm
    compression_level INTEGER DEFAULT 6,      -- Compression level
    retention_days INTEGER DEFAULT 90,        -- Days to retain backups
    last_run_at TIMESTAMP WITH TIME ZONE,     -- Last execution time
    next_run_at TIMESTAMP WITH TIME ZONE,     -- Next execution time
    metadata JSONB DEFAULT '{}',               -- Additional metadata
    source_account_id VARCHAR(255) DEFAULT 'primary',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_dataops_backup_schedule_active ON dataops_backup_schedule(is_active);
CREATE INDEX idx_dataops_backup_schedule_next_run ON dataops_backup_schedule(next_run_at);
```

### dataops_webhook_events

Webhook event log.

```sql
CREATE TABLE dataops_webhook_events (
    id VARCHAR(255) PRIMARY KEY,              -- evt_xxx
    event VARCHAR(100) NOT NULL,              -- Event type
    payload JSONB NOT NULL,                   -- Event payload
    url VARCHAR(2048),                        -- Webhook URL
    status VARCHAR(50) NOT NULL,              -- pending, sent, failed
    response_code INTEGER,                    -- HTTP response code
    response_body TEXT,                       -- Response body
    retry_count INTEGER DEFAULT 0,            -- Retry attempts
    error_message TEXT,                       -- Error if failed
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    sent_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_dataops_webhook_events_event ON dataops_webhook_events(event);
CREATE INDEX idx_dataops_webhook_events_status ON dataops_webhook_events(status);
CREATE INDEX idx_dataops_webhook_events_created ON dataops_webhook_events(created_at DESC);
```

---

## SQL Query Examples

### Find All Exports for a User

```sql
SELECT
    id,
    status,
    format,
    plugins,
    file_size_bytes / 1024.0 / 1024.0 AS file_size_mb,
    download_url,
    download_expires_at,
    created_at
FROM dataops_export_requests
WHERE user_id = 'user_123'
ORDER BY created_at DESC;
```

### Active Deletion Requests Waiting for Cooldown

```sql
SELECT
    id,
    user_id,
    status,
    email,
    plugins,
    cooldown_until,
    cooldown_until - NOW() AS time_remaining,
    created_at
FROM dataops_deletion_requests
WHERE status = 'verified'
  AND cooldown_until > NOW()
ORDER BY cooldown_until;
```

### Migration Jobs Summary

```sql
SELECT
    source_plugin,
    target_plugin,
    status,
    COUNT(*) AS job_count,
    SUM(records_migrated) AS total_records_migrated,
    AVG(progress_pct) AS avg_progress,
    AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) AS avg_duration_seconds
FROM dataops_migration_jobs
GROUP BY source_plugin, target_plugin, status
ORDER BY source_plugin, target_plugin;
```

### Backup Storage Usage

```sql
SELECT
    storage_backend,
    compression,
    COUNT(*) AS snapshot_count,
    SUM(size_bytes) / 1024.0 / 1024.0 / 1024.0 AS total_size_gb,
    SUM(size_uncompressed_bytes) / 1024.0 / 1024.0 / 1024.0 AS uncompressed_size_gb,
    AVG(size_bytes::FLOAT / NULLIF(size_uncompressed_bytes, 0)) AS avg_compression_ratio
FROM dataops_backup_snapshots
WHERE status = 'completed'
GROUP BY storage_backend, compression;
```

### Expired Exports to Clean Up

```sql
SELECT
    id,
    user_id,
    format,
    file_path,
    file_size_bytes / 1024.0 / 1024.0 AS file_size_mb,
    expires_at,
    NOW() - expires_at AS expired_for
FROM dataops_export_requests
WHERE status = 'completed'
  AND expires_at < NOW()
ORDER BY expires_at;
```

### Audit Trail for User

```sql
SELECT
    operation,
    operation_id,
    status,
    details,
    created_at
FROM dataops_transfer_audit
WHERE user_id = 'user_123'
ORDER BY created_at DESC
LIMIT 50;
```

### Failed Operations Requiring Attention

```sql
SELECT
    'export' AS operation_type,
    id AS operation_id,
    user_id,
    error_message,
    created_at
FROM dataops_export_requests
WHERE status = 'failed'

UNION ALL

SELECT
    'import' AS operation_type,
    id AS operation_id,
    NULL AS user_id,
    error_message,
    created_at
FROM dataops_import_requests
WHERE status = 'failed'

UNION ALL

SELECT
    'deletion' AS operation_type,
    id AS operation_id,
    user_id,
    error_message,
    created_at
FROM dataops_deletion_requests
WHERE status = 'failed'

ORDER BY created_at DESC;
```

### Data Operations Statistics

```sql
SELECT
    'exports' AS operation,
    COUNT(*) FILTER (WHERE status = 'pending') AS pending,
    COUNT(*) FILTER (WHERE status = 'processing') AS processing,
    COUNT(*) FILTER (WHERE status = 'completed') AS completed,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed
FROM dataops_export_requests

UNION ALL

SELECT
    'imports' AS operation,
    COUNT(*) FILTER (WHERE status = 'pending'),
    COUNT(*) FILTER (WHERE status = 'processing'),
    COUNT(*) FILTER (WHERE status = 'completed'),
    COUNT(*) FILTER (WHERE status = 'failed')
FROM dataops_import_requests

UNION ALL

SELECT
    'deletions' AS operation,
    COUNT(*) FILTER (WHERE status = 'pending'),
    COUNT(*) FILTER (WHERE status = 'processing'),
    COUNT(*) FILTER (WHERE status = 'completed'),
    COUNT(*) FILTER (WHERE status = 'failed')
FROM dataops_deletion_requests

UNION ALL

SELECT
    'migrations' AS operation,
    COUNT(*) FILTER (WHERE status = 'pending'),
    COUNT(*) FILTER (WHERE status = 'running'),
    COUNT(*) FILTER (WHERE status = 'completed'),
    COUNT(*) FILTER (WHERE status = 'failed')
FROM dataops_migration_jobs;
```

### Upcoming Scheduled Backups

```sql
SELECT
    name,
    description,
    cron_expression,
    next_run_at,
    next_run_at - NOW() AS time_until_next_run,
    last_run_at,
    retention_days,
    plugins
FROM dataops_backup_schedule
WHERE is_active = TRUE
ORDER BY next_run_at;
```

---

## Use Cases

### Use Case 1: GDPR Data Export Request

**Scenario:** A user requests export of all their data (GDPR Article 15 - Right of Access).

**Steps:**

1. **Create Export Request:**
```bash
nself plugin data-operations export \
  --user-id user_123 \
  --format json \
  --email user@example.com \
  --reason "GDPR Article 15 request"
```

2. **System Actions:**
   - Generates unique request ID (e.g., `req_abc123`)
   - Queries all registered plugins for user data
   - Collects data from tables: `stripe_customers`, `github_users`, etc.
   - Creates JSON export with structure:
     ```json
     {
       "user_id": "user_123",
       "export_date": "2024-01-15T10:30:00Z",
       "data": {
         "stripe": { "customers": [...], "subscriptions": [...] },
         "github": { "users": [...], "repositories": [...] }
       }
     }
     ```
   - Compresses export (gzip)
   - Generates signed download URL
   - Sends email with download link

3. **User Downloads:**
```bash
# Via CLI
nself plugin data-operations export download req_abc123 \
  --output my-data.json.gz

# Via browser (from email link)
https://storage.example.com/exports/req_abc123.json.gz?signature=...
```

4. **Automatic Cleanup:**
   - Export expires after 24 hours (configurable)
   - File automatically deleted after 30 days (configurable)

### Use Case 2: GDPR Right to Be Forgotten

**Scenario:** A user requests complete deletion of all personal data (GDPR Article 17).

**Steps:**

1. **Create Deletion Request:**
```bash
nself plugin data-operations delete \
  --user-id user_123 \
  --email user@example.com \
  --reason "GDPR Article 17 - Right to erasure"
```

2. **System Actions:**
   - Generates unique request ID (e.g., `req_def456`)
   - Creates 6-digit verification code
   - Sends email with verification code
   - Sets cooldown period (24 hours default)

3. **User Verifies Request:**
```bash
nself plugin data-operations delete verify \
  --request-id req_def456 \
  --code 123456
```

4. **After Cooldown Period:**
   - System automatically executes deletion
   - Deletes data from all registered plugin tables
   - Logs deletion in audit trail
   - Sends confirmation email
   - Emits `deletion.completed` webhook

5. **Audit Record:**
```sql
SELECT * FROM dataops_transfer_audit
WHERE operation = 'delete' AND user_id = 'user_123';
```

### Use Case 3: Migrate from Stripe to Custom Billing

**Scenario:** Migrate from Stripe plugin to custom billing system.

**Steps:**

1. **Create Transformation Template:**
```bash
cat > stripe-to-billing.json <<EOF
{
  "name": "stripe-to-billing",
  "mappings": [
    {
      "source": "stripe_customers",
      "target": "billing_users",
      "fields": {
        "id": "stripe_customer_id",
        "email": "user_email",
        "name": "display_name",
        "metadata": "custom_fields"
      }
    },
    {
      "source": "stripe_subscriptions",
      "target": "billing_subscriptions",
      "fields": {
        "id": "stripe_subscription_id",
        "customer_id": "user_id",
        "status": "subscription_status"
      }
    }
  ]
}
EOF

nself plugin data-operations transform create \
  --name stripe-to-billing \
  --template-file stripe-to-billing.json
```

2. **Create Migration Job:**
```bash
nself plugin data-operations migrate \
  --source-plugin stripe \
  --target-plugin billing \
  --template stripe-to-billing \
  --batch-size 100 \
  --description "Migrate Stripe to custom billing"
```

3. **Start Migration:**
```bash
nself plugin data-operations migrate start job_ghi789
```

4. **Monitor Progress:**
```bash
# CLI
nself plugin data-operations migrate get job_ghi789

# API
curl http://localhost:3306/api/migrations/job_ghi789
```

5. **After Completion:**
   - Verify data integrity
   - Run validation queries
   - Update application to use new billing tables

### Use Case 4: Automated Daily Backups

**Scenario:** Set up automated daily backups with 90-day retention.

**Steps:**

1. **Create Backup Schedule:**
```bash
nself plugin data-operations backup schedule \
  --name "daily-production-backup" \
  --cron "0 2 * * *" \
  --retention-days 90 \
  --compression gzip \
  --compression-level 6
```

2. **System Actions:**
   - Runs daily at 2:00 AM
   - Creates compressed backup of all plugin data
   - Stores in configured storage backend (local or S3)
   - Calculates SHA-256 checksum
   - Emits `backup.completed` webhook
   - Automatically deletes backups older than 90 days

3. **Manual Backup Before Major Change:**
```bash
nself plugin data-operations backup create \
  --name "pre-migration-backup" \
  --description "Backup before Stripe migration"
```

4. **Restore from Backup:**
```bash
# List backups
nself plugin data-operations backup list

# Restore
nself plugin data-operations restore \
  --snapshot snap_jkl012 \
  --target-database nself_restore
```

### Use Case 5: Bulk Import from CSV

**Scenario:** Import customer data from legacy system CSV export.

**Steps:**

1. **Prepare CSV File:**
```csv
customer_id,email,name,phone,created_at
cust_001,john@example.com,John Doe,+1234567890,2024-01-01T00:00:00Z
cust_002,jane@example.com,Jane Smith,+0987654321,2024-01-02T00:00:00Z
```

2. **Import Data:**
```bash
nself plugin data-operations import \
  --file /path/to/customers.csv \
  --format csv \
  --table legacy_customers \
  --delimiter "," \
  --has-header
```

3. **System Actions:**
   - Validates CSV format
   - Parses records in batches
   - Inserts into `legacy_customers` table
   - Tracks progress
   - Reports errors for invalid records
   - Emits `import.completed` webhook

4. **Check Import Status:**
```bash
nself plugin data-operations import get req_mno345
```

### Use Case 6: Cross-Plugin Data Portability

**Scenario:** User switches from your platform to competitor, requests all data.

**Steps:**

1. **Register All Plugins:**
```bash
nself plugin data-operations plugins register \
  --name stripe --version 1.0.0 \
  --tables stripe_customers,stripe_subscriptions,stripe_invoices \
  --user-id-field customer_id

nself plugin data-operations plugins register \
  --name github --version 1.0.0 \
  --tables github_users,github_repositories \
  --user-id-field user_id
```

2. **Create Comprehensive Export:**
```bash
nself plugin data-operations export \
  --user-id user_123 \
  --format json \
  --plugins stripe,github,shopify,custom-plugin \
  --email user@example.com \
  --reason "User requested data portability"
```

3. **System Collects Data From:**
   - Stripe: customers, subscriptions, invoices, charges
   - GitHub: user profile, repositories, issues
   - Shopify: orders, products, customers
   - Custom Plugin: application-specific data

4. **User Receives:**
   - Single JSON file with all data
   - Machine-readable format
   - Suitable for import to competitor system

---

## GDPR Compliance

### Supported GDPR Rights

| Right | Article | Implementation |
|-------|---------|----------------|
| Right of Access | Art. 15 | Export user data in machine-readable format (JSON, CSV, SQL) |
| Right to Erasure | Art. 17 | Delete all user data with verification code |
| Right to Portability | Art. 20 | Export data in structured format for transfer to another system |
| Right to Be Informed | Art. 13-14 | Audit trail of all operations on user data |
| Right to Rectification | Art. 16 | Import/update operations with audit trail |

### GDPR Compliance Features

**1. Data Export (Right of Access)**
- Machine-readable formats (JSON, CSV, SQL)
- Complete data from all plugins
- Automated email notification
- Secure download links with expiration
- Audit trail of export requests

**2. Data Deletion (Right to Erasure)**
- Verification codes for security
- Configurable cooldown period (default 24 hours)
- Cascading deletion across plugins
- Audit trail of deletions
- Email confirmation
- Cannot be reversed after execution

**3. Data Portability**
- Export in open formats (JSON, CSV)
- Structured data suitable for import to other systems
- Includes metadata and relationships
- Compressed for efficient transfer

**4. Audit Trail**
- Every operation logged in `dataops_transfer_audit`
- IP address and user agent recorded
- Timestamps for all actions
- Export audit log for compliance reporting

**5. Retention Policies**
- Configurable retention for exports (default 30 days)
- Configurable retention for backups (default 90 days)
- Automatic cleanup of expired data
- Audit log retention (recommended: 7 years)

### GDPR Compliance Checklist

- [ ] Configure cooldown period for deletions (24-72 hours recommended)
- [ ] Set export expiration (6-24 hours recommended)
- [ ] Enable email notifications for all requests
- [ ] Implement webhook handlers for `deletion.completed`
- [ ] Register all plugins in plugin registry
- [ ] Configure audit log retention (7 years for GDPR)
- [ ] Set up automated backup schedules
- [ ] Test export process with sample user
- [ ] Test deletion process with sample user
- [ ] Document data retention policies
- [ ] Create user-facing privacy policy
- [ ] Train staff on GDPR request handling

### Example GDPR Workflows

**User Requests Data Export:**
```bash
# 1. User submits request (via app UI or support ticket)
# 2. Admin creates export
nself plugin data-operations export \
  --user-id user_123 \
  --format json \
  --email user@example.com \
  --reason "GDPR Article 15 request"

# 3. User receives email with download link
# 4. User downloads within 24 hours
# 5. Link expires automatically
# 6. File deleted after 30 days
```

**User Requests Deletion:**
```bash
# 1. User submits deletion request
nself plugin data-operations delete \
  --user-id user_123 \
  --email user@example.com \
  --reason "GDPR Article 17 request"

# 2. User receives verification code via email
# 3. User verifies request
nself plugin data-operations delete verify \
  --request-id req_def456 \
  --code 123456

# 4. System waits cooldown period (24 hours)
# 5. System automatically deletes data
# 6. User receives confirmation email
# 7. Operation logged in audit trail
```

---

## Data Migration

### Migration Process

1. **Planning Phase:**
   - Identify source and target systems
   - Map source tables to target tables
   - Define field mappings
   - Create transformation template
   - Determine batch size and rate limits

2. **Preparation Phase:**
   - Create backup before migration
   - Create transformation template
   - Test with sample data
   - Validate target schema exists

3. **Execution Phase:**
   - Create migration job
   - Start migration
   - Monitor progress
   - Handle errors

4. **Validation Phase:**
   - Verify record counts
   - Validate data integrity
   - Compare sample records
   - Run business logic tests

5. **Cutover Phase:**
   - Update application configuration
   - Redirect traffic to new system
   - Monitor for issues
   - Keep old system as backup

### Transformation Templates

Transformation templates define how data is mapped and transformed during migration.

**Template Structure:**
```json
{
  "name": "stripe-to-billing",
  "description": "Transform Stripe data to custom billing schema",
  "version": 1,
  "mappings": [
    {
      "source": "stripe_customers",
      "target": "billing_users",
      "fields": {
        "id": "stripe_customer_id",
        "email": "user_email",
        "name": "display_name",
        "created_at": "registered_at"
      },
      "transformations": [
        {
          "field": "status",
          "type": "map",
          "mapping": {
            "active": "enabled",
            "inactive": "disabled"
          }
        },
        {
          "field": "metadata",
          "type": "jsonb",
          "extract": ["plan", "referral_source"]
        }
      ],
      "filters": [
        {
          "field": "deleted_at",
          "operator": "is_null"
        }
      ]
    }
  ]
}
```

**Transformation Types:**

- **Direct Mapping:** Copy field value as-is
- **Value Mapping:** Map enum values (e.g., `active` → `enabled`)
- **JSONB Extraction:** Extract fields from JSONB column
- **Date Transformation:** Convert date formats
- **Concatenation:** Combine multiple fields
- **Split:** Split field into multiple fields
- **Custom Function:** Apply custom transformation logic

### Migration Strategies

**1. Full Migration:**
- Migrate all data in one pass
- Use for small to medium datasets
- Requires downtime or dual-write period

**2. Incremental Migration:**
- Migrate data in batches
- Use for large datasets
- Allows for zero-downtime migration
- Migrate historical data first, then recent data

**3. Dual-Write Migration:**
- Write to both old and new systems
- Gradually migrate historical data
- Cut over when migration complete
- No downtime required

**4. Shadow Migration:**
- Migrate data in background
- Validate migrated data
- Switch traffic when ready
- Rollback easily if issues found

---

## Backup & Restore

### Backup Types

**1. Full Backup:**
- Complete snapshot of all data
- Suitable for small to medium databases
- Longer backup time
- Faster restore time

**2. Incremental Backup:**
- Only data changed since last backup
- Faster backup time
- More complex restore process
- Requires full backup + all incremental backups

**3. Plugin-Specific Backup:**
- Backup specific plugins only
- Faster backup time
- Selective restore capability

### Backup Storage Backends

**Local Storage:**
```bash
DATAOPS_BACKUP_STORAGE_BACKEND=local
DATAOPS_STORAGE_PATH=/var/lib/nself/data-operations
```

Pros:
- Fast backup/restore
- No external dependencies
- No data transfer costs

Cons:
- Limited by disk space
- No off-site backup
- Single point of failure

**S3-Compatible Storage:**
```bash
DATAOPS_BACKUP_STORAGE_BACKEND=s3
AWS_S3_BUCKET=nself-backups
AWS_S3_REGION=us-east-1
```

Pros:
- Unlimited storage
- High durability (99.999999999%)
- Off-site backup
- Versioning support

Cons:
- Data transfer costs
- Slower than local
- Requires internet connection

**Google Cloud Storage:**
```bash
DATAOPS_BACKUP_STORAGE_BACKEND=gcs
GCS_BUCKET=nself-backups
```

Similar pros/cons to S3.

### Compression Options

| Algorithm | Speed | Compression Ratio | CPU Usage |
|-----------|-------|-------------------|-----------|
| `none` | Fastest | 1.0x (no compression) | None |
| `gzip` | Fast | 2-3x | Low |
| `brotli` | Medium | 3-4x | Medium |
| `zstd` | Fast | 3-5x | Low-Medium |

**Recommendations:**
- **Fast backups:** `gzip` level 1-3
- **Balanced:** `gzip` level 6 (default)
- **Best compression:** `brotli` or `zstd` level 9
- **Large databases:** `zstd` (best compression/speed ratio)

### Backup Scheduling

**Cron Expression Format:**
```
┌───────────── minute (0-59)
│ ┌───────────── hour (0-23)
│ │ ┌───────────── day of month (1-31)
│ │ │ ┌───────────── month (1-12)
│ │ │ │ ┌───────────── day of week (0-6, Sunday = 0)
│ │ │ │ │
* * * * *
```

**Common Schedules:**
```bash
# Daily at 2 AM
0 2 * * *

# Every 6 hours
0 */6 * * *

# Weekly on Sunday at 3 AM
0 3 * * 0

# Monthly on 1st at 4 AM
0 4 1 * *

# Every weekday at midnight
0 0 * * 1-5
```

### Restore Strategies

**1. Full Restore:**
- Restore entire backup snapshot
- Overwrites existing data
- Use for disaster recovery

**2. Selective Restore:**
- Restore specific plugins or tables
- Preserves unaffected data
- Use for targeted recovery

**3. Point-in-Time Restore:**
- Restore to specific timestamp
- Requires backup at or before that time
- Use for data recovery after accidental deletion

**4. Dry-Run Restore:**
- Validate backup without restoring
- Check for corruption
- Estimate restore time

---

## Data Transformation

### Template Syntax

**Basic Field Mapping:**
```json
{
  "source": "stripe_customers",
  "target": "billing_users",
  "fields": {
    "id": "stripe_id",
    "email": "user_email",
    "name": "full_name"
  }
}
```

**Value Mapping:**
```json
{
  "field": "status",
  "type": "map",
  "mapping": {
    "active": "enabled",
    "past_due": "payment_required",
    "canceled": "disabled"
  },
  "default": "unknown"
}
```

**JSONB Extraction:**
```json
{
  "field": "metadata",
  "type": "jsonb_extract",
  "extract": {
    "metadata.plan": "subscription_plan",
    "metadata.referral": "referral_source"
  }
}
```

**Date Transformation:**
```json
{
  "field": "created_at",
  "type": "date",
  "input_format": "unix_timestamp",
  "output_format": "iso8601"
}
```

**Concatenation:**
```json
{
  "type": "concat",
  "fields": ["first_name", "last_name"],
  "separator": " ",
  "target": "full_name"
}
```

**Conditional Transformation:**
```json
{
  "type": "conditional",
  "condition": {
    "field": "amount",
    "operator": ">",
    "value": 1000
  },
  "then": {
    "field": "tier",
    "value": "premium"
  },
  "else": {
    "field": "tier",
    "value": "standard"
  }
}
```

### Custom Transformation Functions

**JavaScript Function:**
```javascript
function transform(record) {
  return {
    ...record,
    email_domain: record.email.split('@')[1],
    is_business: record.email.endsWith('.com') || record.email.endsWith('.io'),
    registration_year: new Date(record.created_at).getFullYear()
  };
}
```

**SQL Function:**
```sql
CREATE OR REPLACE FUNCTION transform_customer(record JSONB)
RETURNS JSONB AS $$
BEGIN
  RETURN jsonb_build_object(
    'user_id', record->>'id',
    'email', lower(record->>'email'),
    'is_verified', (record->>'email_verified')::boolean
  );
END;
$$ LANGUAGE plpgsql;
```

---

## Security & Verification

### Verification Codes

Verification codes protect sensitive operations (deletion requests).

**Properties:**
- 6 digits (configurable)
- Valid for limited time (default: 1 hour)
- Single-use (expires after verification)
- Sent via email
- Required for deletion execution

**Security Measures:**
- Rate limiting on verification attempts
- Account lockout after 5 failed attempts
- IP address logging
- Email notification on failed attempts

### API Authentication

**API Key Authentication:**
```bash
# Set API key
export DATAOPS_API_KEY=your_secret_key

# Use in requests
curl -H "Authorization: Bearer your_secret_key" \
  http://localhost:3306/api/exports
```

**JWT Authentication (optional):**
```bash
# Generate JWT token
export DATAOPS_JWT_SECRET=your_jwt_secret

# Use in requests
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..." \
  http://localhost:3306/api/exports
```

### Data Encryption

**At-Rest Encryption:**
```bash
# PostgreSQL encryption
# Enable at database level or use encrypted filesystem

# File encryption for exports
DATAOPS_ENCRYPT_EXPORTS=true
DATAOPS_ENCRYPTION_KEY=your_encryption_key
```

**In-Transit Encryption:**
```bash
# Force HTTPS for all endpoints
DATAOPS_FORCE_HTTPS=true
DATAOPS_SSL_CERT=/path/to/cert.pem
DATAOPS_SSL_KEY=/path/to/key.pem
```

### Access Control

**Role-Based Access Control:**
```sql
-- Create roles
CREATE ROLE dataops_admin;
CREATE ROLE dataops_operator;
CREATE ROLE dataops_viewer;

-- Grant permissions
GRANT ALL ON dataops_* TO dataops_admin;
GRANT SELECT, INSERT, UPDATE ON dataops_* TO dataops_operator;
GRANT SELECT ON dataops_* TO dataops_viewer;
```

### Audit Logging

All operations are logged in `dataops_transfer_audit` table:

```sql
-- View recent operations
SELECT * FROM dataops_transfer_audit
ORDER BY created_at DESC
LIMIT 100;

-- Suspicious activity
SELECT user_id, COUNT(*) AS attempt_count
FROM dataops_transfer_audit
WHERE operation = 'delete'
  AND status = 'failure'
  AND created_at > NOW() - INTERVAL '1 hour'
GROUP BY user_id
HAVING COUNT(*) >= 5;
```

---

## Troubleshooting

### Common Issues

#### "Export Failed: User Not Found"

```
Error: Export failed: User user_123 not found in any registered plugins
```

**Solutions:**
1. Verify user ID is correct
2. Check plugins are registered:
   ```bash
   nself plugin data-operations plugins list
   ```
3. Register missing plugins
4. Verify user exists in plugin tables

#### "Import Failed: Invalid CSV Format"

```
Error: Import failed: Invalid CSV format at line 42
```

**Solutions:**
1. Validate CSV format:
   ```bash
   head -n 50 /path/to/file.csv
   ```
2. Check delimiter matches (comma, semicolon, tab)
3. Ensure proper quoting for fields with delimiters
4. Check for encoding issues (UTF-8 expected)
5. Use `--dry-run` to validate before importing

#### "Deletion Request Expired"

```
Error: Verification code expired
```

**Solutions:**
1. Request new verification code
2. Increase cooldown period if needed
3. Ensure email was received promptly
4. Check email spam folder

#### "Migration Stuck"

```
Migration job job_abc123 stuck at 45% for 2 hours
```

**Solutions:**
1. Check job status:
   ```bash
   nself plugin data-operations migrate get job_abc123
   ```
2. Check database locks:
   ```sql
   SELECT * FROM pg_locks WHERE granted = false;
   ```
3. Pause and resume migration:
   ```bash
   nself plugin data-operations migrate pause job_abc123
   nself plugin data-operations migrate resume job_abc123
   ```
4. Cancel and restart with smaller batch size

#### "Backup Failed: Disk Space"

```
Error: Backup failed: No space left on device
```

**Solutions:**
1. Check disk space:
   ```bash
   df -h
   ```
2. Delete old backups:
   ```bash
   nself plugin data-operations backup list
   nself plugin data-operations backup delete snap_old123
   ```
3. Reduce backup retention period
4. Use higher compression level
5. Use cloud storage backend (S3, GCS)

#### "Restore Failed: Checksum Mismatch"

```
Error: Restore failed: Checksum mismatch for snapshot snap_abc123
```

**Solutions:**
1. Backup file corrupted during download
2. Download backup again
3. Verify checksum manually:
   ```bash
   sha256sum backup.tar.gz
   ```
4. If S3: check for partial download
5. Create new backup and retry

### Debug Mode

Enable debug logging:

```bash
# Environment variable
LOG_LEVEL=debug nself plugin data-operations server

# CLI flag
nself plugin data-operations server --log-level debug

# Runtime
curl -X POST http://localhost:3306/api/debug/enable
```

### Health Checks

```bash
# Plugin health
nself plugin data-operations status

# Database connectivity
curl http://localhost:3306/health

# Detailed status
curl http://localhost:3306/api/status
```

### Performance Issues

**Slow Exports:**
```bash
# Use compression
DATAOPS_COMPRESSION_ALGORITHM=gzip

# Reduce max export size
DATAOPS_MAX_EXPORT_SIZE_MB=100

# Check for large JSONB fields
SELECT id, pg_column_size(metadata) AS size_bytes
FROM stripe_customers
ORDER BY size_bytes DESC
LIMIT 10;
```

**Slow Imports:**
```bash
# Increase batch size
DATAOPS_MIGRATION_BATCH_SIZE=500

# Disable indexes during import
# Re-enable after completion
```

**Slow Migrations:**
```bash
# Reduce batch size if high load
DATAOPS_MIGRATION_BATCH_SIZE=50

# Add rate limiting
DATAOPS_MIGRATION_RATE_LIMIT_MS=100

# Check database indexes
# Add indexes on foreign key fields
```

---

## Performance

### Optimization Tips

**1. Batch Size Tuning:**
```bash
# Small batches (low memory, slower)
DATAOPS_MIGRATION_BATCH_SIZE=50

# Large batches (high memory, faster)
DATAOPS_MIGRATION_BATCH_SIZE=500

# Find optimal value for your dataset
```

**2. Compression Trade-offs:**
```bash
# Fast backup, larger files
DATAOPS_COMPRESSION_ALGORITHM=gzip
DATAOPS_COMPRESSION_LEVEL=1

# Balanced
DATAOPS_COMPRESSION_ALGORITHM=gzip
DATAOPS_COMPRESSION_LEVEL=6

# Best compression, slower
DATAOPS_COMPRESSION_ALGORITHM=brotli
DATAOPS_COMPRESSION_LEVEL=9
```

**3. Concurrent Processing:**
```bash
# More workers = faster processing
DATAOPS_QUEUE_CONCURRENCY=10

# Limited resources = fewer workers
DATAOPS_QUEUE_CONCURRENCY=2
```

**4. Database Indexes:**
```sql
-- Add indexes for common queries
CREATE INDEX idx_export_user_status
    ON dataops_export_requests(user_id, status);

CREATE INDEX idx_audit_user_created
    ON dataops_transfer_audit(user_id, created_at DESC);

-- Analyze tables
ANALYZE dataops_export_requests;
ANALYZE dataops_import_requests;
```

### Benchmarks

**Export Performance:**
- 10K records: ~5 seconds
- 100K records: ~30 seconds
- 1M records: ~5 minutes

**Import Performance:**
- 10K records: ~10 seconds
- 100K records: ~60 seconds
- 1M records: ~10 minutes

**Backup Performance:**
- 1GB database: ~30 seconds (gzip level 6)
- 10GB database: ~5 minutes (gzip level 6)
- 100GB database: ~45 minutes (gzip level 6)

**Migration Performance:**
- 10K records: ~15 seconds
- 100K records: ~2 minutes
- 1M records: ~15 minutes

*Benchmarks on: PostgreSQL 14, 8 CPU cores, 32GB RAM, SSD storage*

---

## Integration Examples

### Express.js Integration

```javascript
const express = require('express');
const axios = require('axios');

const app = express();

// Handle GDPR export request
app.post('/api/user/export', async (req, res) => {
  const { userId } = req.user;

  try {
    // Create export via data-operations API
    const response = await axios.post('http://localhost:3306/api/exports', {
      user_id: userId,
      format: 'json',
      email: req.user.email,
      reason: 'User requested data export'
    }, {
      headers: { 'Authorization': `Bearer ${process.env.DATAOPS_API_KEY}` }
    });

    res.json({
      message: 'Export request created',
      request_id: response.data.id
    });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// Handle GDPR deletion request
app.post('/api/user/delete', async (req, res) => {
  const { userId } = req.user;

  try {
    const response = await axios.post('http://localhost:3306/api/deletions', {
      user_id: userId,
      email: req.user.email,
      reason: 'User requested account deletion'
    }, {
      headers: { 'Authorization': `Bearer ${process.env.DATAOPS_API_KEY}` }
    });

    res.json({
      message: 'Deletion request created. Check email for verification code.',
      request_id: response.data.id
    });
  } catch (error) {
    res.status(500).json({ error: error.message });
  }
});

// Verify deletion
app.post('/api/user/delete/verify', async (req, res) => {
  const { requestId, code } = req.body;

  try {
    await axios.post(`http://localhost:3306/api/deletions/${requestId}/verify`, {
      code
    }, {
      headers: { 'Authorization': `Bearer ${process.env.DATAOPS_API_KEY}` }
    });

    res.json({ message: 'Deletion verified and scheduled' });
  } catch (error) {
    res.status(400).json({ error: error.message });
  }
});
```

### Python Integration

```python
import requests
import os

class DataOperationsClient:
    def __init__(self, base_url='http://localhost:3306', api_key=None):
        self.base_url = base_url
        self.api_key = api_key or os.getenv('DATAOPS_API_KEY')
        self.headers = {
            'Authorization': f'Bearer {self.api_key}',
            'Content-Type': 'application/json'
        }

    def create_export(self, user_id, email, format='json', plugins=None):
        """Create GDPR export request"""
        response = requests.post(
            f'{self.base_url}/api/exports',
            json={
                'user_id': user_id,
                'email': email,
                'format': format,
                'plugins': plugins or [],
                'reason': 'User requested data export'
            },
            headers=self.headers
        )
        response.raise_for_status()
        return response.json()

    def create_deletion(self, user_id, email, plugins=None):
        """Create GDPR deletion request"""
        response = requests.post(
            f'{self.base_url}/api/deletions',
            json={
                'user_id': user_id,
                'email': email,
                'plugins': plugins or [],
                'reason': 'User requested account deletion'
            },
            headers=self.headers
        )
        response.raise_for_status()
        return response.json()

    def verify_deletion(self, request_id, code):
        """Verify deletion with code"""
        response = requests.post(
            f'{self.base_url}/api/deletions/{request_id}/verify',
            json={'code': code},
            headers=self.headers
        )
        response.raise_for_status()
        return response.json()

    def create_backup(self, name, description='', plugins=None):
        """Create manual backup"""
        response = requests.post(
            f'{self.base_url}/api/backups',
            json={
                'name': name,
                'description': description,
                'plugins': plugins or ['*'],
                'compression': 'gzip',
                'compression_level': 6
            },
            headers=self.headers
        )
        response.raise_for_status()
        return response.json()

# Usage
client = DataOperationsClient()

# Export user data
export = client.create_export(
    user_id='user_123',
    email='user@example.com',
    format='json'
)
print(f"Export request created: {export['id']}")

# Delete user data
deletion = client.create_deletion(
    user_id='user_123',
    email='user@example.com'
)
print(f"Deletion request created: {deletion['id']}")

# Verify deletion
client.verify_deletion(deletion['id'], '123456')
print("Deletion verified")
```

### React Frontend

```typescript
import React, { useState } from 'react';
import axios from 'axios';

const DATAOPS_API = 'http://localhost:3306';

interface ExportButtonProps {
  userId: string;
  userEmail: string;
}

export const ExportDataButton: React.FC<ExportButtonProps> = ({ userId, userEmail }) => {
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState('');

  const handleExport = async () => {
    setLoading(true);
    try {
      const response = await axios.post(`${DATAOPS_API}/api/exports`, {
        user_id: userId,
        email: userEmail,
        format: 'json',
        reason: 'User requested data export'
      }, {
        headers: {
          'Authorization': `Bearer ${process.env.REACT_APP_DATAOPS_API_KEY}`
        }
      });

      setMessage(`Export request created! Request ID: ${response.data.id}. Check your email for download link.`);
    } catch (error) {
      setMessage(`Error: ${error.response?.data?.error || error.message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <button onClick={handleExport} disabled={loading}>
        {loading ? 'Creating export...' : 'Export My Data'}
      </button>
      {message && <p>{message}</p>}
    </div>
  );
};

interface DeleteAccountProps {
  userId: string;
  userEmail: string;
}

export const DeleteAccountButton: React.FC<DeleteAccountProps> = ({ userId, userEmail }) => {
  const [step, setStep] = useState<'initial' | 'verify'>('initial');
  const [requestId, setRequestId] = useState('');
  const [code, setCode] = useState('');
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState('');

  const handleRequestDeletion = async () => {
    setLoading(true);
    try {
      const response = await axios.post(`${DATAOPS_API}/api/deletions`, {
        user_id: userId,
        email: userEmail,
        reason: 'User requested account deletion'
      }, {
        headers: {
          'Authorization': `Bearer ${process.env.REACT_APP_DATAOPS_API_KEY}`
        }
      });

      setRequestId(response.data.id);
      setMessage('Verification code sent to your email. Please check your inbox.');
      setStep('verify');
    } catch (error) {
      setMessage(`Error: ${error.response?.data?.error || error.message}`);
    } finally {
      setLoading(false);
    }
  };

  const handleVerifyDeletion = async () => {
    setLoading(true);
    try {
      await axios.post(`${DATAOPS_API}/api/deletions/${requestId}/verify`, {
        code
      }, {
        headers: {
          'Authorization': `Bearer ${process.env.REACT_APP_DATAOPS_API_KEY}`
        }
      });

      setMessage('Deletion verified! Your account will be deleted within 24 hours.');
    } catch (error) {
      setMessage(`Error: ${error.response?.data?.error || error.message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      {step === 'initial' ? (
        <button onClick={handleRequestDeletion} disabled={loading}>
          {loading ? 'Processing...' : 'Delete My Account'}
        </button>
      ) : (
        <div>
          <input
            type="text"
            placeholder="Enter verification code"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            maxLength={6}
          />
          <button onClick={handleVerifyDeletion} disabled={loading || code.length !== 6}>
            {loading ? 'Verifying...' : 'Verify Deletion'}
          </button>
        </div>
      )}
      {message && <p>{message}</p>}
    </div>
  );
};
```

---

## Support

- **GitHub Issues:** [nself-plugins/issues](https://github.com/acamarata/nself-plugins/issues)
- **Plugin Documentation:** [github.com/acamarata/nself-plugins/wiki/Data-Operations](https://github.com/acamarata/nself-plugins/wiki/Data-Operations)
- **GDPR Resources:** [gdpr.eu](https://gdpr.eu/)
- **PostgreSQL Documentation:** [postgresql.org/docs](https://www.postgresql.org/docs/)

---

*Last Updated: February 11, 2026*
*Plugin Version: 1.0.0*
*nself Version: 0.4.8+*
