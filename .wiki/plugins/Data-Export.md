# Data Export Plugin

GDPR-compliant data export, deletion, and import system for user data and plugin data with verification codes and audit trails for nself applications.

## Overview

The Data Export plugin provides comprehensive tools for managing user data in compliance with GDPR and privacy regulations. It supports full data exports, secure data deletion, data import, and plugin registration for automated data discovery.

### Key Features

- **GDPR Compliance**: Full support for data export and deletion requests
- **User Data Export**: Export all user data in JSON format
- **Plugin Data Export**: Automatically discover and export data from all plugins
- **Secure Deletion**: Verified data deletion with cooldown periods
- **Data Import**: Import previously exported data
- **Plugin Registry**: Auto-discovery of tables and data sources
- **Verification Codes**: Secure verification for sensitive operations
- **Download Links**: Temporary, expiring download links
- **Audit Trail**: Complete logging of all export and deletion requests
- **Multi-Format Support**: JSON, CSV export formats
- **Chunked Exports**: Handle large datasets efficiently
- **Multi-App Support**: Isolated exports per source account

### Use Cases

- **GDPR Compliance**: Respond to data subject access requests
- **Data Portability**: Allow users to take their data elsewhere
- **Account Deletion**: Secure, verified account deletion
- **Data Migration**: Export and import data between systems
- **Backup & Restore**: Create user data backups
- **Audit & Compliance**: Track all data operations
- **Platform Migration**: Move user data to new systems
- **Account Transfer**: Transfer data between accounts

---

## Quick Start

### Installation

```bash
# Install the plugin
nself plugin install data-export

# Initialize database schema
nself data-export init

# Start the server
nself data-export server
```

### Basic Usage

```bash
# Request user data export
curl -X POST http://localhost:3306/v1/export/request \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "email": "user@example.com",
    "include_plugins": ["chat", "cms", "social"]
  }'

# Check export status
curl http://localhost:3306/v1/export/requests/request-id

# Download export
curl http://localhost:3306/v1/export/download/request-id?code=verification-code

# Request data deletion
curl -X POST http://localhost:3306/v1/deletion/request \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "email": "user@example.com",
    "reason": "GDPR request"
  }'

# Check status
nself data-export status
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `EXPORT_PLUGIN_PORT` | No | `3306` | HTTP server port |
| `EXPORT_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| `EXPORT_STORAGE_PATH` | No | `/tmp/nself-exports` | Export file storage path |
| `EXPORT_DOWNLOAD_EXPIRY_HOURS` | No | `24` | Hours until download link expires |
| `EXPORT_DELETION_COOLDOWN_HOURS` | No | `24` | Cooldown before deletion executes |
| `EXPORT_MAX_EXPORT_SIZE_MB` | No | `500` | Maximum export file size (MB) |
| `EXPORT_VERIFICATION_CODE_LENGTH` | No | `6` | Verification code length |
| `EXPORT_API_KEY` | No | - | API key for authentication |
| `EXPORT_RATE_LIMIT_MAX` | No | `200` | Max requests per window |
| `EXPORT_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (ms) |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | - | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `LOG_LEVEL` | No | `info` | Logging level |

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
EXPORT_PLUGIN_PORT=3306
EXPORT_STORAGE_PATH=/data/exports
EXPORT_DOWNLOAD_EXPIRY_HOURS=48
EXPORT_DELETION_COOLDOWN_HOURS=72
EXPORT_MAX_EXPORT_SIZE_MB=1000
EXPORT_API_KEY=your-secret-key
```

---

## CLI Commands

### `init`
Initialize the database schema.

```bash
nself data-export init
```

### `server`
Start the HTTP API server.

```bash
nself data-export server [options]

Options:
  -p, --port <port>    Server port (default: 3306)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

### `export`
Export user or plugin data.

```bash
nself data-export export <userId> [options]

Options:
  --email <email>      User email for verification
  --plugins <list>     Comma-separated plugin list
  --format <format>    Export format (json, csv)
```

**Example:**
```bash
nself data-export export user123 \
  --email user@example.com \
  --plugins chat,cms,social \
  --format json
```

### `delete`
Request GDPR data deletion.

```bash
nself data-export delete <userId> [options]

Options:
  --email <email>      User email for verification
  --reason <reason>    Deletion reason
  --verify             Skip cooldown (use verification code)
```

**Example:**
```bash
nself data-export delete user123 \
  --email user@example.com \
  --reason "GDPR request"
```

### `import`
Import data from export file.

```bash
nself data-export import <filePath> [options]

Options:
  --user <userId>      Target user ID
  --merge              Merge with existing data
  --overwrite          Overwrite existing data
```

### `plugins`
Manage plugin registry for export/deletion.

```bash
nself data-export plugins [command]

Commands:
  list                    List registered plugins
  register <plugin>       Register plugin
  unregister <plugin>     Unregister plugin
  refresh                 Refresh plugin discovery
```

### `stats`
View export and deletion statistics.

```bash
nself data-export stats
```

**Output:**
```
Data Export Statistics
======================
Total Export Requests:     523
Completed Exports:         498
Failed Exports:            12
Pending Exports:           13
Total Deletion Requests:   89
Completed Deletions:       76
Pending Deletions:         13
Registered Plugins:        15
Total Data Exported (GB):  234.5
Average Export Size (MB):  470.3
```

---

## REST API

All endpoints support multi-app isolation via `X-Source-Account-Id` header.

### Health & Status

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "data-export",
  "timestamp": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/status`
Plugin status and statistics.

**Response:**
```json
{
  "plugin": "data-export",
  "version": "1.0.0",
  "status": "running",
  "config": {
    "storagePath": "/data/exports",
    "downloadExpiryHours": 24,
    "deletionCooldownHours": 24,
    "maxExportSizeMB": 500,
    "verificationCodeLength": 6
  },
  "stats": {
    "totalExportRequests": 523,
    "completedExports": 498,
    "failedExports": 12,
    "pendingExports": 13,
    "totalDeletionRequests": 89,
    "completedDeletions": 76,
    "pendingDeletions": 13,
    "registeredPlugins": 15,
    "totalDataExportedGB": 234.5,
    "averageExportSizeMB": 470.3
  },
  "timestamp": "2026-02-11T10:30:00Z"
}
```

### Export Requests

#### `POST /v1/export/request`
Request a data export.

**Request:**
```json
{
  "user_id": "user123",
  "email": "user@example.com",
  "include_plugins": ["chat", "cms", "social", "activity-feed"],
  "format": "json",
  "include_metadata": true,
  "callback_url": "https://example.com/export-complete"
}
```

**Response:**
```json
{
  "request_id": "export-uuid",
  "user_id": "user123",
  "status": "pending",
  "verification_code": "ABC123",
  "verification_sent_to": "user@example.com",
  "estimated_size_mb": 450,
  "estimated_completion": "2026-02-11T10:35:00Z",
  "expires_at": "2026-02-12T10:30:00Z",
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/export/requests/:requestId`
Check export request status.

**Response:**
```json
{
  "request_id": "export-uuid",
  "user_id": "user123",
  "status": "completed",
  "format": "json",
  "file_size_mb": 478.3,
  "download_url": "/v1/export/download/export-uuid",
  "expires_at": "2026-02-12T10:30:00Z",
  "plugins_included": ["chat", "cms", "social", "activity-feed"],
  "tables_exported": 47,
  "records_exported": 125678,
  "created_at": "2026-02-11T10:30:00Z",
  "completed_at": "2026-02-11T10:33:00Z"
}
```

#### `GET /v1/export/requests`
List export requests.

**Query Parameters:**
- `user_id`: Filter by user
- `status`: Filter by status (pending, processing, completed, failed)
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "request_id": "export-uuid",
      "user_id": "user123",
      "status": "completed",
      "file_size_mb": 478.3,
      "created_at": "2026-02-11T10:30:00Z",
      "completed_at": "2026-02-11T10:33:00Z"
    }
  ],
  "total": 523,
  "limit": 50,
  "offset": 0
}
```

#### `GET /v1/export/download/:requestId`
Download exported data.

**Query Parameters:**
- `code`: Verification code (required)

**Response:**
- File download (application/json or application/zip)

**Example:**
```bash
curl -o export.json "http://localhost:3306/v1/export/download/export-uuid?code=ABC123"
```

#### `DELETE /v1/export/requests/:requestId`
Cancel or delete an export request.

**Response:**
```json
{
  "success": true,
  "request_id": "export-uuid",
  "deleted": true
}
```

### Deletion Requests

#### `POST /v1/deletion/request`
Request data deletion (GDPR right to be forgotten).

**Request:**
```json
{
  "user_id": "user123",
  "email": "user@example.com",
  "reason": "GDPR right to be forgotten request",
  "include_plugins": ["chat", "cms", "social"],
  "immediate": false,
  "callback_url": "https://example.com/deletion-complete"
}
```

**Response:**
```json
{
  "request_id": "deletion-uuid",
  "user_id": "user123",
  "status": "pending",
  "verification_code": "DEF456",
  "verification_sent_to": "user@example.com",
  "cooldown_ends_at": "2026-02-12T10:30:00Z",
  "estimated_records": 125678,
  "plugins_affected": ["chat", "cms", "social", "activity-feed"],
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `POST /v1/deletion/verify/:requestId`
Verify and execute deletion request.

**Request:**
```json
{
  "verification_code": "DEF456"
}
```

**Response:**
```json
{
  "request_id": "deletion-uuid",
  "status": "scheduled",
  "execution_at": "2026-02-12T10:30:00Z",
  "message": "Deletion will execute after cooldown period"
}
```

#### `GET /v1/deletion/requests/:requestId`
Check deletion request status.

**Response:**
```json
{
  "request_id": "deletion-uuid",
  "user_id": "user123",
  "status": "completed",
  "reason": "GDPR request",
  "plugins_affected": ["chat", "cms", "social"],
  "tables_affected": 47,
  "records_deleted": 125678,
  "verified_at": "2026-02-11T10:35:00Z",
  "executed_at": "2026-02-12T10:30:00Z",
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/deletion/requests`
List deletion requests.

**Query Parameters:**
- `user_id`: Filter by user
- `status`: Filter by status (pending, verified, scheduled, processing, completed, failed)
- `limit`: Results per page (default: 50)

**Response:**
```json
{
  "data": [
    {
      "request_id": "deletion-uuid",
      "user_id": "user123",
      "status": "completed",
      "records_deleted": 125678,
      "executed_at": "2026-02-12T10:30:00Z"
    }
  ],
  "total": 89
}
```

#### `DELETE /v1/deletion/requests/:requestId`
Cancel a deletion request (before execution).

**Response:**
```json
{
  "success": true,
  "request_id": "deletion-uuid",
  "cancelled": true
}
```

### Import

#### `POST /v1/import`
Import data from export file.

**Request (multipart/form-data):**
- `file`: Export file (JSON or ZIP)
- `user_id`: Target user ID
- `merge`: Merge with existing data (boolean)
- `overwrite`: Overwrite existing data (boolean)

**Response:**
```json
{
  "import_id": "import-uuid",
  "status": "processing",
  "file_size_mb": 478.3,
  "estimated_records": 125678,
  "estimated_completion": "2026-02-11T10:40:00Z",
  "created_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/import/:importId`
Check import status.

**Response:**
```json
{
  "import_id": "import-uuid",
  "user_id": "user123",
  "status": "completed",
  "file_size_mb": 478.3,
  "records_imported": 125678,
  "tables_affected": 47,
  "errors": [],
  "warnings": ["Skipped 3 duplicate records"],
  "created_at": "2026-02-11T10:30:00Z",
  "completed_at": "2026-02-11T10:38:00Z"
}
```

### Plugin Registry

#### `POST /v1/plugins/register`
Register a plugin for data export/deletion.

**Request:**
```json
{
  "plugin_name": "chat",
  "tables": [
    {
      "name": "chat_conversations",
      "user_column": "created_by",
      "type": "primary"
    },
    {
      "name": "chat_messages",
      "user_column": "sender_id",
      "type": "primary"
    },
    {
      "name": "chat_participants",
      "user_column": "user_id",
      "type": "relation"
    }
  ],
  "export_order": 1,
  "deletion_order": 10
}
```

**Response:**
```json
{
  "plugin_name": "chat",
  "registered": true,
  "tables_count": 3,
  "export_order": 1,
  "deletion_order": 10,
  "registered_at": "2026-02-11T10:30:00Z"
}
```

#### `GET /v1/plugins`
List registered plugins.

**Response:**
```json
{
  "data": [
    {
      "plugin_name": "chat",
      "tables_count": 6,
      "export_order": 1,
      "deletion_order": 10,
      "last_used": "2026-02-11T09:00:00Z",
      "registered_at": "2026-02-10T08:00:00Z"
    }
  ],
  "total": 15
}
```

#### `DELETE /v1/plugins/:pluginName`
Unregister a plugin.

**Response:**
```json
{
  "success": true,
  "plugin_name": "chat",
  "unregistered": true
}
```

---

## Database Schema

### `export_requests`
```sql
CREATE TABLE export_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  email VARCHAR(255),
  status VARCHAR(32) DEFAULT 'pending',
  format VARCHAR(16) DEFAULT 'json',
  file_path TEXT,
  file_size_mb DECIMAL(10,2),
  verification_code VARCHAR(32),
  download_count INTEGER DEFAULT 0,
  include_plugins TEXT[],
  tables_exported INTEGER DEFAULT 0,
  records_exported INTEGER DEFAULT 0,
  error TEXT,
  expires_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'expired'))
);
```

### `export_deletion_requests`
```sql
CREATE TABLE export_deletion_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  email VARCHAR(255),
  status VARCHAR(32) DEFAULT 'pending',
  reason TEXT,
  verification_code VARCHAR(32),
  verified_at TIMESTAMP WITH TIME ZONE,
  include_plugins TEXT[],
  tables_affected INTEGER DEFAULT 0,
  records_deleted INTEGER DEFAULT 0,
  cooldown_ends_at TIMESTAMP WITH TIME ZONE,
  executed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (status IN ('pending', 'verified', 'scheduled', 'processing', 'completed', 'failed', 'cancelled'))
);
```

### `export_plugin_registry`
```sql
CREATE TABLE export_plugin_registry (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  plugin_name VARCHAR(255) NOT NULL,
  tables JSONB DEFAULT '[]',
  export_order INTEGER DEFAULT 0,
  deletion_order INTEGER DEFAULT 0,
  enabled BOOLEAN DEFAULT true,
  last_used TIMESTAMP WITH TIME ZONE,
  registered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, plugin_name)
);
```

### `export_import_jobs`
```sql
CREATE TABLE export_import_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  status VARCHAR(32) DEFAULT 'pending',
  file_size_mb DECIMAL(10,2),
  records_imported INTEGER DEFAULT 0,
  tables_affected INTEGER DEFAULT 0,
  merge_mode BOOLEAN DEFAULT false,
  errors JSONB DEFAULT '[]',
  warnings JSONB DEFAULT '[]',
  completed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);
```

### `export_webhook_events`
```sql
CREATE TABLE export_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  payload JSONB NOT NULL,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Examples

### Example 1: GDPR Export Request

```bash
# User requests their data
curl -X POST http://localhost:3306/v1/export/request \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "email": "user@example.com",
    "include_plugins": ["chat", "cms", "social"],
    "format": "json"
  }'

# System sends verification code to email
# User checks status
curl http://localhost:3306/v1/export/requests/export-uuid

# User downloads with verification code
curl -o my-data.json \
  "http://localhost:3306/v1/export/download/export-uuid?code=ABC123"
```

### Example 2: Account Deletion

```bash
# User requests account deletion
curl -X POST http://localhost:3306/v1/deletion/request \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "email": "user@example.com",
    "reason": "Closing my account"
  }'

# System sends verification code
# User verifies request
curl -X POST http://localhost:3306/v1/deletion/verify/deletion-uuid \
  -H "Content-Type: application/json" \
  -d '{"verification_code": "DEF456"}'

# After cooldown period, deletion executes automatically
```

### Example 3: Plugin Registration

```bash
# Register custom plugin for exports
curl -X POST http://localhost:3306/v1/plugins/register \
  -H "Content-Type: application/json" \
  -d '{
    "plugin_name": "my-custom-plugin",
    "tables": [
      {
        "name": "custom_data",
        "user_column": "user_id",
        "type": "primary"
      }
    ],
    "export_order": 5,
    "deletion_order": 5
  }'
```

### Example 4: Data Migration

```bash
# Export from old system
curl -X POST http://localhost:3306/v1/export/request \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "email": "user@example.com",
    "include_plugins": ["all"]
  }'

# Download export
curl -o export.json \
  "http://localhost:3306/v1/export/download/export-uuid?code=ABC123"

# Import to new system
curl -X POST http://localhost:3306/v1/import \
  -F "file=@export.json" \
  -F "user_id=user123" \
  -F "merge=true"
```

### Example 5: Bulk User Export

```bash
# Request exports for multiple users
for user_id in user1 user2 user3; do
  curl -X POST http://localhost:3306/v1/export/request \
    -H "Content-Type: application/json" \
    -d "{
      \"user_id\": \"$user_id\",
      \"email\": \"${user_id}@example.com\"
    }"
done

# Check all exports
curl http://localhost:3306/v1/export/requests?status=completed
```

---

## Troubleshooting

### Export Taking Too Long

**Solution:**
- Check `EXPORT_MAX_EXPORT_SIZE_MB` limit
- Reduce `include_plugins` list
- Export in multiple smaller batches
- Check database performance
- Increase processing resources

### Download Link Expired

**Solution:**
- Adjust `EXPORT_DOWNLOAD_EXPIRY_HOURS`
- Request new export if expired
- Check system time settings

### Deletion Not Executing

**Solution:**
- Verify cooldown period has passed
- Check verification code was submitted
- Review deletion request status
- Check for errors in logs
- Ensure no FK constraints blocking deletion

### Storage Space Issues

**Solution:**
```bash
# Clean up old exports
find /tmp/nself-exports -mtime +7 -delete

# Adjust storage path
export EXPORT_STORAGE_PATH=/data/large-disk/exports
```

---

## License

Source-Available License

## Support

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Homepage: https://github.com/acamarata/nself-plugins/tree/main/plugins/data-export
