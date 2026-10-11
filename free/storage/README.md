# plugin-storage

File storage plugin for nSelf. It exposes bucket management and object PUT/GET/DELETE/LIST over a Go HTTP
service. **Object bytes live in an S3-compatible endpoint** (SeaweedFS, MinIO, AWS S3, Cloudflare R2, ...);
**Postgres holds only metadata** (bucket registry, object key, size, content type, ETag). Bytes never touch Postgres.

**Port:** 9007 | **License:** free (MIT, no license key) | **Bundle:** unbundled

## Quick Start

```bash
nself plugin install storage
```

Set the `S3_*` variables below in the project env so the plugin can reach your S3 endpoint.

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NSELF_DB_URL` | Yes | — | PostgreSQL connection string |
| `STORAGE_PLUGIN_PORT` | No | `9007` | HTTP server port |
| `STORAGE_MAX_UPLOADS` | No | `64` | Uploads of any size that may run at once; others wait |
| `STORAGE_MAX_LARGE_UPLOADS` | No | `8` | Of those, uploads above 1 MiB that may run at once (each holds a 16 MiB buffer) |
| `STORAGE_UPLOAD_SLOT_WAIT` | No | `30s` | How long an upload waits for a free slot before `503` |
| `S3_ENDPOINT` | No | `minio:9000` | S3 endpoint, `host[:port]` or a full `http(s)://` URL (config-only, not user-overridable) |
| `S3_REGION` | No | `us-east-1` | Signing region |
| `S3_ACCESS_KEY` | With `S3_SECRET_KEY` | — | S3 access key |
| `S3_SECRET_KEY` | With `S3_ACCESS_KEY` | — | S3 secret key |
| `S3_USE_SSL` | No | `false` | Use `https://` when `S3_ENDPOINT` has no scheme |

**S3 not configured.** If `S3_ACCESS_KEY` and `S3_SECRET_KEY` are both unset the plugin still boots and serves
`/health` (`"s3": "not_configured"`), the bucket and object list routes and the metadata read
(`GET ...?metadata=true`), but every route that needs S3 (create bucket, upload, download, delete) answers
`503 {"error":"object storage not configured"}`. Setting only one of the two keys is a start-up error. A request that
reaches S3 and fails there answers `502 {"error":"object storage error"}`.

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check (`status`, `migrations`, `s3`: `configured` or `not_configured`; no S3 probe) |
| `POST` | `/storage/buckets` | Create bucket (creates it in S3 if missing, then records it) |
| `GET` | `/storage/buckets` | List buckets |
| `DELETE` | `/storage/buckets/{bucket}` | Delete the account's objects in the bucket and its bucket row |
| `PUT` | `/storage/buckets/{bucket}/objects/{key}` | Upload object (streamed; multipart above 16 MiB). Returns `id`, `key`, `size_bytes`, `etag` |
| `GET` | `/storage/buckets/{bucket}/objects/{key}` | Download the object bytes; `?metadata=true` returns the metadata row as JSON |
| `DELETE` | `/storage/buckets/{bucket}/objects/{key}` | Delete object from S3 and its metadata row |
| `GET` | `/storage/buckets/{bucket}/objects` | List objects from Postgres metadata (param: `prefix`) |

**Behaviour change.** `GET .../objects/{key}` now returns the object bytes (with `Content-Type`, `ETag`,
`X-Content-Type-Options: nosniff` and `Content-Security-Policy: sandbox`, so stored HTML or SVG cannot run script from the
plugin's origin). The JSON metadata row that this route used to return is now `GET .../objects/{key}?metadata=true`.

Bucket names follow S3 rules (3-63 characters of `a-z 0-9 . -`, start and end alphanumeric, no `..`, not an IP
address); anything else is `400 {"error":"invalid bucket name"}`. A `Content-Type` with non-ASCII or control bytes is `400 {"error":"invalid content type"}`. Object keys are 1-1024 bytes (the account prefix counts) with no `..` and no `.` or `/.` ending. A `X-Hasura-Source-Account-Id` that is blank, `.`, `..` or contains control characters is `400 {"error":"invalid source account id"}`.

## Storage layout

The logical bucket name is the physical S3 bucket. Several accounts (`source_account_id`) can share one S3 bucket, so
every object is stored under a per-account prefix:

```
<url-escaped source_account_id>/<key>
```

An account id containing `/` is escaped (`a/b` becomes `a%2Fb`), so one account can never address another's keys.
Deleting a bucket through the API removes that account's objects and bucket row; **the physical S3 bucket is never
deleted** (it may be shared with other accounts and with apps that use the endpoint directly).

Any account may register a physical bucket that already exists (creating it is idempotent) and write under its own
prefix, including a bucket that apps use directly. Isolation between accounts is the per-account key prefix, not
the bucket; use separate S3 credentials or an endpoint-side policy when a bucket must stay closed to the plugin.

Uploads of 16 MiB or less use one `PutObject`; larger uploads use a multipart upload with 16 MiB parts, streamed with
one part in memory, and are aborted on any failure. The client is path-style with static credentials and
"when required" request/response checksums (needed by SeaweedFS and MinIO).

**What is bounded.** At most `STORAGE_MAX_UPLOADS` uploads run at once; each holds its first MiB in memory. Of those,
at most `STORAGE_MAX_LARGE_UPLOADS` hold a pooled 16 MiB buffer, so upload memory is about
`64 x 1 MiB + 8 x 16 MiB` at the defaults. A body announced as larger than 1 MiB takes its large slot before any byte is read. An upload that finds no free slot waits `STORAGE_UPLOAD_SLOT_WAIT`, then
answers `503 {"error":"upload capacity reached, retry"}` with `Retry-After`. An upload body that sends nothing for 60
seconds fails (`408`) and frees its slot; a body that keeps flowing may take as long as it needs. Downloads have no
slot limit and are streamed; the server's write timeout caps a single download at 15 minutes.

**Truncated uploads never commit.** A body that ends early, or whose length differs from the declared
`Content-Length`, answers `400 {"error":"request body incomplete"}`; nothing is stored and a multipart upload is
aborted.

**Keys in URLs.** The key in `/objects/{key}` is percent-decoded exactly once. A `/` inside a key must be sent as
`%2F` (`a%2Fb` is the key `a/b`); `a%252Fb` is the different key `a%2Fb`. Keys containing control characters (a
newline, NUL, DEL) are `400 {"error":"invalid key"}`. This replaces the earlier behaviour where the encoded form was
stored literally, so keys written by an older version with `%` sequences are now addressed by their decoded form.

**Failed metadata write.** If the S3 write succeeds but the Postgres row cannot be written, bytes of a new key are
deleted again (best effort, logged); an overwrite keeps its new bytes.

**Concurrent writes.** Two simultaneous PUTs of one key can reach S3 and Postgres in opposite orders. A download uses
the `Content-Type` stored with the bytes in S3, but the object list and `?metadata=true` (Postgres) may briefly show the
size, ETag and type of the other write. The last S3 write holds the bytes.

Deleting a bucket also deletes every object under the account's prefix in S3 (listed from S3, not only the objects
that have a metadata row), so bytes left by a failed or racing upload do not outlive the bucket.

## S3 operations used

`CreateBucket`, `HeadBucket`, `PutObject`, `GetObject`, `HeadObject`, `DeleteObject`, `DeleteObjects`,
`CreateMultipartUpload`, `UploadPart`, `CompleteMultipartUpload`, `AbortMultipartUpload`, and SigV4 presigned GET/PUT
(`internal/s3store`; presigning is library code, no HTTP route exposes it). Apps and plugins that talk to the endpoint directly also rely on `ListBuckets`, `ListObjectsV2`,
`PutBucketPolicy`/`GetBucketPolicy`, `PutBucketCors`/`GetBucketCors`, `PutBucketVersioning` and `ListObjectVersions`;
the coverage suite below checks those too.

**Known gap on MinIO.** `pgsty/minio` (MinIO community) has no bucket CORS API: `PutBucketCors` answers `501 NotImplemented`.
The suite pins this as a declared expectation (`test/s3coverage/unsupported_test.go`) and still checks that a preflight
gets `Access-Control-Allow-Origin`. Apps on MinIO get CORS from the server-wide setting (`MINIO_API_CORS_ALLOW_ORIGIN`,
default `*`), not per bucket. SeaweedFS honours per-bucket CORS rules.

## S3 coverage suite

`test/s3coverage` (build tag `s3coverage`) runs one test per operation (`TestOp_*`) and the plugin end to end
(`TestPlugin_*`, including a 64 MiB multipart upload, a check that Postgres holds no object bytes, and tenant isolation)
against any S3 endpoint. `run.sh` starts a pinned server image and a pinned Postgres in docker, runs the suite in a
pinned golang container on the same network, and tears everything down. It needs only `docker` and `openssl`;
credentials are random per run and reach containers through files, never on a command line.

```bash
TMPDIR=/path/to/scratch bash test/s3coverage/run.sh seaweedfs
TMPDIR=/path/to/scratch bash test/s3coverage/run.sh minio
```

Pinned images (OCI index digests, also in `run.sh`):

| Role | Image |
|------|-------|
| SeaweedFS | `chrislusf/seaweedfs:4.48@sha256:4e61d15fd35994cb1e43e1e553dff106794841fd9a99ade2fc8c8bfce4d7872d` |
| MinIO | `pgsty/minio:RELEASE.2026-08-04T00-00-00Z@sha256:b6bfe7239bfc83fb90d31612d9704d86039dd714f7904b3f1ad68f211e602372` |
| Test runner | `golang:1.26-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade` |
| Database | `postgres:16@sha256:ca0bd484cb98bf4b24eb1010e73fb3fcbd6714d240fbc1a10eea5b7dbecb641d` |

To point the suite at another endpoint, set `S3COV_ENDPOINT`, `S3COV_ACCESS_KEY`, `S3COV_SECRET_KEY`, `S3COV_DB_URL` and `S3COV_TARGET`
(`seaweedfs` or `minio`, which selects the declared-unsupported table); `S3COV_REGION` is optional. Then run
`go test -mod=vendor -tags s3coverage -count=1 -v ./test/s3coverage/` in this directory. Unit tests (`go test -mod=vendor ./...`) need no network, docker or database.

## Security

- Object keys are validated against path traversal (`..` sequences are rejected).
- `S3_ENDPOINT` is configuration-only; no request parameter can override the endpoint.
- S3 credentials come from the environment only.
- All DB queries are scoped to `source_account_id` for multi-app isolation, and S3 keys carry the same prefix.
- Hasura row filters enforce `source_account_id = X-Hasura-Source-Account-Id` on all tables.

## Database Tables

| Table | Purpose |
|-------|---------|
| `np_storage_buckets` | Bucket registry per account |
| `np_storage_objects` | Object metadata (key, size, content-type, etag); no bytes |
| `np_storage_metadata` | Arbitrary key-value metadata per object |

## Boot-applied migrations

`plugin-storage` applies its own `migrations/*.sql` when it starts (contract `plugin.boot-migrations` v1, `sdk/go/migrate`):
files run in file-name order, one transaction per file, recorded in `np_storage.schema_migrations`. The manifest declares
`migrations.apply: boot`, and `GET /health` reports `migrations: {applied, expected}`. The plugin no longer creates
tables from Go code. The tables `np_storage_buckets`, `np_storage_objects` and `np_storage_metadata` live in `public`. Earlier versions never created them at start-up (the SQL had to be applied by hand); a fresh install now has them without any manual step.

**Upgrading an existing install.** The migrations are idempotent (`IF NOT EXISTS`, guarded blocks), so they apply
over tables an earlier version already created: no data is touched or dropped. On the first start of the new version the
ledger is created and every file is recorded. The first start after upgrading builds indexes on existing tables and can take minutes on large tables; until it finishes the plugin does not answer /health, so the container shows as unhealthy and an install readiness wait can time out: re-run it once the migration log line appears.
