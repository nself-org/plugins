# Object Storage Plugin

Multi-provider object storage with S3-compatible API, local storage, presigned URLs, and multipart uploads.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Storage Providers](#storage-providers)
- [TypeScript Implementation](#typescript-implementation)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Object Storage plugin provides unified object storage across multiple providers with S3-compatible API. It supports:

- **5 Database Tables** - Buckets, objects, uploads, access logs
- **7 Webhook Events** - Real-time storage events
- **Multi-Provider** - Local, S3, MinIO, R2, GCS, B2, Azure
- **Presigned URLs** - Time-limited upload/download URLs
- **Multipart Uploads** - Efficient large file handling
- **Full REST API** - Complete storage operations
- **CLI Interface** - Command-line bucket and object management

### Supported Providers

| Provider | Type | Description |
|----------|------|-------------|
| local | Filesystem | Local disk storage |
| s3 | Cloud | AWS S3 |
| minio | Self-hosted | MinIO object storage |
| r2 | Cloud | Cloudflare R2 |
| gcs | Cloud | Google Cloud Storage |
| b2 | Cloud | Backblaze B2 |
| azure | Cloud | Azure Blob Storage (planned) |

---

## Quick Start

```bash
# Install the plugin
nself plugin install object-storage

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "OS_DEFAULT_PROVIDER=local" >> .env
echo "OS_STORAGE_BASE_PATH=/data/storage" >> .env

# Initialize database schema
nself plugin object-storage init

# Start server
nself plugin object-storage server --port 3301
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `OS_PLUGIN_PORT` | No | `3301` | HTTP server port |
| `OS_STORAGE_BASE_PATH` | Yes (local) | `/data/object-storage` | Local storage directory |
| `OS_DEFAULT_PROVIDER` | No | `local` | Default storage provider |
| `OS_S3_ENDPOINT` | No | - | S3 endpoint URL |
| `OS_S3_REGION` | No | `us-east-1` | S3 region |
| `OS_S3_ACCESS_KEY` | Yes (S3) | - | S3 access key ID |
| `OS_S3_SECRET_KEY` | Yes (S3) | - | S3 secret access key |
| `OS_S3_BUCKET_PREFIX` | No | - | Bucket name prefix |
| `OS_PRESIGN_EXPIRY_SECONDS` | No | `3600` | Presigned URL expiry (1 hour) |
| `OS_MAX_UPLOAD_SIZE` | No | `1073741824` | Max upload size (1GB) |
| `OS_MULTIPART_THRESHOLD` | No | `104857600` | Multipart threshold (100MB) |
| `OS_API_KEY` | No | - | API key for authentication |
| `OS_RATE_LIMIT_MAX` | No | `200` | Max API requests per window |
| `OS_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (milliseconds) |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env Files

#### Local Storage

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Local Storage
OS_DEFAULT_PROVIDER=local
OS_STORAGE_BASE_PATH=/data/object-storage

# Limits
OS_MAX_UPLOAD_SIZE=10737418240  # 10GB
OS_MULTIPART_THRESHOLD=104857600  # 100MB
OS_PRESIGN_EXPIRY_SECONDS=3600

# Server
OS_PLUGIN_PORT=3301
LOG_LEVEL=info
```

#### AWS S3

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# AWS S3
OS_DEFAULT_PROVIDER=s3
OS_S3_REGION=us-east-1
OS_S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
OS_S3_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
OS_S3_BUCKET_PREFIX=myapp-

# Server
OS_PLUGIN_PORT=3301
```

#### MinIO

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# MinIO
OS_DEFAULT_PROVIDER=minio
OS_S3_ENDPOINT=http://localhost:9000
OS_S3_REGION=us-east-1
OS_S3_ACCESS_KEY=minioadmin
OS_S3_SECRET_KEY=minioadmin

# Server
OS_PLUGIN_PORT=3301
```

#### Cloudflare R2

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Cloudflare R2
OS_DEFAULT_PROVIDER=r2
OS_S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
OS_S3_REGION=auto
OS_S3_ACCESS_KEY=<r2-access-key>
OS_S3_SECRET_KEY=<r2-secret-key>

# Server
OS_PLUGIN_PORT=3301
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin object-storage init

# Start server
nself plugin object-storage server

# Custom port
nself plugin object-storage server --port 8080

# Check status
nself plugin object-storage status

# View usage statistics
nself plugin object-storage usage
```

### Bucket Management

```bash
# List all buckets
nself plugin object-storage buckets list

# Create bucket
nself plugin object-storage buckets create my-bucket

# Create with provider
nself plugin object-storage buckets create my-bucket --provider s3

# Get bucket info
nself plugin object-storage buckets get my-bucket

# Delete bucket (must be empty)
nself plugin object-storage buckets delete my-bucket
```

### Object Management

```bash
# List objects in bucket
nself plugin object-storage objects list my-bucket

# List with prefix filter
nself plugin object-storage objects list my-bucket --prefix folder/

# Upload file
nself plugin object-storage upload my-bucket /path/to/file.txt object-key

# Upload to subfolder
nself plugin object-storage upload my-bucket file.txt folder/file.txt

# Download file
nself plugin object-storage download my-bucket object-key /path/to/save.txt

# Delete object
nself plugin object-storage objects delete my-bucket object-key

# Get object metadata
nself plugin object-storage objects get my-bucket object-key
```

### Presigned URLs

```bash
# Generate presigned upload URL
nself plugin object-storage presign upload my-bucket object-key

# Custom expiry (seconds)
nself plugin object-storage presign upload my-bucket object-key --expiry 7200

# Generate presigned download URL
nself plugin object-storage presign download my-bucket object-key

# Custom expiry
nself plugin object-storage presign download my-bucket object-key --expiry 300
```

### Lifecycle Rules

```bash
# List lifecycle rules
nself plugin object-storage lifecycle list my-bucket

# Add expiration rule
nself plugin object-storage lifecycle add my-bucket \
  --prefix temp/ \
  --expiration-days 7

# Delete lifecycle rule
nself plugin object-storage lifecycle delete my-bucket rule-id
```

---

## REST API

### Health & Status

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "object-storage",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "object-storage",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /live
Liveness check with statistics.

**Response:**
```json
{
  "alive": true,
  "plugin": "object-storage",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 52428800
  },
  "stats": {
    "buckets": 5,
    "objects": 1250,
    "totalSizeBytes": 10737418240
  },
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Buckets

#### POST /v1/buckets
Create new bucket.

**Request Body:**
```json
{
  "name": "my-bucket",
  "provider": "local",
  "region": "us-east-1",
  "public": false
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "source_account_id": "primary",
    "name": "my-bucket",
    "provider": "local",
    "region": "us-east-1",
    "public": false,
    "versioning_enabled": false,
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### GET /v1/buckets
List all buckets.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "name": "my-bucket",
      "provider": "local",
      "object_count": 150,
      "total_size_bytes": 524288000
    }
  ]
}
```

#### GET /v1/buckets/:name
Get bucket details.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "name": "my-bucket",
    "provider": "local",
    "region": "us-east-1",
    "public": false,
    "versioning_enabled": false,
    "lifecycle_rules": [],
    "cors_rules": [],
    "object_count": 150,
    "total_size_bytes": 524288000,
    "created_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### PUT /v1/buckets/:name
Update bucket configuration.

**Request Body:**
```json
{
  "public": true,
  "versioning_enabled": true
}
```

#### DELETE /v1/buckets/:name
Delete bucket (must be empty).

**Response:**
```json
{
  "success": true
}
```

### Objects

#### POST /v1/buckets/:bucket/objects
Upload object.

**Form Data:**
- `file` - File to upload
- `key` - Object key (optional, defaults to filename)
- `content_type` - MIME type (optional)
- `metadata` - JSON metadata (optional)

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "bucket": "my-bucket",
    "key": "folder/file.txt",
    "size_bytes": 1024,
    "content_type": "text/plain",
    "etag": "5d41402abc4b2a76b9719d911017c592",
    "uploaded_at": "2026-02-11T10:00:00.000Z"
  }
}
```

#### GET /v1/buckets/:bucket/objects
List objects in bucket.

**Query Parameters:**
- `prefix` (optional) - Filter by key prefix
- `delimiter` (optional) - Delimiter for hierarchical listing
- `limit` (optional) - Max results (default: 1000)
- `marker` (optional) - Pagination marker

**Response:**
```json
{
  "success": true,
  "data": {
    "objects": [
      {
        "id": "uuid",
        "key": "folder/file.txt",
        "size_bytes": 1024,
        "content_type": "text/plain",
        "last_modified": "2026-02-11T10:00:00.000Z"
      }
    ],
    "prefixes": ["folder/", "images/"],
    "truncated": false,
    "next_marker": null
  }
}
```

#### GET /v1/buckets/:bucket/objects/:key
Get object metadata.

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "bucket": "my-bucket",
    "key": "folder/file.txt",
    "size_bytes": 1024,
    "content_type": "text/plain",
    "etag": "5d41402abc4b2a76b9719d911017c592",
    "metadata": {},
    "last_modified": "2026-02-11T10:00:00.000Z",
    "storage_class": "STANDARD"
  }
}
```

#### GET /v1/buckets/:bucket/objects/:key/download
Download object.

**Response:**
Binary file content with appropriate Content-Type and Content-Length headers.

#### DELETE /v1/buckets/:bucket/objects/:key
Delete object.

**Response:**
```json
{
  "success": true
}
```

### Presigned URLs

#### POST /v1/buckets/:bucket/presign/upload
Generate presigned upload URL.

**Request Body:**
```json
{
  "key": "folder/file.txt",
  "content_type": "text/plain",
  "expires_in": 3600,
  "max_size_bytes": 104857600
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "url": "https://storage.example.com/upload?signature=...",
    "expires_at": "2026-02-11T11:00:00.000Z",
    "fields": {
      "key": "folder/file.txt",
      "Content-Type": "text/plain"
    }
  }
}
```

#### POST /v1/buckets/:bucket/presign/download
Generate presigned download URL.

**Request Body:**
```json
{
  "key": "folder/file.txt",
  "expires_in": 3600
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "url": "https://storage.example.com/download?signature=...",
    "expires_at": "2026-02-11T11:00:00.000Z"
  }
}
```

### Multipart Uploads

#### POST /v1/buckets/:bucket/multipart/initiate
Initiate multipart upload.

**Request Body:**
```json
{
  "key": "large-file.zip",
  "content_type": "application/zip"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "upload_id": "uuid",
    "bucket": "my-bucket",
    "key": "large-file.zip"
  }
}
```

#### POST /v1/buckets/:bucket/multipart/:uploadId/part
Upload part.

**Form Data:**
- `part_number` - Part number (1-10000)
- `file` - Part data

**Response:**
```json
{
  "success": true,
  "data": {
    "part_number": 1,
    "etag": "5d41402abc4b2a76b9719d911017c592"
  }
}
```

#### POST /v1/buckets/:bucket/multipart/:uploadId/complete
Complete multipart upload.

**Request Body:**
```json
{
  "parts": [
    { "part_number": 1, "etag": "..." },
    { "part_number": 2, "etag": "..." }
  ]
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "id": "uuid",
    "bucket": "my-bucket",
    "key": "large-file.zip",
    "size_bytes": 524288000
  }
}
```

#### DELETE /v1/buckets/:bucket/multipart/:uploadId
Abort multipart upload.

**Response:**
```json
{
  "success": true
}
```

### Access Logs

#### GET /v1/logs
Get access logs.

**Query Parameters:**
- `bucket` (optional) - Filter by bucket
- `operation` (optional) - Filter by operation (upload, download, delete)
- `start_date` (optional) - Start date (ISO 8601)
- `end_date` (optional) - End date (ISO 8601)
- `limit` (optional) - Max results (default: 100)

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "uuid",
      "bucket": "my-bucket",
      "key": "folder/file.txt",
      "operation": "download",
      "status_code": 200,
      "bytes_transferred": 1024,
      "ip_address": "192.168.1.1",
      "user_agent": "Mozilla/5.0...",
      "timestamp": "2026-02-11T10:00:00.000Z"
    }
  ]
}
```

### Usage Statistics

#### GET /v1/usage
Get storage usage statistics.

**Response:**
```json
{
  "success": true,
  "data": {
    "buckets": 5,
    "objects": 1250,
    "totalSizeBytes": 10737418240,
    "totalSizeGB": 10.0,
    "byBucket": [
      {
        "bucket": "my-bucket",
        "objects": 500,
        "sizeBytes": 5368709120
      }
    ],
    "byProvider": [
      {
        "provider": "local",
        "buckets": 3,
        "objects": 750,
        "sizeBytes": 8053063680
      }
    ]
  }
}
```

---

## Webhook Events

| Event | Description | Payload |
|-------|-------------|---------|
| `bucket.created` | Sync new bucket to database | `{ bucket_name, provider }` |
| `bucket.updated` | Update bucket configuration | `{ bucket_name, changes }` |
| `bucket.deleted` | Mark bucket as deleted | `{ bucket_name }` |
| `object.created` | Track new object upload | `{ bucket, key, size_bytes }` |
| `object.deleted` | Remove object from tracking | `{ bucket, key }` |
| `upload.completed` | Mark multipart upload as completed | `{ upload_id, bucket, key }` |
| `upload.aborted` | Mark multipart upload as aborted | `{ upload_id, bucket, key }` |

### Webhook Payload Example

```json
{
  "id": "evt_abc123",
  "type": "object.created",
  "created": 1707649200,
  "data": {
    "bucket": "my-bucket",
    "key": "folder/file.txt",
    "size_bytes": 1024,
    "content_type": "text/plain",
    "uploaded_by": "user123"
  }
}
```

---

## Database Schema

### os_buckets

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `name` | VARCHAR(255) | Bucket name |
| `provider` | VARCHAR(32) | Storage provider |
| `region` | VARCHAR(64) | Provider region |
| `public` | BOOLEAN | Public access flag |
| `versioning_enabled` | BOOLEAN | Versioning enabled |
| `lifecycle_rules` | JSONB | Lifecycle rules |
| `cors_rules` | JSONB | CORS configuration |
| `encryption_config` | JSONB | Encryption settings |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Update timestamp |

**Indexes:**
- `idx_os_buckets_account` - source_account_id
- `idx_os_buckets_provider` - provider

**Unique Constraint:**
- `(source_account_id, name)`

### os_objects

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `bucket_id` | UUID | Parent bucket reference |
| `key` | TEXT | Object key |
| `size_bytes` | BIGINT | Object size |
| `content_type` | VARCHAR(128) | MIME type |
| `etag` | VARCHAR(255) | Entity tag |
| `metadata` | JSONB | Custom metadata |
| `storage_class` | VARCHAR(32) | Storage class |
| `version_id` | VARCHAR(255) | Version ID (if versioned) |
| `is_latest` | BOOLEAN | Latest version flag |
| `last_modified` | TIMESTAMPTZ | Last modification time |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:**
- `idx_os_objects_account` - source_account_id
- `idx_os_objects_bucket` - bucket_id
- `idx_os_objects_key` - key (prefix)
- `idx_os_objects_size` - size_bytes
- `idx_os_objects_modified` - last_modified DESC

**Unique Constraint:**
- `(source_account_id, bucket_id, key, version_id)`

### os_upload_sessions

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key (upload ID) |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `bucket_id` | UUID | Parent bucket reference |
| `key` | TEXT | Object key |
| `content_type` | VARCHAR(128) | MIME type |
| `parts` | JSONB | Uploaded parts |
| `status` | VARCHAR(16) | Upload status |
| `expires_at` | TIMESTAMPTZ | Expiration time |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `completed_at` | TIMESTAMPTZ | Completion timestamp |

**Upload Statuses:**
- `initiated` - Upload started
- `uploading` - Parts being uploaded
- `completed` - Upload completed
- `aborted` - Upload cancelled

**Indexes:**
- `idx_os_uploads_account` - source_account_id
- `idx_os_uploads_bucket` - bucket_id
- `idx_os_uploads_status` - status
- `idx_os_uploads_expires` - expires_at

### os_access_logs

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `bucket_id` | UUID | Bucket reference |
| `key` | TEXT | Object key |
| `operation` | VARCHAR(16) | Operation type |
| `status_code` | INTEGER | HTTP status code |
| `bytes_transferred` | BIGINT | Bytes transferred |
| `ip_address` | INET | Client IP address |
| `user_agent` | TEXT | User agent string |
| `request_time_ms` | INTEGER | Request duration |
| `timestamp` | TIMESTAMPTZ | Request timestamp |

**Operations:**
- `upload` - Object upload
- `download` - Object download
- `delete` - Object deletion
- `list` - List objects
- `head` - Get metadata

**Indexes:**
- `idx_os_logs_account` - source_account_id
- `idx_os_logs_bucket` - bucket_id
- `idx_os_logs_operation` - operation
- `idx_os_logs_timestamp` - timestamp DESC

### os_webhook_events

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (event ID) |
| `source_account_id` | VARCHAR(128) | Multi-account isolation |
| `event_type` | VARCHAR(128) | Event type |
| `payload` | JSONB | Event payload |
| `processed` | BOOLEAN | Processing status |
| `processed_at` | TIMESTAMPTZ | Processing timestamp |
| `error` | TEXT | Error message if failed |
| `created_at` | TIMESTAMPTZ | Event creation time |

**Indexes:**
- `idx_os_webhook_account` - source_account_id
- `idx_os_webhook_processed` - processed
- `idx_os_webhook_created` - created_at DESC

---

## Storage Providers

### Local Storage

Store files on local filesystem.

**Configuration:**
```bash
OS_DEFAULT_PROVIDER=local
OS_STORAGE_BASE_PATH=/data/object-storage
```

**Directory Structure:**
```
/data/object-storage/
├── my-bucket/
│   ├── folder/
│   │   └── file.txt
│   └── image.jpg
└── another-bucket/
    └── document.pdf
```

### AWS S3

Store files in Amazon S3.

**Configuration:**
```bash
OS_DEFAULT_PROVIDER=s3
OS_S3_REGION=us-east-1
OS_S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
OS_S3_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

### MinIO

Self-hosted S3-compatible storage.

**Configuration:**
```bash
OS_DEFAULT_PROVIDER=minio
OS_S3_ENDPOINT=http://localhost:9000
OS_S3_REGION=us-east-1
OS_S3_ACCESS_KEY=minioadmin
OS_S3_SECRET_KEY=minioadmin
```

### Cloudflare R2

Cloudflare's S3-compatible object storage.

**Configuration:**
```bash
OS_DEFAULT_PROVIDER=r2
OS_S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
OS_S3_REGION=auto
OS_S3_ACCESS_KEY=<r2-access-key>
OS_S3_SECRET_KEY=<r2-secret-key>
```

### Google Cloud Storage

GCS with S3 compatibility mode.

**Configuration:**
```bash
OS_DEFAULT_PROVIDER=gcs
OS_S3_ENDPOINT=https://storage.googleapis.com
OS_S3_REGION=us-central1
OS_S3_ACCESS_KEY=<gcs-access-key>
OS_S3_SECRET_KEY=<gcs-secret-key>
```

### Backblaze B2

B2 with S3-compatible API.

**Configuration:**
```bash
OS_DEFAULT_PROVIDER=b2
OS_S3_ENDPOINT=<b2-endpoint-from-account>
OS_S3_REGION=us-west-002
OS_S3_ACCESS_KEY=<b2-key-id>
OS_S3_SECRET_KEY=<b2-application-key>
```

---

## TypeScript Implementation

### File Structure

```
plugins/object-storage/ts/src/
├── types.ts          # TypeScript interfaces
├── config.ts         # Configuration loading
├── database.ts       # Database operations
├── providers/        # Storage provider implementations
│   ├── base.ts       # Base provider interface
│   ├── local.ts      # Local filesystem
│   └── s3.ts         # S3-compatible providers
├── server.ts         # HTTP server
├── cli.ts            # CLI commands
└── index.ts          # Module exports
```

### Key Components

#### StorageProvider (providers/base.ts)
- Interface for all providers
- Standard operations (upload, download, delete, list)
- Presigned URL generation

#### LocalProvider (providers/local.ts)
- Filesystem-based storage
- Efficient streaming
- Metadata storage

#### S3Provider (providers/s3.ts)
- AWS SDK v3 integration
- Multipart upload support
- Presigned URL generation

---

## Examples

### Example 1: Upload with Presigned URL

```typescript
// Generate presigned upload URL
const response = await fetch('http://localhost:3301/v1/buckets/my-bucket/presign/upload', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    key: 'uploads/file.txt',
    content_type: 'text/plain',
    expires_in: 3600
  })
});

const { data } = await response.json();

// Upload file using presigned URL
const file = new File(['Hello World'], 'file.txt');
await fetch(data.url, {
  method: 'PUT',
  headers: { 'Content-Type': 'text/plain' },
  body: file
});
```

### Example 2: Multipart Upload

```bash
#!/bin/bash

BUCKET="my-bucket"
KEY="large-file.zip"
FILE="/path/to/large-file.zip"
PART_SIZE=$((100 * 1024 * 1024))  # 100MB

# Initiate multipart upload
upload_id=$(curl -X POST "http://localhost:3301/v1/buckets/$BUCKET/multipart/initiate" \
  -H "Content-Type: application/json" \
  -d "{\"key\": \"$KEY\", \"content_type\": \"application/zip\"}" \
  | jq -r '.data.upload_id')

# Split and upload parts
split -b $PART_SIZE "$FILE" part_
part_number=1
parts=()

for part_file in part_*; do
  etag=$(curl -X POST "http://localhost:3301/v1/buckets/$BUCKET/multipart/$upload_id/part" \
    -F "part_number=$part_number" \
    -F "file=@$part_file" \
    | jq -r '.data.etag')

  parts+=("{\"part_number\": $part_number, \"etag\": \"$etag\"}")
  ((part_number++))
  rm "$part_file"
done

# Complete upload
parts_json="[$(IFS=,; echo "${parts[*]}")]"
curl -X POST "http://localhost:3301/v1/buckets/$BUCKET/multipart/$upload_id/complete" \
  -H "Content-Type: application/json" \
  -d "{\"parts\": $parts_json}"
```

### Example 3: Query Storage Usage

```sql
-- Storage usage by bucket
SELECT
  b.name,
  COUNT(o.id) as object_count,
  SUM(o.size_bytes) as total_bytes,
  SUM(o.size_bytes) / 1024 / 1024 / 1024 as total_gb
FROM os_buckets b
LEFT JOIN os_objects o ON o.bucket_id = b.id
WHERE b.source_account_id = 'primary'
GROUP BY b.name
ORDER BY total_bytes DESC;

-- Most accessed objects
SELECT
  b.name as bucket,
  l.key,
  COUNT(*) FILTER (WHERE l.operation = 'download') as downloads,
  SUM(l.bytes_transferred) as total_bytes
FROM os_access_logs l
JOIN os_buckets b ON b.id = l.bucket_id
WHERE l.source_account_id = 'primary'
  AND l.timestamp >= NOW() - INTERVAL '7 days'
GROUP BY b.name, l.key
ORDER BY downloads DESC
LIMIT 20;

-- Failed uploads
SELECT
  bucket,
  key,
  COUNT(*) as failures,
  MAX(timestamp) as last_failure
FROM os_access_logs
WHERE operation = 'upload'
  AND status_code >= 400
  AND timestamp >= NOW() - INTERVAL '24 hours'
GROUP BY bucket, key
ORDER BY failures DESC;
```

---

## Troubleshooting

### Common Issues

#### Permission Denied (Local Storage)

**Error:**
```
Error: EACCES: permission denied
```

**Solution:**
```bash
# Ensure directory exists and is writable
sudo mkdir -p /data/object-storage
sudo chown $USER:$USER /data/object-storage
chmod 755 /data/object-storage
```

#### S3 Credentials Invalid

**Error:**
```
Error: The security token included in the request is invalid
```

**Solution:**
1. Verify access key and secret key
2. Check key has not expired
3. Verify region is correct
4. Test credentials with AWS CLI: `aws s3 ls`

#### MinIO Connection Refused

**Error:**
```
Error: connect ECONNREFUSED
```

**Solution:**
1. Verify MinIO is running: `docker ps | grep minio`
2. Check endpoint URL is correct
3. Ensure port is accessible: `curl http://localhost:9000/minio/health/live`

#### Upload Size Limit Exceeded

**Error:**
```
Error: File size exceeds maximum allowed
```

**Solution:**
Increase limit:
```bash
OS_MAX_UPLOAD_SIZE=10737418240  # 10GB
```

Or use multipart upload for files > 100MB.

#### Presigned URL Expired

**Error:**
```
Error: 403 Forbidden - Request has expired
```

**Solution:**
Generate new presigned URL or increase expiry:
```bash
OS_PRESIGN_EXPIRY_SECONDS=7200  # 2 hours
```

### Debug Mode

Enable debug logging:
```bash
LOG_LEVEL=debug nself plugin object-storage server
```

---

## Support

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki/Object-Storage
- **Issues**: https://github.com/acamarata/nself-plugins/issues
- **S3 API**: https://docs.aws.amazon.com/s3/
