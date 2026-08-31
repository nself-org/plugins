# ɳSelf HIPAA Plugin

**Port:** 3212 (⚠️ conflicts with `admin-api` plugin — see Port Conflict below)
**Bundle:** Unbundled (standalone, requires ɳSelf+ license)
**License flag:** `NSELF_HIPAA=true`

The HIPAA plugin provides Health Insurance Portability and Accountability Act (HIPAA) compliance tooling for nSelf deployments handling Protected Health Information (PHI). It includes three core capabilities: a PHI column registry, a PHI access audit log with 6-year retention enforcement, and de-identification/tokenization of PHI data.

## Features

### PHI Column Registry

Register which table.column combinations contain PHI so the audit log and de-identification engine know what to protect.

```http
GET    /hipaa/phi-columns
POST   /hipaa/phi-columns
DELETE /hipaa/phi-columns/{id}
```

Supported `phi_category` values: `name`, `dob`, `ssn`, `mrn`, `address`, `phone`, `email`, `other`
Supported `de_id_method` values: `mask` (default), `tokenize`, `redact`

### PHI Access Audit Log

Every access to registered PHI columns is logged with accessor identity, purpose, and timestamp. Logs are retained for a minimum of 6 years per 45 CFR § 164.530(j).

```http
GET  /hipaa/audit-log          # paginated access log
GET  /hipaa/audit-log/export   # CSV export for auditors (phi:admin role required)
```

**Retention enforcement** is implemented at the Postgres row-level security layer (`migration 002`): `DELETE` is blocked until `retain_until < CURRENT_DATE`. The `retain_until` column is a generated column: `(accessed_at + INTERVAL '6 years')::DATE STORED`.

### De-Identification

De-identify datasets in bulk using HIPAA Safe Harbor method, or tokenize individual PHI values.

```http
POST /hipaa/deidentify          # bulk de-id rows from a registered table
POST /hipaa/tokenize            # tokenize a single PHI value
GET  /hipaa/tokenize/{token}    # detokenize (phi:detokenize role required)
```

Safe Harbor masking by category:
- **ssn** → `XXX-XX-NNNN` (last 4 preserved)
- **dob** → `YYYY-XX-XX` (year preserved only)
- **name** → `J*** D**` (first char per word preserved)
- **phone** → `XXX-XXX-NNNN`
- **email** → `j**n@example.com`
- **address** → `123 [STREET MASKED]`
- **mrn** → leading chars masked, last 4 preserved

### BAA Record Management (Enterprise)

Business Associate Agreement tracking (requires `NSELF_HIPAA_BAA=true`):

```http
GET  /hipaa/baa
POST /hipaa/baa/request
POST /hipaa/baa/activate
POST /hipaa/baa/terminate
```

## Installation

```bash
nself plugin install hipaa
```

Verify:

```bash
nself plugin list | grep hipaa
```

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | Postgres connection string |
| `NSELF_HIPAA` | Yes | `false` | Enable PHI registry + audit log + de-id endpoints |
| `NSELF_HIPAA_BAA` | No | `false` | Enable BAA management endpoints |
| `HIPAA_PLUGIN_PORT` | No | `3212` | HTTP listen port |
| `HIPAA_PLUGIN_HOST` | No | `0.0.0.0` | HTTP listen host |
| `HIPAA_API_KEY` | No | — | Shared secret for X-HIPAA-API-Key header auth |
| `NSELF_HIPAA_VAULT` | No | `false` | Use Vault Transit for tokenization instead of in-memory store |

## Port Conflict

⚠️ **Port 3212 is also claimed by the `admin-api` plugin.** If both plugins are installed on the same nSelf instance, one must be reconfigured. Set `HIPAA_PLUGIN_PORT` to an available port from the F10 port registry. This conflict is tracked in [F10-PORT-REGISTRY.md](.opencode/phases/sport/F10-PORT-REGISTRY.md) and will be resolved in a future F10 registry task.

## Multi-Tenant Isolation

All `np_phi_*` tables include `source_account_id TEXT NOT NULL DEFAULT 'primary'`. Every query is scoped to the caller's account via the `X-Source-Account-ID` header (falls back to `source_account_id` query param, then `primary`).

## Roles

| Header value (`X-HIPAA-Role`) | Permissions |
|---|---|
| `phi:read` | Read audit log |
| `phi:admin` | Read + export audit log; unregister PHI columns |
| `phi:detokenize` | Detokenize PHI tokens |
| `hipaa:admin` | All of the above + bypass RLS for post-retention purge |

## Migrations

| File | Description |
|---|---|
| `001_hipaa_tables.sql` | PHI column registry, PHI audit log (with 6-year generated `retain_until`), BAA records |
| `002_retention_rls.sql` | Row-level security enforcing 6-year DELETE block on `np_phi_audit_log` |
