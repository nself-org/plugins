# plugin-storage

S3-compatible file storage plugin for nSelf. Provides bucket management and object
PUT/GET/DELETE/LIST via a Go HTTP service backed by Postgres metadata and an S3-compatible
storage backend (MinIO, AWS S3, Cloudflare R2, or any S3-compatible endpoint).

**Port:** 9007 | **License:** Pro (requires_license=true) | **Bundle:** unbundled

## Quick Start

```bash
nself license set <your-key>
nself plugin install storage
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NSELF_DB_URL` | Yes | — | PostgreSQL connection string |
| `STORAGE_PLUGIN_PORT` | No | `9007` | HTTP server port |
| `S3_ENDPOINT` | No | `minio:9000` | S3-compatible endpoint (config-only, not user-overridable) |
| `S3_REGION` | No | `us-east-1` | Default region |
| `S3_ACCESS_KEY` | No | — | S3 access key |
| `S3_SECRET_KEY` | No | — | S3 secret key |
| `S3_USE_SSL` | No | `false` | Enable TLS for S3 endpoint |
| `NSELF_LICENSE_KEY` | Yes | — | License key |

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/storage/buckets` | Create bucket |
| `GET` | `/storage/buckets` | List buckets |
| `DELETE` | `/storage/buckets/{bucket}` | Delete bucket |
| `PUT` | `/storage/buckets/{bucket}/objects/{key}` | Upload object |
| `GET` | `/storage/buckets/{bucket}/objects/{key}` | Get object metadata |
| `DELETE` | `/storage/buckets/{bucket}/objects/{key}` | Delete object |
| `GET` | `/storage/buckets/{bucket}/objects` | List objects (param: `prefix`) |

## Security

- Object keys are validated against path traversal (`..` sequences are rejected).
- `S3_ENDPOINT` is configuration-only; no request parameter can override the endpoint.
- All DB queries are scoped to `source_account_id` for multi-app isolation.
- Hasura row filters enforce `source_account_id = X-Hasura-Source-Account-Id` on all tables.

## Database Tables

| Table | Purpose |
|-------|---------|
| `np_storage_buckets` | Bucket registry per account |
| `np_storage_objects` | Object metadata (key, size, content-type, etag) |
| `np_storage_metadata` | Arbitrary key-value metadata per object |
