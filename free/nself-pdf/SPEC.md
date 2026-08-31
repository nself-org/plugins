# Plugin Spec: nself-pdf (pdf-generation)

**Bundle:** shared (available to all bundles; cross-product use recommended via ɳSelf+)
**Tier:** ɳSelf+ $3.99/mo or any single bundle at $0.99/mo
**Language:** Go (chromedp — headless Chromium)
**Port:** 3828
**Repo:** `plugins-pro/paid/nself-pdf/`
**Target version:** v1.1.0
**Status:** Planned (P98)

---

## §1 Overview

Server-side PDF generation from HTML templates. A Handlebars or Nunjucks template combined with
JSON data produces a PDF blob written to MinIO. Exposes a Hasura Action (`generatePdf`) for
trigger-based generation from any nSelf app.

**Why this exists:** Ummat P3 Sprint 11 (donation receipts), Sprint M2 (approval confirmation
docs), and Sprint 12 GDPR export (S12-83) each need PDF output. Without nself-pdf, every feature
reaches for Puppeteer or a PDF SaaS independently. Current workaround emits HTML-only emails and
JSON-only GDPR exports — both are user-facing quality gaps that become P4 requirements.

---

## §2 Architecture

### Renderer choice: chromedp (Go — headless Chromium)

**Decision: chromedp over wkhtmltopdf.**

Rationale:
- wkhtmltopdf is deprecated upstream (last release 2020, no active maintainer, known CVEs).
- chromedp drives a real Chromium binary — full CSS3, flexbox, grid, custom fonts, SVG.
- Donation receipts and GDPR summaries require precise table layout; wkhtmltopdf flex/grid bugs
  cause rendering drift.
- wkhtmltopdf ships a patched Qt WebKit (~100 MB binary footprint). Chromium is already present
  in most CI and server environments; the `chromedp` Go library is ~10 KB.
- Trade-off: chromedp cold-start is ~1–2 s. Acceptable for async/batch generation. Not acceptable
  for synchronous user-facing requests — callers must enqueue, not block.

### Service shape

Runs as a Custom Service (`CS_N`). Exposes HTTP on `127.0.0.1:3828`. Chromium launched once at
startup as a long-running headless process and reused across requests (no per-job cold start after
the first).

```
[Hasura Action / direct HTTP] → [nself-pdf :3828] → [chromedp → Chromium binary]
                                                   → [MinIO output bucket]
```

### Template engine

Two modes per request:

| Mode | Field value | Notes |
|------|-------------|-------|
| **Handlebars** | `"engine": "handlebars"` | Loops, conditionals, partials |
| **Nunjucks** | `"engine": "nunjucks"` | Jinja2-compatible; richer filters |

Templates stored in MinIO `nself-pdf-templates/` bucket. Template slug resolves to
`nself-pdf-templates/{slug}.html`.

---

## §3 API Endpoints

### POST /pdf/generate

Enqueue or execute a PDF generation job. Response includes the MinIO output key.

**Request:**
```json
{
  "template": "donation-receipt",
  "engine": "handlebars",
  "data": {
    "donor_name": "Ahmad Abdullah",
    "amount": "25.00",
    "currency": "USD",
    "date": "2026-04-28",
    "org_name": "Masjid Al-Noor"
  },
  "output": {
    "bucket": "nself-pdf-output",
    "key": "receipts/2026/04/receipt-xyz.pdf"
  },
  "options": {
    "format": "A4",
    "margin": {"top": "20mm", "bottom": "20mm", "left": "15mm", "right": "15mm"}
  }
}
```

**Response (200):**
```json
{
  "output_key": "receipts/2026/04/receipt-xyz.pdf",
  "size_bytes": 48200,
  "pages": 1,
  "duration_ms": 380
}
```

**Error (422):**
```json
{"error": "template not found", "template": "donation-receipt"}
```

### GET /pdf/status/:id

Returns the status of a queued job (when `PDF_AUDIT_ENABLED=true`).

**Response:**
```json
{
  "id": "uuid",
  "status": "done",
  "output_path": "receipts/2026/04/receipt-xyz.pdf",
  "created_at": "2026-04-28T12:00:00Z",
  "completed_at": "2026-04-28T12:00:00.380Z"
}
```

### POST /pdf/render

Inline render — returns PDF bytes directly (`Content-Type: application/pdf`). No MinIO write.
For small one-off PDFs where storage is not needed.

### GET /templates

Lists available template slugs from the templates bucket.

**Response:**
```json
{"templates": ["donation-receipt", "gdpr-export-summary", "audit-report"]}
```

### GET /health

```json
{"status": "ok", "chromium": "ready", "version": "0.1.0"}
```

---

## §4 Data Model

### Optional audit table (`PDF_AUDIT_ENABLED=true`)

Audit table is **off by default** to keep the service stateless. Enable with
`PDF_AUDIT_ENABLED=true`.

```sql
CREATE SCHEMA IF NOT EXISTS np_pdf;

CREATE TABLE np_pdf.jobs (
  id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           UUID,
  tenant_id         UUID,
  source_account_id TEXT          NOT NULL DEFAULT 'primary',
  template          TEXT,         -- template slug or NULL for HTML-direct
  status            TEXT          NOT NULL DEFAULT 'pending',
    -- 'pending' | 'done' | 'failed'
  output_path       TEXT,
  error             TEXT,
  created_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
  completed_at      TIMESTAMPTZ
);

CREATE INDEX ON np_pdf.jobs (user_id, created_at DESC);
CREATE INDEX ON np_pdf.jobs (tenant_id, status);
```

> Audit table enabled via `PDF_AUDIT_ENABLED=true`. Default off.

### Environment variables

```
PDF_GENERATION_PORT=3828
PDF_GENERATION_MINIO_ENDPOINT=
PDF_GENERATION_MINIO_ACCESS_KEY=
PDF_GENERATION_MINIO_SECRET_KEY=
PDF_GENERATION_TEMPLATES_BUCKET=nself-pdf-templates
PDF_GENERATION_OUTPUT_BUCKET=nself-pdf-output
PDF_GENERATION_SHARED_SECRET=
PDF_GENERATION_MAX_CONCURRENT=4
PDF_GENERATION_TIMEOUT_SECONDS=30
PDF_AUDIT_ENABLED=false
DATABASE_URL=                  # required only when PDF_AUDIT_ENABLED=true
```

---

## §5 Permissions / Hasura Role Matrix

| Role | Permission |
|------|-----------|
| `user` | Submit PDF generation jobs scoped to their own `user_id` |
| `admin` | Submit jobs for any user; view all job statuses |
| `anonymous` | No access |
| `service` | Full access (internal plugin-to-plugin calls via shared secret) |

**Row-level security note:** `np_pdf.jobs` enforces `user_id = X-Hasura-User-Id` for the `user`
role via Hasura row permission `{"user_id": {"_eq": "X-Hasura-User-Id"}}`.

**Service-to-service auth:** HMAC shared secret (`PDF_GENERATION_SHARED_SECRET`). All HTTP calls
carry `Authorization: Bearer <HMAC-SHA256>` header. No unauthenticated access to `/pdf/generate`
or `/pdf/render`.

---

## §6 Multi-Tenant Convention

This plugin uses **both** nSelf multi-tenancy mechanisms per the Multi-Tenant Convention Wall:

| Column | Mechanism | Applies to |
|--------|-----------|-----------|
| `source_account_id TEXT NOT NULL DEFAULT 'primary'` | Multi-App Isolation | Separates PDF jobs across independent apps on one nSelf deploy |
| `tenant_id UUID` | Cloud Multi-Tenancy | Separates paying Cloud customers in nSelf Cloud SaaS |

**NEVER** use `source_account_id` to separate Cloud customers.
**NEVER** use `tenant_id` for multi-app isolation within one deploy.

Hasura row filter for Cloud multi-tenancy:
```json
{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}
```

**Enforcement:** `nself doctor --deep` check `PERM-RLS-01` verifies both columns and Hasura row filters are present on all `np_pdf.*` tables.

---

## §7 Plugin Manifest

```json
{
  "name": "nself-pdf",
  "display_name": "PDF Generation",
  "version": "0.1.0",
  "language": "go",
  "bundle": "shared",
  "visibility": "public",
  "tier": "pro",
  "port": 3828,
  "service_type": "custom_service",
  "env_vars": [
    "PDF_GENERATION_PORT",
    "PDF_GENERATION_MINIO_ENDPOINT",
    "PDF_GENERATION_MINIO_ACCESS_KEY",
    "PDF_GENERATION_MINIO_SECRET_KEY",
    "PDF_GENERATION_TEMPLATES_BUCKET",
    "PDF_GENERATION_OUTPUT_BUCKET",
    "PDF_GENERATION_SHARED_SECRET",
    "PDF_GENERATION_MAX_CONCURRENT",
    "PDF_GENERATION_TIMEOUT_SECONDS",
    "PDF_AUDIT_ENABLED"
  ],
  "dependencies": {
    "required": ["minio"],
    "optional": ["database"]
  },
  "hasura_actions": [
    {
      "name": "generatePdf",
      "endpoint": "http://127.0.0.1:3828/pdf/generate"
    }
  ],
  "system_packages": [
    "chromium-browser"
  ],
  "doctor_checks": [
    "PDF-DEPS-01"
  ]
}
```

---

## §8 Doctor Dependency Check (PDF-DEPS-01)

**Check ID:** `PDF-DEPS-01`
**Trigger:** `nself doctor --deep` (runs automatically at install + on every `nself doctor`)
**Severity:** CRITICAL (blocks `nself start` with pdf plugin when binary not found)

### What the check does

```bash
which chromium-browser 2>/dev/null || \
which google-chrome 2>/dev/null || \
which chromium 2>/dev/null || \
which wkhtmltopdf 2>/dev/null
```

If none found: emit CRITICAL finding and block startup.

### Error message shown to operator

```
[PDF-DEPS-01] CRITICAL: PDF plugin requires Chromium or wkhtmltopdf.

  None of the following binaries were found:
    chromium-browser, google-chrome, chromium, wkhtmltopdf

  Install options:
    Ubuntu/Debian:  apt install chromium-browser
    Hetzner CX23:   apt install chromium-browser
    macOS (dev):    brew install --cask chromium
    Manual:         https://www.chromium.org/getting-involved/download-chromium

  Note: wkhtmltopdf is a fallback option but is deprecated upstream (last
  release 2020). Chromium is strongly recommended for production use.
```

### Plugin install flow

`nself plugin install nself-pdf` automatically runs the system package install step:

```go
// cli/internal/plugins/install.go (future implementation)
// After binary extraction, runs:
if runtime.GOOS == "linux" {
    exec.Command("apt-get", "install", "-y", "chromium-browser").Run()
} else if runtime.GOOS == "darwin" {
    exec.Command("brew", "install", "--cask", "chromium").Run()
}
// Falls back gracefully if apt/brew not available — operator installs manually.
```

---

## §9 Competitive Parity

| Capability | nself-pdf | Puppeteer (self-hosted) | DocRaptor | PDFShift | WeasyPrint |
|-----------|-----------|------------------------|-----------|---------|------------|
| HTML+CSS → PDF | Yes (Chromium) | Yes (Chromium) | Yes | Yes | Yes (Python) |
| No SaaS dependency | Yes | Yes | No (SaaS) | No (SaaS) | Yes |
| MinIO output | Native | Manual | No | No | Manual |
| Hasura Action trigger | Native | Manual setup | No | No | Manual |
| Go-native | Yes | No (Node.js) | N/A | N/A | No |
| Template engine | Handlebars + Nunjucks | None built-in | None | Mustache | None |
| Self-hosted cost | Free (bundle) | Free | $15+/mo | $9+/mo | Free |
| CSS3 / flexbox / grid | Yes | Yes | Yes | Yes | Partial |

**Key differentiator:** nself-pdf is the only option with native MinIO integration, Hasura Action
wiring, and Go-native deployment — no Node.js sidecar needed. Puppeteer is the closest
alternative but requires a Node.js runtime and manual MinIO + Hasura wiring.

---

## §10 Observability

- `/metrics` endpoint: `pdf_generation_jobs_total`, `pdf_generation_duration_seconds` (histogram
  by template + engine), `pdf_generation_errors_total` (by error type)
- Structured logs: `{level, ts, template, engine, output_key, pages, size_bytes, duration_ms, error?}`
- `nself doctor` checks: port 3828 listening, Chromium process alive, MinIO buckets accessible
  (`PDF-DEPS-01`, `PDF-MINIO-01`)

---

## §11 Test Plan

| Test type | Scenario | Assertion |
|-----------|---------|-----------|
| Unit | Handlebars rendering with known data | Output HTML contains expected strings |
| Unit | Nunjucks rendering with known data | Same |
| Unit | Missing template slug | Returns 422 with `"template not found"` |
| Integration | POST /pdf/generate → MinIO write → download | PDF validates (`pdfinfo` page count = 1) |
| Integration | POST /pdf/render (inline) | Response is `application/pdf`, non-empty |
| Integration | GET /pdf/status/:id (audit on) | Returns correct status |
| Perf | A4 single-page receipt | < 2 s cold start; < 500 ms warm |
| Error | MinIO write failure | Job marked `failed`, error logged, 500 returned |
| Error | Chromium crash | Service restarts Chromium, retries once, returns 503 on second failure |
| Doctor | PDF-DEPS-01 with no binary | CRITICAL finding emitted, startup blocked |

---

## §12 Migration Path

- No schema migration needed for stateless use.
- When `PDF_AUDIT_ENABLED=true`: apply `migrations/np_pdf/001_create_jobs.sql` (DDL in §4).
- Existing apps using Puppeteer / HTML emails: replace with Hasura Action `generatePdf` call.
  Starter templates provided in `plugins-pro/paid/nself-pdf/templates/`.

---

## §13 Rollout

1. `nself plugin install nself-pdf`
   — install script provisions Chromium via `apt install chromium-browser` or `brew install --cask chromium`
2. Upload starter templates: `nself plugin run nself-pdf seed-templates`
3. Consumer apps point Hasura Action to `http://127.0.0.1:3828/pdf/generate`
4. Optional: set `PDF_AUDIT_ENABLED=true` and apply migration to enable job tracking

---

## §14 Docs to Create

- `plugins-pro/paid/nself-pdf/README.md` — install, Chromium note, template guide
- `web/docs` plugin catalog entry
- Starter templates: `plugins-pro/paid/nself-pdf/templates/donation-receipt.html`,
  `gdpr-export-summary.html`, `audit-report.html`

---

## §15 Bundle Classification

- **Target version:** v1.1.0
- **Bundle:** shared — available with any single bundle ($0.99/mo) or ɳSelf+ ($3.99/mo)
- **Tier:** pro
- **Pricing impact:** no new bundle tier needed; shared bundle slot

---

## Open Questions

1. Should Chromium be bundled in the Docker image or installed via apt at runtime?
   Bundled = reproducible, larger image (~400 MB). Runtime = smaller image, install dependency
   at deploy. Recommend bundled for production deployments.
2. Nunjucks engine requires a Go port (`nunjucks-go`) or a Node.js sidecar. Evaluate whether
   Handlebars alone covers all planned use cases (donation receipts, GDPR summaries). If yes,
   drop Nunjucks to simplify — single engine is easier to maintain.
