# Object Storage Plugin

Production-ready object storage plugin for nself with multi-provider support (local, S3, MinIO, Cloudflare R2, GCS, B2).

## Features

- **Multi-Provider Support**: Local filesystem, AWS S3, MinIO, Cloudflare R2, Google Cloud Storage, Backblaze B2
- **S3-Compatible API**: Full support for S3-compatible operations
- **Presigned URLs**: Generate temporary upload/download URLs
- **Multipart Uploads**: Support for large file uploads
- **Bucket Management**: Create, list, update, and delete buckets
- **Quota Management**: Set and enforce storage quotas per bucket
- **Access Logging**: Track all storage operations
- **CORS Support**: Configurable CORS origins per bucket
- **MIME Type Filtering**: Restrict uploads by content type
- **Multi-App Support**: Isolate storage by source account

## Installation

```bash
cd plugins/object-storage/ts
npm install
npm run build
```

## Configuration

Copy `.env.example` to `.env` and configure:

```bash
# Required
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Optional - Local Storage (default)
OS_DEFAULT_PROVIDER=local
OS_STORAGE_BASE_PATH=/data/object-storage

# Optional - S3-Compatible
OS_DEFAULT_PROVIDER=s3
OS_S3_ENDPOINT=https://s3.us-east-1.amazonaws.com
OS_S3_REGION=us-east-1
OS_S3_ACCESS_KEY=your-access-key
OS_S3_SECRET_KEY=your-secret-key
```

## Quick Start

```bash
# Initialize schema
npm run build && node dist/cli.js init

# Start server
npm run dev

# Or in production
npm start
```

## CLI Commands

```bash
# Initialize schema
nself-object-storage init

# Start server
nself-object-storage server

# Show status
nself-object-storage status

# List buckets
nself-object-storage buckets

# List objects in bucket
nself-object-storage objects -b my-bucket

# Upload file
nself-object-storage upload -b my-bucket -f /path/to/file.jpg

# Download file
nself-object-storage download -b my-bucket -k file.jpg -o /path/to/output.jpg

# Generate presigned URL
nself-object-storage presign -b my-bucket -k file.jpg -m GET

# Show bucket usage
nself-object-storage usage -b my-bucket
```

## API Endpoints

### Health & Status

- `GET /health` - Basic health check
- `GET /ready` - Readiness check with database
- `GET /live` - Liveness check with stats
- `GET /v1/status` - Overall status

### Buckets

- `POST /v1/buckets` - Create bucket
- `GET /v1/buckets` - List buckets
- `GET /v1/buckets/:id` - Get bucket details
- `PUT /v1/buckets/:id` - Update bucket
- `DELETE /v1/buckets/:id` - Delete bucket (must be empty)

### Objects

- `GET /v1/buckets/:id/objects` - List objects
- `POST /v1/buckets/:id/objects` - Upload object (multipart/form-data)
- `GET /v1/buckets/:id/objects/:key` - Get object metadata
- `DELETE /v1/buckets/:id/objects/:key` - Delete object

### Presigned URLs

- `POST /v1/presign/upload` - Generate presigned upload URL
- `POST /v1/presign/download` - Generate presigned download URL

### Multipart Uploads

- `POST /v1/upload-sessions` - Start multipart upload
- `PUT /v1/upload-sessions/:id/parts/:partNumber` - Upload part
- `POST /v1/upload-sessions/:id/complete` - Complete upload
- `POST /v1/upload-sessions/:id/abort` - Abort upload

### Usage & Logs

- `GET /v1/buckets/:id/usage` - Get bucket usage stats
- `GET /v1/access-logs` - Query access logs

## Database Schema

### Tables

1. **os_buckets** - Bucket configuration and metadata
2. **os_objects** - Object metadata and version tracking
3. **os_upload_sessions** - Multipart upload tracking
4. **os_access_logs** - Access logging for all operations
5. **os_webhook_events** - Webhook event tracking

## Examples

### Create a Bucket

```bash
curl -X POST http://localhost:3301/v1/buckets \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-bucket",
    "provider": "local",
    "quota_bytes": 1073741824
  }'
```

### Upload a File

```bash
curl -X POST http://localhost:3301/v1/buckets/{bucket-id}/objects \
  -F "file=@/path/to/file.jpg" \
  -F "key=uploads/file.jpg"
```

### Generate Presigned Upload URL

```bash
curl -X POST http://localhost:3301/v1/presign/upload \
  -H "Content-Type: application/json" \
  -d '{
    "bucket_id": "{bucket-id}",
    "key": "uploads/file.jpg",
    "content_type": "image/jpeg",
    "expires_in": 3600
  }'
```

## Provider Configuration

### Local Storage

```env
OS_DEFAULT_PROVIDER=local
OS_STORAGE_BASE_PATH=/data/object-storage
```

### AWS S3

```env
OS_DEFAULT_PROVIDER=s3
OS_S3_REGION=us-east-1
OS_S3_ACCESS_KEY=your-access-key
OS_S3_SECRET_KEY=your-secret-key
```

### MinIO

```env
OS_DEFAULT_PROVIDER=minio
OS_S3_ENDPOINT=http://localhost:9000
OS_S3_REGION=us-east-1
OS_S3_ACCESS_KEY=minioadmin
OS_S3_SECRET_KEY=minioadmin
```

### Cloudflare R2

```env
OS_DEFAULT_PROVIDER=r2
OS_S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
OS_S3_REGION=auto
OS_S3_ACCESS_KEY=your-r2-access-key
OS_S3_SECRET_KEY=your-r2-secret-key
```

## Development

```bash
# Watch mode
npm run watch

# Type check
npm run typecheck

# Dev server
npm run dev

# Clean build artifacts
npm run clean
```

## License

MIT
