# Plugin Spec: transactional-email

**Bundle:** Shared utility (ɳSelf+ and all individual bundles)
**Tier:** pro
**Version-target:** v1.1.0
**Port:** 3822
**Language:** Go
**Status:** SPEC ONLY — no implementation code in P98

---

## §1 Overview

- **Purpose:** Native nSelf transactional email plugin providing a template-aware `POST /email/send` Hasura Action, per-tenant sender domain config, SPF/DKIM status reporting, and delivery webhook relay. Provider-agnostic: Elastic Email (default), any SMTP, or any HTTP email API.
- **Why now:** Four P3 Ummat sprints (M2, S11, S14) call Elastic Email directly with independent API keys per domain, creating credential drift and inconsistent retry/bounce handling. Consolidating behind this plugin eliminates duplication.
- **Bundle Classification:** Shared utility — no bundle exclusive ownership. Available to ɳSelf+ and any individual bundle install. See §16.
- **Language:** Go. No CPU-intensive work; provider HTTP/SMTP calls are I/O bound.
- **Coordinates with:** 08.T14 auth_server email backend (MF-1 amendment — auth plugin's magic link + verification email dispatch delegates to this plugin when installed, falling back to the built-in SMTP stub).

---

## §2 Architecture

- **Service shape:** Go binary, Docker container, port 3822.
- **Hasura Remote Schema:** Yes — exposes `sendEmail`, `getEmailStatus`, `listDomains`, `upsertDomain` mutations/queries.
- **Database schema:** Role `np_email`, schema `np_email`.
- **Provider abstraction:**

```
Hasura Action
    └─ transactional-email Go service (port 3822)
           ├─ Elastic Email HTTP v4 API (default)
           ├─ SMTP (generic — Postmark, SendGrid, Mailgun, etc.)
           └─ Webhook relay ← provider delivery callbacks
```

---

## §3 Multi-Tenant Convention Wall

**This plugin applies BOTH nSelf multi-tenancy mechanisms per the PPI Multi-Tenant Convention Wall Hard Rule.**

| Mechanism | Column | Applies to |
|-----------|--------|-----------|
| **Multi-App Isolation** | `source_account_id TEXT NOT NULL DEFAULT 'primary'` | Separates independent consumer apps running on the same nSelf deploy (e.g., nchat backend vs nclaw backend) |
| **Cloud Multi-Tenancy** | `tenant_id UUID` (nullable) + Hasura row filter `{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}` | Separates paying Cloud customers within an nSelf Cloud instance |

- **NEVER** use `source_account_id` to separate Cloud customers.
- **NEVER** use `tenant_id` for multi-app isolation within one deploy.
- All `np_email.*` tables carry both columns.
- Hasura row filter enforces `tenant_id` isolation for Cloud tenants.
- Reference: `.claude/docs/architecture/multi-tenant-conventions.md`
- **Enforcement:** `nself doctor --deep` check `PERM-RLS-01` verifies both columns and Hasura row filters are present on all `np_email.*` tables.

---

## §4 Data Model

```sql
-- Domain configuration
CREATE TABLE np_email.domains (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID,
  source_account_id TEXT NOT NULL DEFAULT 'primary',
  domain TEXT NOT NULL,
  from_name TEXT NOT NULL DEFAULT 'nSelf',
  from_address TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'elastic_email',
  provider_config JSONB NOT NULL DEFAULT '{}', -- encrypted at app layer
  spf_verified BOOLEAN NOT NULL DEFAULT false,
  dkim_verified BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, source_account_id, domain)
);
CREATE INDEX idx_email_domains_tenant ON np_email.domains (tenant_id, source_account_id);

-- Outbound message log
CREATE TABLE np_email.messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID,
  source_account_id TEXT NOT NULL DEFAULT 'primary',
  domain_id UUID NOT NULL REFERENCES np_email.domains(id),
  to_addresses TEXT[] NOT NULL,
  subject TEXT NOT NULL,
  template_id TEXT,
  template_vars JSONB,
  body_html TEXT,
  body_text TEXT,
  provider_message_id TEXT,
  status TEXT NOT NULL DEFAULT 'queued',  -- queued|sent|delivered|bounced|failed
  sent_at TIMESTAMPTZ,
  delivered_at TIMESTAMPTZ,
  error_detail TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_email_messages_tenant ON np_email.messages (tenant_id, source_account_id, created_at DESC);
CREATE INDEX idx_email_messages_status ON np_email.messages (status, created_at);

-- Templates
CREATE TABLE np_email.templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID,
  source_account_id TEXT NOT NULL DEFAULT 'primary',
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  subject TEXT NOT NULL,
  body_html TEXT NOT NULL,
  body_text TEXT,
  vars_schema JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, source_account_id, slug)
);
```

**Rollback:** `DROP SCHEMA np_email CASCADE; DROP ROLE np_email;`

---

## §5 API Endpoints

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| POST | `/email/send` | Hasura Action JWT | Send immediately or queue |
| POST | `/email/send-template` | Hasura Action JWT | Render template then send |
| GET | `/email/status/:id` | Hasura Action JWT | Poll delivery status |
| POST | `/email/webhook/elastic` | HMAC sig | Inbound delivery event relay (Elastic Email) |
| POST | `/email/webhook/generic` | HMAC sig | Generic provider delivery event relay |
| GET | `/health` | none | Liveness probe |

**Rate limits:** 100 req/min per `tenant_id`; burst 20. Configurable via `EMAIL_RATE_LIMIT_RPM`.

---

## §6 Hasura Permissions

| Role | sendEmail | getEmailStatus | listDomains | upsertDomain | listTemplates | upsertTemplate |
|------|-----------|----------------|-------------|--------------|---------------|----------------|
| admin | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| user | ✓ (own tenant) | own only | ✗ | ✗ | own only | ✗ |
| service | ✓ | own only | ✗ | ✗ | own only | ✗ |
| anonymous | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |

Hasura row-level filter on all tables: `{"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}`.

---

## §7 Bundle Membership

- **Bundle:** `shared` — no single bundle owns this plugin.
- **License requirement:** `pro` tier or higher (`any-bundle`, `nself-plus`, `cloud`).
- **Install:** `nself plugin install transactional-email` (requires any active pro license).
- **Bundle classification (F06 update target):** Shared pro plugin — add to F06 as `transactional-email → shared-pro` after implementation lands.

---

## §8 Pricing Impact

- No per-bundle price change. Plugin is available to all existing paid tiers.
- nSelf+ ($3.99/mo or $39.99/yr) — included.
- Individual bundle ($0.99/mo) — included.
- Free tier — excluded.
- No per-email cost to nSelf (cost is borne by the user's email provider account).

---

## §9 Competitive Parity

Self-hosted nSelf operators currently have no native transactional email abstraction — they call providers directly or use the bare `notify` plugin stub. Commercial alternatives include SendGrid ($19.95+/mo), Mailgun ($35+/mo), and Postmark ($15+/mo), all of which charge per email volume and require vendor lock-in.

nSelf's transactional-email plugin is self-hosted with no per-email cost from nSelf's side; provider cost is entirely optional and at the operator's chosen rate. The provider-agnostic abstraction (Elastic Email default, any SMTP or HTTP API) means operators can switch providers without any code change. Template management, SPF/DKIM verification, and bounce tracking are built in at no extra cost.

---

## §10 Provider Strategy

**Default provider:** `elastic_email` — set via `EMAIL_DEFAULT_PROVIDER=elastic_email`.

**Supported providers (v1.1.0):**

| Provider | Type | Config env vars |
|----------|------|-----------------|
| `elastic_email` | HTTP v4 API | `EMAIL_ELASTIC_API_KEY` |
| `smtp` | SMTP | `EMAIL_SMTP_HOST`, `EMAIL_SMTP_PORT`, `EMAIL_SMTP_USER`, `EMAIL_SMTP_PASS` |
| `postmark` | HTTP API | `EMAIL_POSTMARK_SERVER_TOKEN` |

**Override per-request:** clients may specify `provider_override` in the `sendEmail` payload to route a single message through a different configured provider.

**Default provider override:** `TRANSACTIONAL_EMAIL_PROVIDER` env var sets the session default (e.g., `TRANSACTIONAL_EMAIL_PROVIDER=postmark`). This resolves the MF-1 Postmark/Elastic Email alignment: Elastic Email remains the shipped default; Postmark is a fully supported alternate activated via env var, with no spec conflict.

---

## §11 Plugin Manifest

```json
{
  "name": "transactional-email",
  "displayName": "Transactional Email",
  "version": "1.0.0",
  "api_version": "1.0.0",
  "description": "Provider-agnostic transactional email: template rendering, per-tenant domain management, SPF/DKIM reporting, delivery webhook relay.",
  "author": "nself",
  "license": "Source-Available",
  "isCommercial": true,
  "licenseType": "pro",
  "requiredEntitlements": ["pro"],
  "requires_license": true,
  "homepage": "https://nself.org/plugins",
  "repository": "https://github.com/nself-org/plugins-pro",
  "minNselfVersion": "1.1.0",
  "port": 3822,
  "category": "communication",
  "tags": ["email", "transactional", "smtp", "elastic-email", "postmark", "templates", "webhooks"],
  "language": "go",
  "entryPoint": "binary",
  "entry": "cmd/main.go",
  "status": "planned",
  "tier": "pro",
  "bundle": ["shared"],
  "tables": [
    "np_email_domains",
    "np_email_messages",
    "np_email_templates"
  ],
  "hasuraRemoteSchema": true,
  "systemDependencies": [],
  "envVars": {
    "required": ["DATABASE_URL"],
    "optional": [
      "EMAIL_DEFAULT_PROVIDER",
      "EMAIL_ELASTIC_API_KEY",
      "EMAIL_SMTP_HOST",
      "EMAIL_SMTP_PORT",
      "EMAIL_SMTP_USER",
      "EMAIL_SMTP_PASS",
      "EMAIL_POSTMARK_SERVER_TOKEN",
      "EMAIL_RATE_LIMIT_RPM",
      "EMAIL_WEBHOOK_SECRET",
      "TRANSACTIONAL_EMAIL_PROVIDER"
    ]
  },
  "config": {
    "defaultProvider": "elastic_email",
    "rateLimitRPM": 100,
    "rateLimitBurst": 20
  },
  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "pkStrategy": "uuid",
    "defaultValue": "primary"
  },
  "cloudMultiTenancy": {
    "supported": true,
    "isolationColumn": "tenant_id",
    "hasuraRowFilter": {"tenant_id": {"_eq": "X-Hasura-Tenant-Id"}}
  },
  "webhooks": {
    "email.sent": "Message dispatched to provider",
    "email.delivered": "Provider confirmed delivery",
    "email.bounced": "Provider reported hard bounce",
    "email.failed": "Send attempt failed"
  },
  "actions": {
    "server": "Start transactional-email HTTP server",
    "send": "Send a transactional email (CLI shortcut)",
    "domains": "List configured sender domains",
    "domain-add": "Add and verify a sender domain",
    "stats": "Show email delivery statistics"
  },
  "permissions": {
    "database": ["create", "read", "update", "delete"],
    "network": [
      "api.elasticemail.com",
      "smtp.elasticemail.com",
      "api.postmarkapp.com"
    ],
    "filesystem": ["logs"]
  },
  "hooks": {
    "postInstall": null,
    "preUninstall": null,
    "postSync": null
  },
  "dependencies": {
    "required": [],
    "optional": ["mux", "notify"]
  }
}
```

---

## §12 Doctor Dependency Check

No system-level binary dependencies. All provider calls are pure HTTP or SMTP via Go standard library.

`nself doctor --deep` checks for this plugin:
- `PLUGIN-EMAIL-01`: `EMAIL_DEFAULT_PROVIDER` is one of `elastic_email | smtp | postmark`
- `PLUGIN-EMAIL-02`: If `elastic_email`, verify `EMAIL_ELASTIC_API_KEY` is non-empty
- `PLUGIN-EMAIL-03`: If `smtp`, verify `EMAIL_SMTP_HOST` and `EMAIL_SMTP_USER` are non-empty
- `PLUGIN-EMAIL-04`: At least one sender domain configured in `np_email.domains`

---

## §13 Test Plan

**Unit tests:**
- Provider adapter interface: mock each provider's HTTP/SMTP response, verify `messages.status` transitions
- Template rendering: Go `text/html.Template` with variable substitution, HTML sanitization
- Rate limiter: token-bucket behavior at burst and sustained rate
- HMAC webhook verification: valid signature → process; tampered → reject with 401

**Integration tests:**
- Docker Compose: real Postgres + SMTP4Dev mock
- Send via `POST /email/send` → verify `np_email.messages.status = 'sent'`
- Send via `POST /email/send-template` → verify template rendered in stored `body_html`
- Inbound delivery webhook relay → verify `messages.status = 'delivered'`

**E2E tests:**
- Hasura Action `sendEmail` mutation → plugin processes → SMTP4Dev captures message → delivery webhook fires → DB status = `delivered`

**Test coverage target:** 80%+ branch on send path; 100% on HMAC verification and rate limiter.

---

## §14 Migration

- Schema applied by `nself plugin install transactional-email` — creates `np_email` schema and role.
- No breaking changes to existing tables.
- Backwards compat: existing Ummat direct Elastic Email calls continue until the tenant migrates domain config to this plugin.
- Ummat migration ticket: `ummat/.claude/inbox/msg-2026-04-30-migrate-to-nself-email.md` (to be filed post-P98).
- Feature flag: `EMAIL_PLUGIN_ENABLED=true` (default `false` until at least one domain is configured).

---

## §15 Rollout

1. `nself plugin install transactional-email` on staging
2. Configure one sender domain via `nself email domain-add`
3. Verify SPF/DKIM: `nself email domains` (shows `spf_verified`, `dkim_verified`)
4. Run integration smoke: `nself email send --to test@example.com --subject "smoke" --body "ok"`
5. Enable in production: set `EMAIL_PLUGIN_ENABLED=true` in `.env.prod`

---

## §16 Bundle Classification

| Classification | Value |
|---------------|-------|
| **Bundle** | `shared` — not exclusive to any single bundle |
| **Tier** | `pro` — requires any active paid license |
| **Version target** | `v1.1.0` |
| **Category** | communication |
| **Free tier** | No — transactional email with domain management is a premium feature |
| **Security-Always-Free exception** | Delivery failure alerting (bounce rate > 10%) is free via `nself doctor --deep` |

---

## §17 Cross-Plugin Coordination

**MF-1 Amendment (08.T14 auth_server email backend):**
The `auth` plugin sends transactional emails for magic links, verification, and password reset. When `transactional-email` is installed, `auth` MUST delegate email dispatch to this plugin's `POST /email/send` endpoint rather than its built-in SMTP stub.

Coordination contract:
- `auth` plugin checks `TRANSACTIONAL_EMAIL_PLUGIN_URL` env var at startup
- If set, auth routes all outbound email through `${TRANSACTIONAL_EMAIL_PLUGIN_URL}/email/send` with HMAC signed payload
- If not set, auth falls back to its own `EMAIL_SMTP_*` vars (built-in stub)
- This plugin does NOT depend on `auth` — auth depends on this plugin optionally

**Optional dependencies:**
- `mux`: mux can route to this plugin for outbound email events (optional integration)
- `notify`: notify can delegate email delivery here when `NOTIFY_EMAIL_BACKEND=transactional-email` is set

---

## §18 Observability

- **Metrics:** `email_messages_total{status,provider}`, `email_send_duration_seconds`, `email_bounce_rate`
- **Logs:** Structured JSON — `{msg_id, tenant_id, status, provider, duration_ms, error}`
- **Traces:** OTel spans on `send`, `render_template`, `provider_call`, `webhook_ingest`
- **Alerts:**
  - `bounce_rate > 5%` over 1h → WARNING
  - `bounce_rate > 10%` over 1h → CRITICAL
  - Provider unreachable > 30s → WARNING

---

## §19 Docs to Create (Post-Implementation)

- `plugins-pro/.github/docs/transactional-email.md` — operator guide
- `web/docs/src/content/plugins/transactional-email.mdx` — public docs
- `plugins-pro/paid/transactional-email/README.md` — plugin README
- SPORT F04 plugin count: +1 when implementation ships
- SPORT F06: add `transactional-email → shared-pro` when implementation ships

---

## Open Questions

1. **notify coexistence:** Should this plugin replace the free `notify` plugin's email path or coexist? Recommendation: coexist — `notify` handles simple notifications, this handles transactional with templates + domain management.
2. **Ummat migration:** Who owns cutting over Ummat M2/S11/S14 to use this plugin? (Post-P98 cross-project ticket.)
