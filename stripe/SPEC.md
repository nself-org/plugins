# nself-stripe Plugin Specification

**Plugin:** `stripe`
**Port:** 3830 (post-T2-14 collision resolution — shifted from 3829 to 3830; nself-scan retained 3829)
**Language:** Go
**Category:** commerce
**License:** MIT (free plugin — D7 decision: Stripe is FREE, lives in `plugins/stripe/` not `plugins-pro/paid/`)
**Bundle:** None — Cloud MAX only, `visibility: internal`
**Status:** Planned — v1.1.0
**SPEC version:** 1.0.0 (authored P98 S98-04)

---

## 1. Overview

`nself-stripe` is an internal Cloud MAX plugin that wires nSelf Cloud's subscription billing to Stripe Connect. It handles the full managed-billing lifecycle: onboarding operators to Connect, creating checkout sessions for bundle purchases, processing webhook events from both platform and connected accounts, and maintaining a queryable entitlement cache.

**This plugin is `visibility: internal`.** It is NOT installed by self-hosted operators. It runs on nSelf Cloud infrastructure to power the `cloud.nself.org` billing surface.

**Primary use cases:**
- Operator onboarding to Stripe Connect (Express or Standard account)
- Bundle subscription checkout (monthly and annual)
- Webhook processing for subscription lifecycle events
- Entitlement cache for fast `billing.can(entity_id, capability)` checks
- Connect platform-level account status sync

**Non-goals:**
- General-purpose payment processing for end-users of operator apps (use `stripe` from a custom service)
- Invoice generation (use Stripe Billing portal directly)
- Marketplace fee collection (future — deferred to v2)

---

## 2. Architecture

```
cloud.nself.org (Vercel)
    │
    ▼
web/backend Hasura
    │  Remote Schema
    ▼
nself-stripe service (Go, port 3830)
    │               │               │
    ▼               ▼               ▼
PostgreSQL      Stripe API      Redis (entitlement cache)
(np_stripe_*)   (platform +     TTL: GEO_CACHE_TTL_SECONDS
                 connect accts)
```

### Key components

| Component | Purpose |
|-----------|---------|
| `cmd/server/main.go` | HTTP server bootstrap, webhook signature verification middleware |
| `internal/stripe/handler.go` | HTTP handlers for all endpoints |
| `internal/stripe/connect.go` | Stripe Connect onboarding and account sync |
| `internal/stripe/checkout.go` | Stripe Checkout session creation for subscriptions |
| `internal/stripe/webhook.go` | Dual webhook routing: platform vs connect |
| `internal/stripe/entitlements.go` | `billing.can()` helper and entitlement cache |
| `internal/stripe/db.go` | PostgreSQL operations for `np_stripe_*` tables |
| `internal/stripe/cache.go` | Redis entitlement cache with write-through invalidation |
| `plugin.json` | Plugin manifest |

---

## 3. Multi-Tenant Convention Wall Compliance

**Classification:** Cloud-MAX-only plugin. `tenant_id` present on all tables; `source_account_id` is NOT APPLICABLE.

### Wall declaration (HF-1)

Per PPI Hard Rule (Multi-Tenant Convention Wall):

- **`source_account_id`** (multi-app isolation within one deploy): **NOT APPLICABLE** — this plugin runs exclusively in nSelf Cloud MAX deployments, where each paying customer receives an isolated nSelf instance. There is no multi-app scenario within a single nself-stripe installation. Adding `source_account_id` would create a meaningless default-`primary` column on every row.
- **`tenant_id UUID`** (Cloud customer isolation): **REQUIRED** — present on all `np_stripe_*` tables with Hasura row filter `{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}`. This isolates billing data between Cloud customers.

This HF-1 justification must be reviewed at code review. Any future addition of multi-app support MUST add `source_account_id` per the Wall doctrine.

### Hasura row filters

Both tables carry:
```json
{
  "tenant_id": { "_eq": "X-Hasura-Tenant-Id" }
}
```

Applied to: select, insert (preset), update, delete operations for the `user` role. The `admin` role bypasses filters for cloud ops tooling.

---

## 4. Data Model

### Table: `np_stripe_accounts`

Stores Stripe Connect account state per Cloud tenant.

```sql
CREATE TABLE np_stripe_accounts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL,                    -- Cloud customer isolation (Wall)
    entity_id          TEXT NOT NULL,                    -- nSelf entity (org ID, user ID)
    entity_type        TEXT NOT NULL CHECK (entity_type IN ('org', 'user', 'instance')),
    stripe_account_id  TEXT NOT NULL UNIQUE,             -- acct_xxx from Stripe Connect
    account_type       TEXT NOT NULL DEFAULT 'express'   -- 'express' or 'standard'
                       CHECK (account_type IN ('express', 'standard')),
    status             TEXT NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'onboarding', 'active', 'restricted', 'deauthorized')),
    details_submitted  BOOLEAN NOT NULL DEFAULT FALSE,   -- Stripe: requirements.past_due empty
    charges_enabled    BOOLEAN NOT NULL DEFAULT FALSE,   -- Stripe: charges_enabled
    payouts_enabled    BOOLEAN NOT NULL DEFAULT FALSE,   -- Stripe: payouts_enabled
    capabilities       JSONB NOT NULL DEFAULT '{}',      -- Stripe capabilities snapshot
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_np_stripe_accounts_tenant_entity ON np_stripe_accounts (tenant_id, entity_id);
CREATE INDEX idx_np_stripe_accounts_stripe_id ON np_stripe_accounts (stripe_account_id);
```

### Table: `np_stripe_entitlements`

Entitlement cache — records active subscriptions and their capabilities.

```sql
CREATE TABLE np_stripe_entitlements (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL,                    -- Cloud customer isolation (Wall)
    entity_id          TEXT NOT NULL,                    -- subscriber entity
    entity_type        TEXT NOT NULL DEFAULT 'org',
    capability         TEXT NOT NULL,                    -- 'nchat_bundle', 'nself_plus', etc.
    stripe_sub_id      TEXT,                             -- sub_xxx (null if comped)
    stripe_price_id    TEXT,                             -- price_xxx
    status             TEXT NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'past_due', 'canceled', 'comped', 'trial')),
    period_start       TIMESTAMPTZ,
    period_end         TIMESTAMPTZ,                      -- used for cache-bust logic
    cancel_at_period   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (entity_id, capability, tenant_id)
);

CREATE INDEX idx_np_stripe_entitlements_tenant_entity ON np_stripe_entitlements (tenant_id, entity_id);
CREATE INDEX idx_np_stripe_entitlements_stripe_sub ON np_stripe_entitlements (stripe_sub_id);
CREATE INDEX idx_np_stripe_entitlements_expiry ON np_stripe_entitlements (period_end)
    WHERE status IN ('active', 'past_due', 'trial');
```

### Drizzle migration

Two migration files:
- `0001_create_np_stripe_accounts.sql`
- `0002_create_np_stripe_entitlements.sql`

Drizzle schema lives at `ts/src/database.ts`.

---

## 5. API Endpoints

All endpoints require internal service token (`Authorization: Bearer $STRIPE_PLUGIN_INTERNAL_TOKEN`). Platform-level endpoints additionally require admin role. Webhook endpoints require valid Stripe signature (separate from bearer token).

### POST /onboard

Initiate Stripe Connect Express onboarding for an entity.

**Request body:**

```json
{
  "entity_id": "org_01HXXX",
  "entity_type": "org",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "return_url": "https://cloud.nself.org/billing/onboard/complete",
  "refresh_url": "https://cloud.nself.org/billing/onboard/refresh",
  "account_type": "express"
}
```

**Response 200:**

```json
{
  "account_id": "acct_1ABCxxx",
  "onboarding_url": "https://connect.stripe.com/express/onboarding/xxx",
  "expires_at": "2026-05-03T13:00:00Z"
}
```

Creates a record in `np_stripe_accounts` with `status: 'onboarding'`. The onboarding URL is valid for 1 hour (Stripe AccountLink TTL).

---

### POST /checkout

Create a Stripe Checkout Session for a subscription purchase.

**Request body:**

```json
{
  "entity_id": "org_01HXXX",
  "tenant_id": "550e8400-...",
  "capability": "nchat_bundle",
  "price_id": "price_1ABCxxx",
  "success_url": "https://cloud.nself.org/billing/success?session_id={CHECKOUT_SESSION_ID}",
  "cancel_url": "https://cloud.nself.org/billing/cancel"
}
```

**Response 200:**

```json
{
  "checkout_url": "https://checkout.stripe.com/pay/cs_live_xxx",
  "session_id": "cs_live_xxx"
}
```

On `checkout.session.completed` webhook, `np_stripe_entitlements` is upserted and cache is invalidated.

---

### POST /stripe/webhook/platform

Handles Stripe platform-level webhook events (events on the nSelf platform Stripe account).

**Stripe-Signature header required.** HMAC-SHA256 validated against `STRIPE_WEBHOOK_SECRET_PLATFORM`.

**Handled events:**

| Event | Action |
|-------|--------|
| `account.updated` | Sync `np_stripe_accounts` status, capabilities, charges_enabled, payouts_enabled |
| `account.application.deauthorized` | Set `np_stripe_accounts.status = 'deauthorized'`, revoke entitlements |
| `checkout.session.completed` | Upsert `np_stripe_entitlements`, invalidate Redis cache |
| `checkout.session.expired` | Log expired session, no entitlement change |

**Response 200:** `{"received": true}` — always 200 on valid signature (Stripe retry semantics).
**Response 400:** Invalid signature.
**Response 500:** Processing error (Stripe will retry).

---

### POST /stripe/webhook/connect

Handles Stripe Connect webhook events (events on connected accounts).

**Stripe-Signature header required.** HMAC-SHA256 validated against `STRIPE_WEBHOOK_SECRET_CONNECT`.

**Handled events:**

| Event | Action |
|-------|--------|
| `customer.subscription.created` | Upsert entitlement with `status: 'active'`, cache write-through |
| `customer.subscription.updated` | Update entitlement status/period, cache invalidate |
| `customer.subscription.deleted` | Set entitlement `status: 'canceled'`, cache invalidate |
| `customer.subscription.trial_will_end` | Fire notification via `notify` plugin if installed |
| `invoice.payment_failed` | Set entitlement `status: 'past_due'`, cache invalidate |
| `invoice.payment_succeeded` | Clear `past_due`, restore `active`, cache write-through |

**T2-13 compliance note:** Platform events (`account.updated`) and Connect subscription events use separate webhook secrets and separate handler paths. This prevents a compromised connect-account webhook secret from injecting platform-level account mutations.

---

### GET /entitlement

Check entitlement status for an entity+capability pair. Primary surface for `billing.can()`.

**Query parameters:**

| Parameter | Type | Required |
|-----------|------|----------|
| `entity_id` | string | Yes |
| `capability` | string | Yes |
| `tenant_id` | string | Yes |

**Response 200:**

```json
{
  "entity_id": "org_01HXXX",
  "capability": "nchat_bundle",
  "status": "active",
  "period_end": "2026-06-03T00:00:00Z",
  "cached": true
}
```

**Response 404:** No entitlement record found (interpret as not entitled).
**Cache behavior:** checked in Redis first (key: `ent:{tenant_id}:{entity_id}:{capability}`); on miss, queries `np_stripe_entitlements` and writes to Redis with TTL = `STRIPE_ENTITLEMENT_CACHE_TTL_SECONDS` (default: 3600).

---

### GET /account/:entity_id

Retrieve Stripe Connect account status for an entity.

**Query parameters:**

| Parameter | Type | Required |
|-----------|------|----------|
| `tenant_id` | string | Yes |

**Response 200:**

```json
{
  "entity_id": "org_01HXXX",
  "stripe_account_id": "acct_1ABCxxx",
  "status": "active",
  "charges_enabled": true,
  "payouts_enabled": true,
  "details_submitted": true
}
```

**Response 404:** No account record for this entity.

---

### GET /health

**Response 200:**

```json
{
  "status": "ok",
  "db": "connected",
  "cache": "connected",
  "stripe_mode": "live"
}
```

`stripe_mode` is `"test"` when `STRIPE_SECRET_KEY` starts with `sk_test_`.

---

## 6. `billing.can()` Entitlement Helper

The `billing.can()` helper is the canonical surface for checking whether an entity holds a capability. It MUST be used by all Cloud MAX components instead of direct DB queries.

```go
// BillingCan returns true if entity_id holds capability for tenant_id.
// Checks Redis cache first; falls back to DB; writes through to cache on miss.
func BillingCan(ctx context.Context, tenantID, entityID, capability string) (bool, error) {
    key := fmt.Sprintf("ent:%s:%s:%s", tenantID, entityID, capability)
    
    // 1. Redis check
    if val, err := cache.Get(ctx, key); err == nil {
        return val == "active" || val == "trial", nil
    }
    
    // 2. DB fallback
    ent, err := db.GetEntitlement(ctx, tenantID, entityID, capability)
    if err != nil {
        if errors.Is(err, ErrNotFound) {
            cache.Set(ctx, key, "none", entitlementCacheTTL)
            return false, nil
        }
        return false, err
    }
    
    // 3. Write through
    cache.Set(ctx, key, ent.Status, entitlementCacheTTL)
    return ent.Status == "active" || ent.Status == "trial", nil
}
```

**Capabilities register** (partial — full list in `internal/stripe/capabilities.go`):

| Capability string | Bundle / tier |
|------------------|---------------|
| `nchat_bundle` | nChat bundle |
| `nclaw_bundle` | nClaw bundle |
| `nfamily_bundle` | nFamily bundle |
| `ntv_bundle` | nTV bundle |
| `clawde_bundle` | ClawDE bundle |
| `nself_plus` | ɳSelf+ (all bundles) |
| `ncloud_max` | nCloud MAX tier |
| `all_bundles` | ɳSelf+ legacy alias |

---

## 7. Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `STRIPE_PORT` | `3830` | No | HTTP listen port |
| `STRIPE_SECRET_KEY` | — | Yes | Stripe secret key (sk_live_... or sk_test_...) |
| `STRIPE_PUBLISHABLE_KEY` | — | Yes | Stripe publishable key |
| `STRIPE_WEBHOOK_SECRET_PLATFORM` | — | Yes | Webhook signing secret for /stripe/webhook/platform |
| `STRIPE_WEBHOOK_SECRET_CONNECT` | — | Yes | Webhook signing secret for /stripe/webhook/connect |
| `STRIPE_PLUGIN_INTERNAL_TOKEN` | — | Yes | Bearer token for internal service auth |
| `STRIPE_DB_URL` | — | Yes | PostgreSQL connection URL |
| `STRIPE_REDIS_URL` | `redis://127.0.0.1:6379` | No | Redis URL for entitlement cache |
| `STRIPE_ENTITLEMENT_CACHE_TTL_SECONDS` | `3600` | No | Entitlement cache TTL (1h default) |
| `STRIPE_CONNECT_ACCOUNT_TYPE` | `express` | No | Default Connect account type: `express` or `standard` |
| `STRIPE_PLATFORM_ACCOUNT_ID` | — | Yes | nSelf's Stripe platform account ID (acct_xxx) |
| `STRIPE_LOG_LEVEL` | `info` | No | Logging level: debug/info/warn/error |

---

## 8. Plugin Manifest

```json
{
  "name": "stripe",
  "displayName": "Stripe Billing",
  "version": "1.0.0",
  "category": "commerce",
  "description": "Cloud MAX internal billing plugin. Handles Stripe Connect onboarding, subscription checkout, dual webhook routing, and entitlement caching for nSelf Cloud subscriptions.",
  "isCommercial": false,
  "licenseType": "free",
  "requires_license": false,
  "requiredEntitlements": [],
  "visibility": "internal",
  "cloudMaxOnly": true,
  "language": "go",
  "port": 3830,
  "portNote": "Shifted from 3829 to 3830 in P98 T2-14 collision resolution. nself-scan retains 3829.",
  "dependencies": {
    "postgres": "required",
    "redis": "required"
  },
  "envVars": [
    "STRIPE_PORT",
    "STRIPE_SECRET_KEY",
    "STRIPE_PUBLISHABLE_KEY",
    "STRIPE_WEBHOOK_SECRET_PLATFORM",
    "STRIPE_WEBHOOK_SECRET_CONNECT",
    "STRIPE_PLUGIN_INTERNAL_TOKEN",
    "STRIPE_DB_URL",
    "STRIPE_REDIS_URL",
    "STRIPE_ENTITLEMENT_CACHE_TTL_SECONDS",
    "STRIPE_CONNECT_ACCOUNT_TYPE",
    "STRIPE_PLATFORM_ACCOUNT_ID",
    "STRIPE_LOG_LEVEL"
  ],
  "endpoints": [
    "POST /onboard",
    "POST /checkout",
    "POST /stripe/webhook/platform",
    "POST /stripe/webhook/connect",
    "GET /entitlement",
    "GET /account/:entity_id",
    "GET /health"
  ],
  "hasura": {
    "remoteSchema": true,
    "actions": ["createCheckoutSession", "getEntitlement", "getStripeAccount"]
  },
  "tenant_id_required": true,
  "source_account_id_applicable": false,
  "wall_justification": "HF-1: Cloud-MAX-only plugin; each Cloud customer gets isolated instance; source_account_id N/A; tenant_id required on all np_stripe_* tables"
}
```

---

## 9. Hasura Integration

### Remote Schema actions

```graphql
type Mutation {
  createCheckoutSession(
    entity_id: String!
    capability: String!
    price_id: String!
    success_url: String!
    cancel_url: String!
  ): CheckoutSessionResult!

  initiateStripeOnboarding(
    entity_id: String!
    entity_type: EntityType!
    return_url: String!
    refresh_url: String!
  ): OnboardingResult!
}

type Query {
  stripeEntitlement(
    entity_id: String!
    capability: String!
  ): EntitlementResult

  stripeAccount(
    entity_id: String!
  ): StripeAccountResult
}
```

### Hasura permissions (role matrix)

| Role | `np_stripe_accounts` | `np_stripe_entitlements` | Notes |
|------|---------------------|--------------------------|-------|
| admin | Full CRUD (no row filter) | Full CRUD (no row filter) | Cloud ops tooling |
| user | SELECT only (tenant_id filter) | SELECT only (tenant_id filter) | End users: read own entitlements |
| cloud_operator | INSERT, SELECT (tenant_id filter) | SELECT (tenant_id filter) | Cloud-level write for onboarding |
| anonymous | None | None | — |

All non-admin roles have Hasura row filter: `{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}`.

---

## 10. Security Controls

### Webhook signature verification

Both webhook endpoints MUST verify Stripe's HMAC-SHA256 `Stripe-Signature` header BEFORE any processing:

```go
func verifyWebhookSignature(r *http.Request, secret string) ([]byte, error) {
    payload, err := io.ReadAll(io.LimitReader(r.Body, 65536))
    if err != nil {
        return nil, err
    }
    event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), secret)
    if err != nil {
        return nil, fmt.Errorf("signature verification failed: %w", err)
    }
    return payload, nil
}
```

Rejected signatures return 400 immediately. No business logic runs on unverified payloads.

### Idempotency

All webhook handlers are idempotent. Processing the same event twice MUST produce the same result. Use Stripe event ID as an idempotency key in the DB:

```sql
-- Optional: webhook event dedup table
CREATE TABLE np_stripe_events_processed (
    stripe_event_id TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

If event is already in `np_stripe_events_processed`, return 200 immediately without reprocessing.

### Secret key protection

- `STRIPE_SECRET_KEY` is NEVER logged, NEVER included in error responses, NEVER exposed in any endpoint
- Only the last 4 chars of the key are permitted in debug logs (for env identification)
- Health endpoint does NOT expose key material — only `stripe_mode: "live"` or `"test"`

### Connect account validation

When processing events on a Connect account, verify the account ID (`account` field on the event) exists in `np_stripe_accounts` before processing. Events from unknown accounts are logged and rejected with 400.

### Rate limiting (Security-Always-Free)

Per-IP rate limiting on `/onboard` and `/checkout` (10 req/min per IP) is FREE and always-on. Prevents abuse of expensive Stripe API calls. Uses token bucket backed by Redis.

---

## 11. Doctor Dependency Check

`nself doctor --plugins stripe` validates:

| Check ID | Severity | Condition | Message |
|----------|---------|-----------|---------|
| `STRIPE-KEY-01` | CRITICAL | `STRIPE_SECRET_KEY` empty | "STRIPE_SECRET_KEY is required. Set in .env.secrets." |
| `STRIPE-KEY-02` | WARNING | `STRIPE_SECRET_KEY` starts with `sk_test_` AND env is production | "Stripe is in test mode on production. Set STRIPE_SECRET_KEY to a live key." |
| `STRIPE-WH-01` | CRITICAL | `STRIPE_WEBHOOK_SECRET_PLATFORM` empty | "STRIPE_WEBHOOK_SECRET_PLATFORM is required for /stripe/webhook/platform." |
| `STRIPE-WH-02` | CRITICAL | `STRIPE_WEBHOOK_SECRET_CONNECT` empty | "STRIPE_WEBHOOK_SECRET_CONNECT is required for /stripe/webhook/connect. Must be different from platform secret." |
| `STRIPE-WH-03` | ERROR | `STRIPE_WEBHOOK_SECRET_PLATFORM` == `STRIPE_WEBHOOK_SECRET_CONNECT` | "Platform and Connect webhook secrets MUST be different. Using the same secret breaks dual-webhook security." |
| `STRIPE-DB-01` | CRITICAL | `STRIPE_DB_URL` empty OR DB unreachable | "Database required for np_stripe_* tables." |
| `STRIPE-REDIS-01` | WARNING | Redis unreachable | "Redis unavailable — entitlement checks will fall back to DB. Performance will degrade under load." |
| `STRIPE-INTERNAL-01` | WARNING | `visibility: internal` check | "This plugin is marked visibility:internal. It is NOT for self-hosted deployments. Cloud MAX only." |

---

## 12. Competitive Context

nself-stripe is an internal billing infrastructure plugin, not a feature for end-users. It replaces:

| Alternative | Why Not |
|-------------|---------|
| Direct Stripe SDK in web/backend | Violates plugin-first doctrine; billing logic not reusable across Cloud instances |
| Paddle / Lemon Squeezy | Stripe Connect is essential for marketplace revenue splits; LS pre-staged as fallback (see operations docs), not primary |
| Chargebee / Recurly | Overkill for current scale; Stripe Billing + Connect handles all requirements at lower cost |

**Failover:** Lemon Squeezy is pre-staged as MoR fallback per `web/backend/docs/stripe-failover-runbook.md`. If Stripe terminates the platform account, migration path is documented. This plugin does NOT expose Lemon Squeezy routes — that failover uses a separate `lemon-squeezy` internal plugin stub.

---

## 13. Test Plan

### Tier 1 — Unit tests (≥40% coverage)

- Webhook signature verification: valid signature passes, tampered payload fails, expired timestamp fails
- `billing.can()` helper: Redis hit, DB fallback, not-found → false, `past_due` → false, `trial` → true
- Entitlement upsert logic: new record, update status, honor UNIQUE constraint
- Input validation: entity_id max length, capability enum, tenant_id UUID format
- Idempotency: duplicate event ID → 200 without DB write

### Tier 2 — Integration tests (≥20% coverage)

- `/onboard` end-to-end with mocked Stripe client: creates `np_stripe_accounts` row
- `/checkout` with mocked Stripe client: returns checkout URL, pending entitlement
- `/stripe/webhook/platform` with `account.updated`: syncs account status
- `/stripe/webhook/connect` with `customer.subscription.created`: creates entitlement, Redis write
- `/entitlement` cache miss → DB query → cache write

### Tier 3 — E2E scaffold (≥5% coverage)

- Health check returns 200 with `db: "connected"` and `stripe_mode: "test"`
- Live Stripe test mode call (skipped in CI, opt-in via `STRIPE_RUN_LIVE_TESTS=true`)

### Test prohibitions

- No real Stripe API calls in unit or integration tests (use `stripe-mock` or request/response fixtures)
- No real webhook signatures in tests (compute HMAC from test secret)
- No production `sk_live_` keys in any test fixture

---

## 14. Port Registry Annotation

**Port 3830** is the canonical port for `nself-stripe`.

**Collision resolution note:** Original port assignment was 3829. In P98 T2-14, a collision was discovered between `nself-scan` and `nself-stripe` (both assigned 3829). Resolution: `nself-scan` retains 3829 (assigned first, Security-Always-Free priority), `nself-stripe` shifted to 3830 (extending the reserved block from 3820-3829 to 3820-3830).

This annotation must appear in:
- `plugin.json` (`portNote` field) ← done above
- PPI port registry table
- SPORT F09 (port inventory)

---

## 15. Bundle Classification

**`visibility: internal` — Cloud MAX only. NOT for self-hosted operators.**

nself-stripe is FREE per D7 (lives in `plugins/stripe/`) but is marked `cloudMaxOnly: true`. It will:
- NOT appear in `nself plugin list` output for self-hosted installs
- NOT be installable via `nself plugin install stripe` (blocked by visibility check)
- Only activate on Cloud MAX instances where nSelf Cloud provisions it automatically

**Why free despite Cloud-only?** D7 established that payment infrastructure should not itself be paywalled — the revenue from billing this plugin enables justifies keeping the plugin code MIT-licensed and free. The plugin only runs on Cloud infra; self-hosters use their own Stripe integration.

---

## 16. Cross-Plugin Coordination

| Plugin | Relationship |
|--------|-------------|
| `notify` | nself-stripe fires `trial_will_end` notification via notify plugin when subscription trial is about to expire |
| `auth` | `entity_id` maps to auth user/org IDs; entitlement checks use the same entity model |
| `analytics` | Billing events (subscription created, canceled, upgraded) emitted as analytics events |
| `entitlements` | `np_stripe_entitlements` is the underlying source for the `entitlements` plugin's cache |
| `admin-api` | Cloud ops can view/override entitlements via admin-api using the `billing.can()` helper |

---

## 17. Migration Plan

### Migrations

Two Drizzle migration files applied in order:
1. `0001_create_np_stripe_accounts.sql`
2. `0002_create_np_stripe_entitlements.sql`

Both run automatically on `nself start` (internal deploy only). Self-hosters never run these migrations.

### Stripe setup prerequisites

Before first deploy:
1. Create nSelf platform Stripe account
2. Enable Connect on the platform account
3. Create two webhook endpoints in Stripe Dashboard:
   - Platform webhook → `https://api.nself.org/stripe/webhook/platform`
   - Connect webhook → `https://api.nself.org/stripe/webhook/connect`
4. Copy signing secrets to `.env.secrets` as `STRIPE_WEBHOOK_SECRET_PLATFORM` and `STRIPE_WEBHOOK_SECRET_CONNECT`
5. Never reuse the same secret for both endpoints (STRIPE-WH-03 check will catch this)

---

## 18. Rollout Plan

| Phase | Action |
|-------|--------|
| v1.1.0 | Initial deploy on Cloud MAX: Connect onboarding, checkout, dual webhook, entitlement cache |
| v1.1.x | Subscription portal (customer-facing), invoice retrieval, trial management |
| v1.2.0 | Marketplace revenue splits, operator payout dashboard |
| v2.0.0 | `lemon-squeezy` failover plugin activated as live alternative (not just pre-staged runbook) |

---

## 19. Observability

### Metrics (Prometheus)

| Metric | Type | Labels |
|--------|------|--------|
| `stripe_webhook_events_total` | counter | `event_type`, `webhook_type` (`platform`/`connect`), `status` |
| `stripe_checkout_sessions_total` | counter | `capability`, `status` |
| `stripe_onboarding_started_total` | counter | `account_type` |
| `stripe_entitlement_checks_total` | counter | `capability`, `source` (`cache`/`db`) |
| `stripe_entitlement_cache_hit_ratio` | gauge | `capability` |
| `stripe_db_latency_seconds` | histogram | `operation` |

### Structured logging

```json
{
  "level": "info",
  "ts": "2026-05-03T12:00:00Z",
  "msg": "webhook_processed",
  "event_type": "customer.subscription.updated",
  "webhook_type": "connect",
  "entity_id": "org_01HXXX",
  "capability": "nchat_bundle",
  "new_status": "past_due",
  "stripe_event_id": "evt_1ABCxxx",
  "idempotent": false
}
```

No Stripe keys, no customer PII in logs. `entity_id` only (internal ID, not email/name).

---

## 20. Docs to Create

| Document | Location | Purpose |
|----------|---------|---------|
| `stripe/README.md` | `plugins/stripe/README.md` | Internal overview, visibility warning, prerequisites |
| Stripe billing setup | `web/backend/.github/docs/stripe-setup.md` | Step-by-step Stripe account + Connect setup for nSelf Cloud |
| Entitlements guide | `.github/docs/cloud/entitlements.md` | How `billing.can()` works, capability strings, cache behavior |
| SPORT F09 update | `.claude/temp/p98-sport-update-notes.md` | Add port 3830 to port inventory with T2-14 collision note |
| SPORT F04 update | `.claude/temp/p98-sport-update-notes.md` | Add `stripe` to free plugin count (+1) |
