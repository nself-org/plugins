# Plugin Spec: nself-image

**Bundle:** nFamily  
**Tier:** $0.99/mo (nFamily bundle) or ɳSelf+ $3.99/mo  
**Language:** Go (see §1 Architecture)  
**Port:** 3827  
**Target version:** v1.2.0  
**Repo path:** `plugins-pro/paid/nself-image/`  
**Status:** Planned (P98 S04)

---

## §1 Architecture

### Language (Go)

This plugin is written in Go, like the other nSelf plugin services. An earlier draft chose Rust on the grounds of a "Plugin Language Policy" and an unbenchmarked SIMD speed-up. No such policy document exists, and no benchmark backs the claim, so the draft is withdrawn. The draft's precedent (the `nclaw` `libs/libnclaw` Rust crate) is also not a policy: it is an orphaned crate.

Rust stays off the table until someone attaches a benchmark on CX21 hardware that shows Go cannot meet the targets in §8 and records an ADR 0031 exception. Until then:

- Resize and crop use the Go standard library plus `golang.org/x/image` (or a maintained resizing library chosen in the implementation sprint).
- WebP and AVIF encoders are the open risk: `golang.org/x/image` has no encoder for either. The implementation sprint picks between a cgo binding to libwebp/libavif and a pure-Go or WebAssembly encoder, and records the choice. Prefer `CGO_ENABLED=0` so the output stays one static binary.
- The performance targets in §8 are targets to measure, not claims. If Go misses them, that is the evidence for an ADR 0031 exception.

### Service Shape

Runs as a Custom Service (`CS_N`) in the nSelf stack. Exposes HTTP on `127.0.0.1:3827` (internal only). Registered as a Hasura Action endpoint. Reads from MinIO, writes output to MinIO.

```
[Hasura Action] ──→ [nself-image :3827] ──→ [MinIO input bucket]
                                         ──→ [MinIO output bucket]
```

All traffic via Nginx reverse proxy when accessed from outside the host. No direct external port exposure.

### Processing Pipeline

1. Receive job via `POST /process`: `{input, output, ops[]}`
2. Validate HMAC token (`IMAGE_SHARED_SECRET`) — reject if invalid
3. Stream-download input object from MinIO in chunks (no full-file buffer for large inputs)
4. Decode the image with Go image decoders (JPEG, PNG, WebP, GIF, BMP, TIFF)
5. Apply ops in declared order: resize → crop → strip_exif → format_convert
6. Stream-upload output to MinIO output bucket
7. Optionally record audit row in `np_image.jobs` if `IMAGE_AUDIT_ENABLED=true`
8. Return `{output_key, width, height, size_bytes, format, duration_ms}`

---

## §2 Data Model

The plugin is stateless by default. An optional audit table is available for compliance and debugging.

### np_image.jobs (optional audit table)

Enabled via `IMAGE_AUDIT_ENABLED=true`. Default: off.

```sql
CREATE SCHEMA IF NOT EXISTS np_image;

CREATE TABLE np_image.jobs (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           UUID,
  tenant_id         UUID,
  source_account_id TEXT        NOT NULL DEFAULT 'primary',
  operation         TEXT        NOT NULL,  -- 'resize' | 'crop' | 'convert' | 'exif_strip' | 'pipeline'
  input_path        TEXT        NOT NULL,
  output_path       TEXT,
  status            TEXT        NOT NULL DEFAULT 'pending',
                                           -- 'pending' | 'processing' | 'done' | 'failed'
  error             TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at      TIMESTAMPTZ
);

CREATE INDEX ON np_image.jobs (user_id, created_at DESC);
CREATE INDEX ON np_image.jobs (tenant_id, status);
CREATE INDEX ON np_image.jobs (source_account_id, status);
```

**Audit note:** "Audit table is optional — enabled via `IMAGE_AUDIT_ENABLED=true`. Default off to keep the service stateless. When enabled, completed_at is set on terminal status transitions."

### Multi-Tenancy Declaration

This plugin follows the **Multi-Tenant Convention Wall**:

| Column | Mechanism | Purpose |
|---|---|---|
| `source_account_id TEXT NOT NULL DEFAULT 'primary'` | Multi-App Isolation | Separates job records across independent consumer apps within one nSelf deploy |
| `tenant_id UUID` (nullable) | Cloud Multi-Tenancy | Separates records across paying nSelf Cloud customers |

**Decision tree applied:**
- `source_account_id` — present on all `np_image.*` tables because nself-image can be used by multiple apps (nFamily, nChat attachment plugin) running on a single nSelf deploy.
- `tenant_id` — present and nullable; populated only when the nSelf Cloud tier assigns a tenant context. Hasura row filter `{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}` enforces isolation for Cloud customers.

**Never** use `source_account_id` to separate Cloud customers. **Never** use `tenant_id` for multi-app isolation. See PPI Multi-Tenant Convention Wall.

**Enforcement:** `nself doctor --deep` check `PERM-RLS-01` verifies both columns and Hasura row filters are present on all `np_image.*` tables.

### Environment Variables

```
IMAGE_PROCESSING_PORT=3827
IMAGE_PROCESSING_MINIO_ENDPOINT=         # from nSelf stack shared config
IMAGE_PROCESSING_MINIO_ACCESS_KEY=
IMAGE_PROCESSING_MINIO_SECRET_KEY=
IMAGE_PROCESSING_MAX_INPUT_BYTES=52428800  # 50MB default
IMAGE_PROCESSING_MAX_CONCURRENT=8          # per-instance concurrency cap
IMAGE_PROCESSING_SHARED_SECRET=           # HMAC token for request auth
IMAGE_AUDIT_ENABLED=false                  # opt-in audit table logging
```

---

## §3 API

### POST /process

Request:
```json
{
  "input": {
    "bucket": "uploads",
    "key": "avatars/raw/user-123.jpg"
  },
  "output": {
    "bucket": "uploads",
    "key": "avatars/processed/user-123.webp"
  },
  "ops": [
    {"type": "resize", "width": 256, "height": 256, "fit": "cover"},
    {"type": "strip_exif"},
    {"type": "format", "to": "webp", "quality": 85}
  ]
}
```

Response (200 OK):
```json
{
  "output_key": "avatars/processed/user-123.webp",
  "width": 256,
  "height": 256,
  "size_bytes": 14200,
  "format": "webp",
  "duration_ms": 42
}
```

### GET /health

Returns `{"status":"ok","version":"0.1.0"}`. Used by `nself doctor`.

### Supported ops

| Op type | Params | Notes |
|---|---|---|
| `resize` | `width`, `height`, `fit` (cover/contain/fill) | Library chosen in the implementation sprint (see §1) |
| `crop` | `x`, `y`, `width`, `height` | Pixel-coordinate crop |
| `format` | `to` (webp/avif/jpeg/png), `quality` (1-100) | AVIF capped at 1MP input by default — see open questions |
| `strip_exif` | (no params) | Recommended for all user uploads; removes GPS, device info |

**AVIF note:** AVIF encoding is slow (~300ms for 256x256 on CX21). Cap at 1MP max input unless `"allow_slow": true` is explicitly set in the op.

---

## §4 (see §2 — Data Model contains DDL)

---

## §5 Permissions / Hasura Role Matrix

| Role | Permission |
|---|---|
| `user` | Can submit image processing jobs scoped to their own `user_id`; read own job status |
| `admin` | Can submit jobs for any user; view all job statuses; access audit table |
| `anonymous` | No access |
| `service` | Full access — internal plugin-to-plugin calls (e.g., nChat attachment plugin calling nself-image) |

**Row-level security note:** `np_image.jobs` enforces `user_id = X-Hasura-User-Id` for the `user` role via Hasura permission row filter:
```json
{"user_id": {"_eq": "X-Hasura-User-Id"}}
```

For the `service` role, no row filter applies (full table access for internal automation).

**HMAC auth:** Service-to-service calls (Hasura Action invocations) pass `X-Nself-Image-Token: <HMAC>` header. Token derived from `IMAGE_PROCESSING_SHARED_SECRET`. Callers without a valid HMAC receive 401.

---

## §6 Plugin Manifest (YAML)

```yaml
name: nself-image
version: 0.1.0
language: go
bundle: nfamily
visibility: public
port: 3827
service_type: custom_service
requires_license: true
license_type: nfamily
env_vars:
  - IMAGE_PROCESSING_PORT
  - IMAGE_PROCESSING_MINIO_ENDPOINT
  - IMAGE_PROCESSING_MINIO_ACCESS_KEY
  - IMAGE_PROCESSING_MINIO_SECRET_KEY
  - IMAGE_PROCESSING_MAX_INPUT_BYTES
  - IMAGE_PROCESSING_MAX_CONCURRENT
  - IMAGE_PROCESSING_SHARED_SECRET
  - IMAGE_AUDIT_ENABLED
dependencies:
  required:
    - minio
  optional: []
hasura_actions:
  - name: processImage
    endpoint: http://127.0.0.1:3827/process
    auth: hmac
arch_support:
  - linux-x86_64
  - linux-arm64
  - darwin-aarch64
  - windows-x86_64
```

---

## §7 Observability

### Prometheus metrics (exposed on `/metrics`)

| Metric | Type | Description |
|---|---|---|
| `image_processing_jobs_total` | Counter | Total jobs by `{op_type, format, status}` |
| `image_processing_duration_seconds` | Histogram | End-to-end job duration with buckets 0.01..10 |
| `image_processing_errors_total` | Counter | Errors by `{error_class}` (oversize, corrupt, minio_unavailable, auth_fail) |
| `image_processing_concurrent_jobs` | Gauge | Current in-flight jobs |
| `image_processing_minio_bytes_in` | Counter | Bytes downloaded from MinIO |
| `image_processing_minio_bytes_out` | Counter | Bytes uploaded to MinIO |

### Structured logging

```json
{"level":"info","ts":"2026-04-30T12:00:00Z","input_key":"avatars/raw/u.jpg","output_key":"avatars/processed/u.webp","ops":["resize","strip_exif","format"],"duration_ms":42,"size_bytes":14200}
{"level":"error","ts":"2026-04-30T12:01:00Z","input_key":"video/raw/v.mp4","error":"unsupported_format","duration_ms":3}
```

### nself doctor checks

- Port 3827 listening: `nc -zv 127.0.0.1 3827`
- MinIO connectivity: `GET /health` with MinIO round-trip
- `/health` returns 200 with `{"status":"ok"}`
- HMAC secret set: warns if `IMAGE_PROCESSING_SHARED_SECRET` is empty

---

## §8 Testing Plan

### Unit tests (Go, `go test`)

- Each op type with known input (JPEG 4K) → assert output dimensions/format/size
- `strip_exif`: assert GPS + device metadata removed from output
- Oversized input → 413 error response
- Corrupt image (truncated) → graceful error, no panic or recovered panic
- Unsupported format (MP4, PDF) → 400 error response
- HMAC validation: valid token → pass; missing token → 401; tampered token → 401

### Integration tests

- End-to-end MinIO roundtrip with a test bucket (`test-nself-image-ci`)
- Hasura Action `processImage` callthrough (Docker Compose test setup)
- Concurrent jobs up to `IMAGE_PROCESSING_MAX_CONCURRENT` without deadlock

### Performance baseline

Targets, not measured yet. A miss is the evidence an ADR 0031 exception needs (see §1).

- 256×256 WebP from 4K JPEG in < 100ms on CX21 (2vCPU, 4GB)
- 1MP JPEG resize to 512×512 PNG in < 200ms on CX21

### Error cases

- MinIO unreachable → 503 with retry hint
- Input bucket/key not found → 404
- Output bucket lacks write permission → 500 with explicit error detail
- AVIF with >1MP input and `allow_slow` not set → 400 with message

---

## §9 Competitive Parity

**Alternatives:** Cloudinary, Imgix, Vercel's `@vercel/og`, imgproxy (open-source Go), Sharp standalone (Node.js).

**Why nself-image is different:**
- **Self-hosted:** no per-transform cost, no vendor API key, no data leaving the server. Cloudinary charges per transformation; nself-image is flat-rate via the nFamily bundle ($0.99/mo).
- **Single static binary:** no runtime libvips dependency on the host. No speed advantage over imgproxy (the closest open-source competitor, also Go) is claimed.
- **nSelf-native:** directly integrated with MinIO (nSelf's storage layer) and Hasura Actions — zero glue code needed for nSelf apps vs Cloudinary's SDK requirement.

---

## §10 Rollout

1. Build the Go binary via the CI cross-compilation matrix (see §14)
2. Package as Docker image `nself/plugin-nself-image:0.1.0`
3. Install: `nself plugin install nself-image` (requires nFamily bundle license)
4. Run `nself plugin migrate nself-image` to create `np_image` schema (migration is no-op if `IMAGE_AUDIT_ENABLED=false`)
5. Provide `scripts/migrate-sharp.sh` for projects with existing Sharp usage — sed replacements for common call patterns (`sharp(buf).resize(w,h)` → GraphQL `processImage` mutation)
6. Add Hasura Action `processImage` to Hasura metadata on install

---

## §11 Docs

- `plugins-pro/paid/nself-image/README.md` — install guide, config reference, MinIO setup
- `web/docs` plugin catalog entry (created during nFamily v1.2.0 docs sprint)
- `plugins-pro/paid/nself-image/.github/wiki/` — op reference, MinIO IAM policy example, performance tuning

---

## §12 Bundle Classification

**Bundle:** `nfamily` ($0.99/mo)

nself-image is part of the nFamily bundle. It also benefits nChat (attachment WebP conversion) but is not part of the nChat bundle — nChat attachment plugin uses nself-image as a service-role caller. Any project with an nFamily license gains nself-image; nChat installations that want image processing must purchase nFamily or ɳSelf+.

**Shared-pro note:** No new bundle is needed. nself-image fits cleanly in nFamily. If future demand from non-nFamily apps is high, promotion to a standalone "media" bundle can be proposed in a future STORM phase.

**F06 update required** (post-P98, SPORT regeneration): add `nself-image` to `nfamily` bundle entry.

---

## §13 Security Notes

- All traffic on internal network only (`127.0.0.1:3827`). Nginx proxies external requests.
- HMAC auth (`IMAGE_PROCESSING_SHARED_SECRET`) required on all `/process` calls. Reject any request without valid HMAC.
- `strip_exif` recommended as default for all user-uploaded images — removes GPS metadata, device info, embedded thumbnails. Consider auto-applying as security default (open question).
- MinIO access scoped to declared input/output buckets via least-privilege IAM policy. Service does not have bucket-creation or bucket-deletion permissions.
- No internet egress required. All I/O is internal (MinIO on loopback/VPC).
- `IMAGE_PROCESSING_MAX_INPUT_BYTES=52428800` (50MB) caps memory usage per job. Requests exceeding this limit are rejected with 413 before MinIO download begins.
- Concurrent job cap (`IMAGE_PROCESSING_MAX_CONCURRENT=8`) prevents OOM from parallel large-image jobs.

---

## §14 CI Matrix — Go Cross-Compilation

Per SIEGE T2-15: nself-image ships per-architecture binaries, so CI builds a GOOS/GOARCH matrix. Go cross-compiles natively; no Docker-based cross toolchain is needed while the build stays `CGO_ENABLED=0` (see §1 for the encoder decision that could change that).

### Toolchain

- **Go toolchain:** the version pinned in `go.mod` (the repo standard); no beta or MSRV leg.
- **Build:** `CGO_ENABLED=0 GOOS=<os> GOARCH=<arch> go build -mod=vendor -trimpath`

### Build matrix

| GOOS/GOARCH | OS | Arch | Tier | CI trigger |
|---|---|---|---|---|
| `linux/amd64` | Linux (Hetzner CX-series) | amd64 | Primary | PR + release |
| `linux/arm64` | Linux (Hetzner CAX-series) | arm64 | Primary | PR + release |
| `darwin/amd64` | macOS (Intel) | amd64 | Secondary | Release only |
| `darwin/arm64` | macOS (Apple Silicon) | arm64 | Secondary | PR + release (local dev) |
| `windows/amd64` | Windows | amd64 | Future | Release only (when Windows support ships) |

### GitHub Actions workflow excerpt

```yaml
strategy:
  matrix:
    include:
      - { goos: linux, goarch: amd64 }
      - { goos: linux, goarch: arm64 }
      - { goos: darwin, goarch: arm64 }

steps:
  - uses: actions/checkout@v4
  - uses: actions/setup-go@v5
    with:
      go-version-file: go.mod
  - run: CGO_ENABLED=0 GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} go build -mod=vendor -trimpath ./...
  - run: go vet -mod=vendor ./... && go test -mod=vendor ./...   # linux/amd64 leg only
```

**Note:** Tests run on the native runner leg only. Other targets are compile-checked.

### Cross-compilation CI trigger rules

- **PR:** `linux/amd64` + `linux/arm64` + `darwin/arm64` (build-only)
- **Release (tag push):** Full matrix including Windows

---

## §15 Shippability (v1.2.0)

**Target version:** v1.2.0

**Prerequisite gate:**
- nFamily bundle ratified and in production (v1.1.0 milestone)
- MinIO optional service stable and documented
- Hasura Actions custom-service pattern documented in `web/docs`

**Pre-ship checklist:**
- [ ] Go binary builds cleanly for `linux/amd64` and `linux/arm64`
- [ ] All unit + integration tests pass
- [ ] perf baseline verified on CX21 (256×256 WebP < 100ms)
- [ ] `nself doctor` check passes on clean install
- [ ] F06 SPORT entry added (post-code-verification)
- [ ] Marketing: nFamily bundle page at `nself.org/products/nfamily` updated
- [ ] Migration guide for Sharp users published

**Not blocked on:** Stripe, nchat-sdk NPM token, or any T1-series user actions.

---

## Open Questions (deferred to implementation sprint)

1. AVIF encoding slow (~300ms for 256x256 on CX21). Cap AVIF at 1MP max input, or make it opt-in via `"allow_slow": true`?
2. Should `strip_exif` be auto-applied to all user uploads as a security default (EXIF GPS data in user photos is a privacy risk)?
3. Async processing with job queue plugin, or sync resize-on-request? Current spec assumes sync. Async via `job-queue` plugin is a v2 enhancement.
4. Preserve original + processed variant, or replace? Current spec: caller specifies both `input.key` and `output.key` explicitly, so both behaviors are supported.
