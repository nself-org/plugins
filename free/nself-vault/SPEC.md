# Plugin Spec: nself-vault

**Bundle:** Shared pro + `vault-cloud` internal extension
**Tier:** pro
**Version-target:** v1.1.0 (HARD DEPENDENCY for nself-stripe, social, compliance plugins)
**Port:** 3823
**Language:** Go (libsodium via CGo — XChaCha20-Poly1305)
**Status:** SPEC ONLY — no implementation code in P98

---

## §1 Overview

nSelf-managed envelope encryption KMS. App code calls `vault.encrypt(plaintext) → ciphertext` and `vault.decrypt(ciphertext) → plaintext` without holding the master key (KEK) directly. Provides key rotation with dual-key grace period, a full audit log, and a Hasura Action surface.

**Why this exists:** Decision D-P3-26 flagged a divergence risk. Three P3 features independently implement AES-256 wrappers using raw `NSELF_VAULT_KEY` env vars — Sprint 11 (`stripe_byos_key_enc`), Sprint 14 (`fl_parent_consent_record.consent_evidence_url`), and Sprint 12 (`ua_social_accounts.access_token_enc`, `refresh_token_enc`). These will diverge by P4. Security-sensitive surfaces (Stripe BYOS keys, consent evidence, OAuth tokens) must not rely on ad-hoc per-team wrappers.

---

## §2 Architecture

- **Service shape:** Go binary, Docker container, port 3823. Minimal attack surface — no external network calls except to Postgres.
- **Hasura Remote Schema:** Yes — exposes `vaultEncrypt`, `vaultDecrypt`, `vaultRotateKey`, `vaultListKeys` (admin only), `vaultAuditLog` (admin only).
- **Database schema:** Postgres role `np_vault`, schema `np_vault`. Data (DEKs) encrypted at rest via KEK stored in env. Plaintext never persisted.
- **Envelope encryption model:**
  - KEK (Key-Encryption-Key) — stored only in env var `VAULT_KEK_V{N}`. Never in DB.
  - DEK (Data-Encryption-Key) — generated per (tenant, alias). Stored encrypted with KEK in `np_vault.keys`. Used to encrypt/decrypt application data.
  - Ciphertext format: `v1:base64url(nonce+ciphertext)` where nonce is random 24 bytes (XChaCha20-Poly1305 requirement).

**Cross-plugin note — key rotation pattern:** The `VAULT_KEK_V{N}` version-numbered env var scheme mirrors the JWT dual-key rotation pattern in `cli/internal/auth/jwt_rotation.go` (`RotateResult.GraceUntil` + dual-key acceptance window). Vault key rotation follows the same invariant: old KEK version remains accepted for decrypt until all DEKs are re-wrapped under the new KEK. `VAULT_CURRENT_KEK_VERSION` controls which version encrypts new DEKs; older versions are accepted for decrypt during the grace window. The `RotationWindowDays` / `RotationHardDays` pattern from jwt_rotation.go is the reference implementation.

---

## §3 Multi-Tenancy

**Multi-Tenant Convention Wall declaration (T2-10 requirement):**

This plugin uses BOTH isolation mechanisms — each serves a distinct purpose:

| Column | Mechanism | Applied to |
|--------|-----------|------------|
| `source_account_id TEXT NOT NULL DEFAULT 'primary'` | Multi-App Isolation | Keys scoped per app within one nSelf deploy |
| `tenant_id UUID` | Cloud Multi-Tenancy | Keys scoped per paying Cloud customer |

**Wall rule:** Keys are scoped to `(tenant_id, source_account_id)` pair. A self-hosted deployer with one app has `tenant_id = NULL` and `source_account_id = 'primary'`. A Cloud customer with two apps has `tenant_id = <their-uuid>` and two `source_account_id` values.

**NEVER** use `source_account_id` to separate Cloud customers. **NEVER** use `tenant_id` for multi-app isolation within one deploy.

Hasura row filter on `np_vault.keys` and `np_vault.audit_log`:
```json
{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}
```

**Enforcement:** `nself doctor --deep` check `PERM-RLS-01` verifies both columns and Hasura row filters are present on all `np_vault.*` tables.

---

## §4 Data Model

```sql
-- Postgres role
CREATE ROLE np_vault NOLOGIN;
GRANT USAGE ON SCHEMA np_vault TO np_vault;

-- Master key envelopes (never stores plaintext DEKs or KEKs)
CREATE TABLE np_vault.keys (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         UUID,
  source_account_id TEXT        NOT NULL DEFAULT 'primary',
  key_alias         TEXT        NOT NULL DEFAULT 'default',
  kek_version       INT         NOT NULL DEFAULT 1,       -- matches VAULT_CURRENT_KEK_VERSION
  encrypted_dek     BYTEA       NOT NULL,                 -- DEK encrypted with KEK[kek_version]
  algorithm         TEXT        NOT NULL DEFAULT 'xchacha20poly1305',
  status            TEXT        NOT NULL DEFAULT 'active', -- active|rotated|revoked
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  rotated_at        TIMESTAMPTZ,
  UNIQUE (tenant_id, source_account_id, key_alias, kek_version)
);
CREATE INDEX idx_vault_keys_tenant ON np_vault.keys (tenant_id, source_account_id, status);

-- Audit log (immutable — INSERT only, no UPDATE/DELETE)
CREATE TABLE np_vault.audit_log (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         UUID,
  source_account_id TEXT        NOT NULL DEFAULT 'primary',
  key_id            UUID        REFERENCES np_vault.keys(id),
  operation         TEXT        NOT NULL, -- encrypt|decrypt|rotate|revoke
  caller_ip         INET,
  caller_role       TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_vault_audit_tenant ON np_vault.audit_log (tenant_id, source_account_id, created_at DESC);
```

**Rollback:** `DROP SCHEMA np_vault CASCADE; DROP ROLE np_vault;`

**Migration note:** Existing `_enc` columns (S11 `stripe_byos_key_enc`, S12 `access_token_enc`, S14 `consent_evidence_url`) are preserved. A migration guide provides a per-table script to re-encrypt values under a vault DEK on first write. Migration is additive and backwards-compatible — old raw AES-256 env-var approach continues until callers opt in.

---

## §5 Permissions / Hasura Role Matrix

| Role | encrypt | decrypt | rotate | audit | listKeys |
|------|---------|---------|--------|-------|----------|
| admin | ✓ | ✓ | ✓ | ✓ | ✓ |
| service | ✓ | ✓ | ✗ | ✗ | ✗ |
| user | ✗ | ✗ | ✗ | ✗ | ✗ |
| anonymous | ✗ | ✗ | ✗ | ✗ | ✗ |

Row-level security: all `np_vault.*` tables have RLS enabled. The `user` role has no row filter because users have no direct table access — all access is through Hasura Actions (service-role JWT). Admin operations require `X-Hasura-Role: admin`.

---

## §6 API

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| POST | `/vault/encrypt` | Hasura Action JWT (service role) | Body: `{key_alias, plaintext}`. Returns `{ciphertext}` (base64url). |
| POST | `/vault/decrypt` | Hasura Action JWT (service role) | Body: `{key_alias, ciphertext}`. Returns `{plaintext}`. |
| POST | `/vault/rotate` | admin JWT | Rotates DEK for `(tenant, source_account_id, alias)`. Old DEK remains for decrypt during grace period. Returns new `key_id`. |
| GET | `/vault/keys` | admin JWT | Lists key metadata only — never DEK material. |
| GET | `/vault/audit` | admin JWT | Paginated audit log. Params: `?key_id=&from=&to=&limit=`. |
| GET | `/health` | none | Liveness — returns `{"status":"ok"}`. |

**Rate limits** (enforced in handler, not Nginx):
- `POST /vault/decrypt`: 1000 req/min per (tenant, source_account_id)
- `POST /vault/encrypt`: 500 req/min per (tenant, source_account_id)
- Violations: HTTP 429 with `Retry-After` header.

---

## §7 Plugin Manifest

```json
{
  "name": "nself-vault",
  "displayName": "Per-Row Encryption (Vault)",
  "version": "1.0.0",
  "tier": "pro",
  "bundle": ["shared"],
  "port": 3823,
  "language": "go",
  "systemDependencies": ["libsodium"],
  "env": {
    "required": [
      "DATABASE_URL",
      "VAULT_KEK_V1"
    ],
    "optional": {
      "VAULT_KEK_V2": "",
      "VAULT_CURRENT_KEK_VERSION": "1",
      "VAULT_ENCRYPT_RATE_RPM": "500",
      "VAULT_DECRYPT_RATE_RPM": "1000",
      "VAULT_AUDIT_RETENTION_DAYS": "365",
      "VAULT_PLUGIN_ENABLED": "false"
    }
  },
  "hasuraRemoteSchema": true,
  "featureFlag": "VAULT_PLUGIN_ENABLED"
}
```

`VAULT_PLUGIN_ENABLED=false` by default — explicit opt-in required given security sensitivity. Setting to `true` enables the service and registers the Hasura Remote Schema.

---

## §8 Observability

- **Metrics:** `vault_operations_total{op,status}`, `vault_operation_duration_seconds{op}`, `vault_key_rotation_total`, `vault_decrypt_errors_total`
- **Logs:** Structured JSON. Every encrypt/decrypt/rotate logs `{op, key_id, tenant_id, source_account_id, duration_ns}`. Plaintext NEVER logged — enforced by log sanitizer.
- **Traces:** OpenTelemetry spans on each operation with `key.alias` and `op` attributes.
- **Alerts:**
  - `vault_decrypt_errors_total` rate > 1% over 5 min → CRITICAL (possible KEK mismatch or DB corruption)
  - Audit log write failure (operation completes but audit INSERT fails) → CRITICAL
  - `VAULT_KEK_V{N}` missing for active `kek_version` in DB → CRITICAL on startup

---

## §9 Competitive Parity

vs. full-database encryption (pgcrypto, Transparent Data Encryption): those approaches encrypt the entire DB or table, providing no selectivity. nself-vault provides per-column, per-row selective encryption — only sensitive fields are encrypted, leaving query plans and indexes unaffected on non-sensitive columns.

vs. AWS KMS / HashiCorp Vault: those require vendor accounts, network egress, and per-call costs. nself-vault is fully self-hosted with zero vendor lock-in, zero per-operation cost, and zero data egress. The libsodium XChaCha20-Poly1305 implementation is auditable in the plugin source.

vs. ad-hoc AES-256 env-var wrappers (the current nSelf P3 pattern): ad-hoc wrappers diverge across teams, have no key rotation, no audit trail, and no centralized management. nself-vault centralizes all of this with a single plugin install.

---

## §10 Testing Plan

- **Unit:** XChaCha20-Poly1305 round-trip, KEK version selection logic, rotation idempotency (rotate twice = same result), bad-ciphertext rejection (tampered nonce → error, not panic), rate limiter counters.
- **Integration:** Postgres schema creation from migration, full key lifecycle (create → encrypt → rotate → decrypt with re-wrapped DEK), audit log entry creation, RLS enforcement (service role only).
- **E2E:** Hasura Action end-to-end: `vaultEncrypt` → store ciphertext → `vaultDecrypt` → assert plaintext matches. Verify `audit_log` entry created. Verify plaintext never appears in `np_vault.keys` or `audit_log`.
- **Security regression:** Verify decryption fails with wrong `tenant_id` (cross-tenant isolation). Verify old KEK version still decrypts during grace window after rotation. Verify revoked key returns 403.

---

## §11 Rollout

1. Install: `nself plugin install nself-vault` (requires pro license)
2. Set env: `VAULT_KEK_V1=<32-byte-hex>` and `VAULT_PLUGIN_ENABLED=true`
3. Run: `nself build && nself start` — plugin registers Hasura Remote Schema on startup
4. Validate: `curl localhost:3823/health` returns `{"status":"ok"}`
5. Migrate existing `_enc` columns: run provided per-table migration script (additive, no downtime)
6. Key rotation: set `VAULT_KEK_V2=<new-32-byte-hex>`, `VAULT_CURRENT_KEK_VERSION=2`, restart. Old DEKs re-wrapped lazily on next encrypt call. Old KEK V1 accepted for decrypt until all DEKs are migrated.

**Backwards compat:** Existing raw AES-256 env-var pattern continues to work until callers explicitly migrate to `vaultEncrypt`. vault plugin is purely additive. No forced migration.

---

## §12 Docs to Update

- `plugins-pro/.github/docs/per-row-encryption.md` — user-facing wiki page
- `web/docs/src/content/plugins/nself-vault.mdx` — public docs
- `plugins-pro/paid/nself-vault/README.md` — repo README (create at implementation time)
- SPORT F04 plugin count +1, F06 add `nself-vault` as shared pro plugin (post-P98 regeneration)

---

## §13 Bundle Classification

- **Base:** `shared` pro — available to any ɳSelf+ subscriber or standalone pro license
- **nClaw bundle:** included (nClaw uses vault for OAuth token storage via `social` plugin and Stripe BYOS keys via `nself-stripe`)
- **vault-cloud extension:** `visibility: internal` — adds centralized key rotation UI in `web/cloud` and SIEM log export. Separate plugin entry with `"extends": "nself-vault"`. Cloud MAX tier only.

---

## §14 Security Notes

- **Security-Always-Free compliance:** nself-vault is a paid plugin. However, the encryption-at-rest config that enables vault-backed column encryption for existing free-tier data (audit logs, user PII) must remain accessible without license. Resolution: `nself doctor --deep` check `SEC-VAULT-01` warns (not blocks) if sensitive `_enc` columns exist but vault plugin is not installed. The warning is free; the fix (installing vault) requires a license. This is accepted per the Security-Always-Free Doctrine — the detection is free, the remediation is paid.
- **KEK storage:** KEK never enters the DB. Stored only in env var, loaded into process memory. On shutdown, Go's GC does not guarantee zeroing — mitigated by using `golang.org/x/crypto/memguard` wrappers (future: add locked memory).
- **Audit log integrity:** `np_vault.audit_log` has `REVOKE UPDATE, DELETE ON np_vault.audit_log FROM np_vault;` — service role can INSERT but not modify or delete audit entries.
- **SSRF:** vault service makes zero outbound network calls. Postgres connection uses `VAULT_DB_HOST` env var — validated to be a local address on startup.
- **Dependency:** libsodium must be present in the container image. `nself doctor --deep` check `DEP-LIBSODIUM-01` verifies `libsodium.so` is loadable before enabling the plugin.

---

## §15 Shippability

**Target version:** v1.1.0

**Hard dependencies for this plugin (blocks v1.1.0):**
- `VAULT_KEK_V1` env var must be added to `web/backend/.env.example` (template update only)
- Hasura Remote Schema registration tested against Hasura v2.x

**Plugins blocked on this spec:**
- `nself-stripe` (T2-01) — uses vault for BYOS Stripe key storage
- `social` plugin upgrade — uses vault for OAuth token storage (`ua_social_accounts.access_token_enc`)
- `compliance` plugin upgrade — uses vault for consent evidence URL storage

**No v1.0.x backport.** This plugin targets v1.1.0 and later only. The existing ad-hoc AES-256 wrappers remain in place for v1.0.x.

**Open questions (resolved):**
- `vault-cloud` as extension vs separate plugin: **separate plugin** with `"extends": "nself-vault"` in manifest, `visibility: internal`.
- Key rotation strategy (lazy vs eager): **lazy** — caller re-encrypts on next write. Eager requires job-queue dependency (available in v1.2.0 via 04.T30 job-queue plugin). Lazy is additive and simpler.
