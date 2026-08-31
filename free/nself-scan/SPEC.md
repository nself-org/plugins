# Plugin Spec: nself-scan (file-virus-scanning)

**Plugin name:** `nself-scan`
**Display name:** File Virus & Content Scanning
**Bundle:** nFamily + nChat (ClamAV free tier); CSAM extension deferred — partner-agreement tier (see §14)
**Language:** Go (ClamAV bridge via TCP socket to `clamd`)
**Port:** 3829
**Repo:** `plugins-pro/paid/nself-scan/`
**Target version:** v1.1.0
**Status:** Planned (P98)
**Spec source:** `.claude/docs/plugin-specs/file-virus-scanning.md` + P98 HF-2 amendment (04.T21)

---

## §1 Overview

Server-side file scanning hook for the MinIO storage pipeline. Acts as a pre-storage filter that
runs on every file upload event via MinIO bucket notifications. Three scanning layers are defined:

1. **MIME magic-byte validation** — inspects file bytes to detect declared vs. actual Content-Type
   mismatches. Always runs; always free.
2. **ClamAV malware/virus scanning** — checks file content against ClamAV signature database.
   **ALWAYS FREE** — this capability is never paywalled. Security-Always-Free Doctrine applies.
3. **CSAM hash detection** — content hash matched against a photoDNA-compatible hash DB.
   **PARTNER-TIER — deferred to v2.** Requires a signed Microsoft PhotoDNA Partner Agreement (or
   equivalent NCMEC program enrollment) before it can ship. This is NOT bundled in any $0.99 tier.
   See §14 for the tier model and partner-agreement requirement.

**Why now:** Three upload surfaces in consumer apps lack server-side scanning:
- `nfamily` photo and board-letter uploads (nFamily bundle consumers)
- `nchat` file attachment pipeline (S13-10 — required for "E2E safe" marketing claim)
- Ummat P3: board letter uploads (M2-07), government ID images (S14-10)

Current workaround is MIME-header validation only — insufficient against crafted uploads.

**Security-Always-Free Doctrine compliance (HF-2):** ClamAV virus/malware scanning is part of
nSelf's free core security hardening. It ships without a license check, runs on every nSelf deploy
that has `clamav-daemon` installed, and is documented in `nself doctor --deep` scan output. No
part of ClamAV scanning is behind a bundle paywall.

---

## §2 Architecture

### Service shape

Go binary + ClamAV (`clamd`) sidecar. Docker Compose adds a `clamd` service on internal port 3310
(not in the external port registry — internal-only binding to `127.0.0.1`).

```
[MinIO webhook] → [nself-scan :3829] ──→ [magic-byte MIME check]
                                      ──→ [clamd TCP :3310 — ClamAV scan]
                                      ──→ [CSAM hash lookup — disabled by default]
                                      ──→ [Hasura result write / webhook response]
```

### ClamAV integration

ClamAV connects via TCP socket to `clamd` at `127.0.0.1:3310`. I/O-bound — no subprocess fork
per scan. The Go client sends the file stream using the `INSTREAM` clamd protocol command.
`freshclam` runs as a sidecar on a 6-hour cron managed by the plugin's Compose block to keep
virus definitions current. Stale definitions (>48 hours) trigger a `clamav_db_age_hours` alert.

### MinIO webhook

Plugin exposes `POST /scan/webhook/minio` with HMAC-SHA256 request signature verification. MinIO
bucket notifications route to this endpoint on `s3:ObjectCreated:*` events. The plugin queues the
event (`np_scan.pending_events`), processes asynchronously, and writes the result to
`np_scan.results`. Files with `overall_verdict = 'blocked'` are deleted from MinIO via the MinIO
S3 API and the upload originator receives an error.

---

## §3 Data Model

**Schema:** `np_scan` · **Role:** `np_scan`

### Multi-Tenant Convention Wall compliance

Per PPI Hard Rule, both isolation mechanisms are applied because scan results are scoped both
per-app (multi-app isolation) and per-Cloud-customer (Cloud multi-tenancy):

- `source_account_id TEXT NOT NULL DEFAULT 'primary'` — multi-app isolation within one deploy
- `tenant_id UUID` (nullable) + Hasura row filter `{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}` — Cloud customer isolation

**NEVER** use `source_account_id` to separate Cloud customers. **NEVER** use `tenant_id` for multi-app isolation within one deploy.
**Enforcement:** `nself doctor --deep` check `PERM-RLS-01` verifies both columns and Hasura row filters are present on all `np_scan.*` tables.

```sql
-- Scan results
CREATE TABLE np_scan.results (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id            UUID,
  source_account_id    TEXT NOT NULL DEFAULT 'primary',
  file_path            TEXT NOT NULL,           -- MinIO object path
  file_size_bytes      BIGINT,
  mime_detected        TEXT,                    -- magic-byte result
  mime_claimed         TEXT,                    -- Content-Type header
  clamav_verdict       TEXT NOT NULL DEFAULT 'pending', -- pending|clean|threat|error
  clamav_threat_name   TEXT,
  csam_verdict         TEXT NOT NULL DEFAULT 'skipped', -- skipped|clean|match|error
  csam_hash_db_version TEXT,
  overall_verdict      TEXT NOT NULL DEFAULT 'pending', -- pending|allowed|blocked
  blocked_reason       TEXT,
  scanned_at           TIMESTAMPTZ,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_scan_results_tenant  ON np_scan.results (tenant_id, source_account_id, created_at DESC);
CREATE INDEX idx_scan_results_verdict ON np_scan.results (overall_verdict, tenant_id);
CREATE INDEX idx_scan_results_path    ON np_scan.results (file_path, tenant_id);

-- MinIO webhook event queue
CREATE TABLE np_scan.pending_events (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         UUID,
  source_account_id TEXT NOT NULL DEFAULT 'primary',
  minio_event       JSONB NOT NULL,
  received_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at      TIMESTAMPTZ
);
```

**Rollback:** `DROP SCHEMA np_scan CASCADE; DROP ROLE np_scan;`

---

## §4 API

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| `POST` | `/scan/file` | Hasura Action JWT | Synchronous scan — files < 10 MB |
| `POST` | `/scan/async` | Hasura Action JWT | Queue scan, return job ID |
| `GET` | `/scan/result/:id` | Hasura Action JWT | Poll async result |
| `POST` | `/scan/webhook/minio` | HMAC-SHA256 sig | MinIO bucket notification relay |
| `GET` | `/health` | none | Liveness; includes clamd ping + db-age |

**Rate limits:** 200 req/min per tenant · Synchronous scan timeout: 30 s

### Hasura Remote Schema

Three queries/mutations exposed:

| Operation | Type | Notes |
|-----------|------|-------|
| `scanFile` | Mutation | Synchronous scan; returns `{verdict, threatName, mimeDetected}` |
| `getScanResult` | Query | Poll async job by ID |
| `getScanConfig` | Query | Admin: current clamd status + db age |

---

## §5 Bundle Classification

| Layer | Tier | License check | Paywall |
|-------|------|---------------|---------|
| MIME magic-byte validation | **Free — always runs** | None | None |
| ClamAV virus/malware scanning | **Free — Security-Always-Free Doctrine** | None | None |
| CSAM hash detection (PhotoDNA) | **Partner-agreement tier — deferred v2** | Partner agreement required | Not bundled |

**ClamAV is free.** `nself-scan` (ClamAV layer) is a free plugin — it does not require a pro
license and does not count against bundle entitlement. It ships in the free plugin set
(`plugins/free/nself-scan/`) once the ClamAV Compose block is validated. The paid entry in
`plugins-pro/paid/nself-scan/` governs the CSAM extension path only.

**CSAM is not a bundle add-on.** The CSAM hash-detection capability (`nself-scan-csam`) requires
a Microsoft PhotoDNA Partner Agreement or NCMEC enrollment. It cannot ship as a $0.99 add-on
because the legal prerequisite is a bilateral commercial agreement, not a license key. Operators
wishing to enable CSAM detection must obtain their own partner agreement and supply the hash DB
path via `SCAN_CSAM_HASH_DB_PATH`. This is documented in §14.

---

## §6 Environment Variables

| Variable | Required | Default | Notes |
|----------|----------|---------|-------|
| `DATABASE_URL` | Yes | — | Postgres connection |
| `SCAN_PORT` | No | `3829` | HTTP listen port |
| `SCAN_CLAMD_ADDR` | No | `127.0.0.1:3310` | clamd TCP address |
| `SCAN_CLAMD_TIMEOUT_SECONDS` | No | `30` | Per-scan timeout |
| `SCAN_CSAM_ENABLED` | No | `false` | Enable CSAM hash check — requires partner DB |
| `SCAN_CSAM_HASH_DB_PATH` | No | — | Path to CSAM hash DB (operator-supplied) |
| `SCAN_CSAM_PROVIDER` | No | `none` | `photodna` or `ncmec` — inert when CSAM disabled |
| `SCAN_MAX_SYNC_SIZE_MB` | No | `10` | Max file size for synchronous endpoint |
| `SCAN_MINIO_WEBHOOK_SECRET` | No | — | HMAC key for MinIO webhook verification |
| `SCAN_BLOCKED_MIME_TYPES` | No | `application/x-executable,application/x-msdownload` | Comma-separated block list |
| `SCAN_FRESHCLAM_INTERVAL_HOURS` | No | `6` | ClamAV definition update frequency |

---

## §7 Permissions Matrix

| Role | Scan file | View own results | View all results | Configure scanner | View threat log |
|------|-----------|-----------------|-----------------|-------------------|-----------------|
| admin | ✓ | ✓ | ✓ | ✓ | ✓ |
| user | ✓ (own uploads) | own only | ✗ | ✗ | ✗ |
| service | ✓ | own only | ✗ | ✗ | ✗ |

Hasura row-level filter on `np_scan.results`:
```json
{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}
```

---

## §8 Doctor Checks

| Check ID | Condition | Severity | Notes |
|----------|-----------|----------|-------|
| `SCAN-DEPS-01` | `clamd` reachable at configured address | CRITICAL | Plugin non-functional without clamd |
| `SCAN-DEPS-02` | ClamAV virus DB age < 48 h | WARNING | Stale definitions reduce detection rate |
| `SCAN-DEPS-03` | `SCAN_MINIO_WEBHOOK_SECRET` is non-empty | WARNING | Unsigned webhooks accepted — security risk |
| `SCAN-CSAM-01` | If `SCAN_CSAM_ENABLED=true`, hash DB file exists and is readable | ERROR | CSAM enabled but DB missing |
| `SCAN-CSAM-02` | If `SCAN_CSAM_ENABLED=true`, partner-agreement attestation env var present | WARNING | Legal compliance reminder |
| `SCAN-PERF-01` | Async queue depth < 1000 pending events | WARNING | Scan backlog indicates throughput issue |

---

## §9 Loader Hook

The nself CLI loader verifies:

1. `clamav` and `clamav-daemon` are available in the Compose environment (via Docker image base or
   system package)
2. `clamd.conf` is generated from the plugin's config template
3. `freshclam.conf` is generated with the configured update interval
4. MinIO bucket notification webhook is registered on the target bucket(s) via the MinIO API
5. Hasura Remote Schema is added after first successful `/health` response from the plugin service

**Install command:**
```bash
nself plugin install nself-scan
```

No license key required. Plugin installs to the free tier and activates ClamAV scanning
immediately on `nself start`.

---

## §10 Observability

**Metrics (Prometheus):**

| Metric | Labels | Notes |
|--------|--------|-------|
| `scan_files_total` | `verdict`, `scan_type` | Counter — files scanned by outcome |
| `scan_duration_seconds` | `scan_type` | Histogram — `mime`, `clamav`, `csam` |
| `scan_threats_detected_total` | `threat_category` | Counter — blocked files |
| `clamav_db_age_hours` | — | Gauge — hours since last freshclam update |
| `scan_queue_depth` | — | Gauge — pending async scan events |

**Alerts:**

| Condition | Level |
|-----------|-------|
| `clamav_db_age_hours > 48` | WARNING — definitions stale |
| `scan_threats_detected_total` rate > 0.1% of total | INFO — elevated threat rate |
| `clamd` unreachable | CRITICAL — scanning halted |

**Logs:** Structured JSON. File paths are SHA-256 hashed before logging to avoid PII exposure.
Fields: `{file_path_hash, mime_detected, clamav_verdict, csam_verdict, overall_verdict, duration_ms}`.

**Traces:** OpenTelemetry spans on `mime_check`, `clamav_scan`, `csam_lookup` (if enabled).

---

## §11 Integration Points

- **Depends on:** `object-storage` plugin (MinIO — receives webhook on `s3:ObjectCreated:*` events)
- **Consumed by:** `nchat` attachment pipeline, `nfamily` photo + board-letter uploads, `photos`
  plugin, Ummat P3 surfaces (M2-07 board letters, S14-10 government ID images, S13-10 nChat
  attachments)
- **ClamAV sidecar:** `clamd` service in Compose block — port `3310` (internal only, not in
  external port registry)
- **Does not call external services by default.** All scanning is self-hosted. CSAM hash DB is
  operator-supplied; no external API calls in default config.

---

## §12 Testing Plan

**Unit:**
- Magic-byte MIME detection with EICAR test file and a mismatched `Content-Type: image/png`
- ClamAV TCP socket mock — inject `EICAR` response, verify `blocked` verdict
- CSAM hash lookup mock — inject hash match, verify `blocked` verdict (gate behind
  `SCAN_CSAM_ENABLED=true` test flag only)
- MIME blocklist enforcement (`application/x-executable` → blocked immediately, no ClamAV call)

**Integration:**
- Docker Compose with real `clamd` (CI uses `clamav/clamav:stable`)
- Upload a clean file → expect `overall_verdict = 'allowed'`
- Upload EICAR test string → expect `overall_verdict = 'blocked'`, `clamav_verdict = 'threat'`,
  `clamav_threat_name = 'EICAR-Test-Signature'`
- Corrupted MinIO webhook HMAC → expect 401

**E2E:**
- Upload file via MinIO → webhook fires → async scan completes → result in Hasura → file blocked
  from downstream serving

---

## §13 Rollout

- **Feature flag:** `SCAN_PLUGIN_ENABLED=true` (default `false` — ClamAV dependency must exist)
- **Migration:** Schema-only. Existing uploads are not retroactively scanned. Scan applies to new
  uploads from activation forward.
- **MinIO configuration:** Operator must enable bucket notifications on target buckets, or run
  `nself plugin configure nself-scan` which auto-registers the webhook via the MinIO API.
- **Backwards compat:** Plugin is fully additive. No existing API contracts change on install.

---

## §14 Tier Model — CSAM Partner Agreement, Not Bundle Paywall

This section documents the HF-2 amendment to the nself-scan tier model (04.T21).

### ClamAV virus/malware scanning — ALWAYS FREE

ClamAV scanning is a baseline security hardening feature. Per the **Security-Always-Free
Doctrine** (PPI Hard Rule), no security feature in nSelf is paywalled. ClamAV virus scanning:

- Ships in the free plugin tier alongside `plugins/free/` entries
- Requires no license key
- Activates automatically on `nself start` when ClamAV is installed
- Is surfaced in `nself doctor` output as a security health check
- Is documented in public security docs as a free, default capability

### CSAM hash detection — Partner-Agreement Tier (deferred to v2)

CSAM (Child Sexual Abuse Material) hash-matching via PhotoDNA or NCMEC is a distinct legal and
technical capability from ClamAV virus scanning. It is NOT paywalled behind the $0.99 nFamily or
nChat bundles for the following reasons:

1. **Microsoft PhotoDNA Partner Agreement required.** PhotoDNA API access requires a bilateral
   commercial agreement with Microsoft, not a license key purchase. An operator cannot unlock
   CSAM detection by subscribing to a bundle — they must independently obtain the partner
   agreement.

2. **NCMEC enrollment alternative.** NCMEC (National Center for Missing and Exploited Children)
   offers a hash-sharing program for qualifying platforms. Enrollment requires a signed agreement
   and organizational vetting, not a payment.

3. **Legal jurisdiction complexity.** Mandatory CSAM reporting requirements vary by jurisdiction.
   A self-hosted operator enabling CSAM detection takes on legal obligations that cannot be
   transferred via a plugin subscription. nSelf cannot and should not make this decision for
   operators.

### v1.x behavior (this release)

`SCAN_CSAM_ENABLED` defaults to `false`. The CSAM code path is present but inert. The plugin
ships with full documentation of the partner-agreement requirement. Operators who independently
obtain PhotoDNA or NCMEC access can set `SCAN_CSAM_ENABLED=true` and supply `SCAN_CSAM_HASH_DB_PATH`.

### v2 prerequisite

`nself-scan-csam` will be introduced as a separate plugin entry in v2 once nSelf has completed a
partner agreement or NCMEC enrollment. This plugin will be classified as:
- Free for operators who supply their own partner-obtained hash DB
- NOT a paid bundle add-on — partner agreement supersedes any license-key model

### Compliance note

Operators enabling CSAM scanning are responsible for:
- Obtaining a valid partner agreement or NCMEC enrollment
- Complying with local mandatory reporting laws
- Supplying and maintaining an up-to-date hash DB

nSelf provides the scanning infrastructure; legal compliance is operator responsibility.

---

## §15 Documentation

| Surface | Path |
|---------|------|
| Plugin wiki | `plugins-pro/.github/docs/nself-scan.md` |
| Public docs | `web/docs/src/content/plugins/nself-scan.mdx` |
| Plugin README | `plugins-pro/paid/nself-scan/README.md` |
| SPORT | F04 plugin count +1; F06 add `nself-scan` to nFamily + nChat bundles (free tier); note CSAM deferred |

---

## Open Questions

1. **CSAM hash DB sourcing (user decision required):** PhotoDNA vs. NCMEC. NCMEC is free for
   qualifying platforms; PhotoDNA requires a commercial partner agreement. Recommendation: pursue
   NCMEC enrollment for v2. Decision must be recorded in `memory/decisions.md` before
   `SCAN_CSAM_ENABLED` goes production-default anywhere.

2. **freshclam management:** Plugin-managed (recommended — runs sidecar cron) vs. operator-
   managed. Current spec defaults to plugin-managed on a 6-hour interval. Operator can override
   via `SCAN_FRESHCLAM_INTERVAL_HOURS=0` to disable the built-in cron.

3. **Retroactive scan API:** Should `nself plugin configure nself-scan --backfill` trigger async
   scans on existing unscanned objects? Deferred to v1.1 patch — requires MinIO object listing
   and a rate-limited batch worker.
