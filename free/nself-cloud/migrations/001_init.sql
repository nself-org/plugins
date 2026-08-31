-- Migration 001: nself-cloud master tenant schema
-- Plugin: nself-cloud (ɳCloud internal — not a consumer bundle)
-- Port: 3845
-- Multi-Tenant Convention Wall (PPI Hard Rule 2026-04-20):
--   ALL np_cloud_* tables (except np_cloud_tenants itself) use tenant_id UUID.
--   NEVER use source_account_id here — these are Cloud customers, not multi-app isolation.
--   Hasura row filter required on every table with tenant_id: {"tenant_id":{"_eq":"X-Hasura-Tenant-Id"}}
-- RLS: ENABLE + FORCE ROW LEVEL SECURITY on all tables except np_cloud_tenants (it IS the tenant).
-- PatternTenantScoped policy: USING (tenant_id = current_setting('app.current_tenant_id')::UUID)

-- ─────────────────────────────────────────────────────────────────
-- 1. np_cloud_tenants — master tenant table (no tenant_id; it IS the tenant)
-- ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS np_cloud_tenants (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id        TEXT        NOT NULL,                     -- Hasura auth user id
    name                 TEXT        NOT NULL,
    slug                 TEXT        NOT NULL UNIQUE,              -- url-safe, 3-30 chars
    plan_tier            TEXT        NOT NULL DEFAULT 'trialing'
                             CHECK (plan_tier IN ('trialing', 'active', 'past_due', 'canceled', 'suspended')),
    stripe_customer_id   TEXT,
    ls_customer_id       TEXT,                                     -- Lemon Squeezy backup
    trial_ends_at        TIMESTAMPTZ,
    provisioning_enabled BOOLEAN     NOT NULL DEFAULT false,       -- per-tenant kill-switch
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ                               -- soft-delete
);

CREATE INDEX IF NOT EXISTS idx_np_cloud_tenants_owner_user_id
    ON np_cloud_tenants (owner_user_id);
CREATE INDEX IF NOT EXISTS idx_np_cloud_tenants_slug
    ON np_cloud_tenants (slug);
CREATE INDEX IF NOT EXISTS idx_np_cloud_tenants_plan_tier
    ON np_cloud_tenants (plan_tier);

-- np_cloud_tenants: no RLS (it is the root isolation boundary)
-- Hasura: nself_admin role only; no user-facing direct access (users access via API layer)

-- ─────────────────────────────────────────────────────────────────
-- 2. np_cloud_instances — one row per provisioned VPS
-- ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS np_cloud_instances (
    id                       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                UUID        NOT NULL REFERENCES np_cloud_tenants (id) ON DELETE CASCADE,
    slug                     TEXT        NOT NULL UNIQUE,          -- url-safe, 3-30 chars
    region                   TEXT        NOT NULL,                 -- 'fsn1' | 'nbg1'
    server_type              TEXT        NOT NULL DEFAULT 'cx23',
    hetzner_server_id        BIGINT,                               -- NULL until provisioned
    byoh_mode                BOOLEAN     NOT NULL DEFAULT false,
    byoh_token_encrypted     TEXT,                                 -- AES-256-GCM, key=NSELF_DB_ENCRYPTION_KEY
    ipv4                     TEXT,
    ipv6                     TEXT,
    status                   TEXT        NOT NULL DEFAULT 'pending'
                                 CHECK (status IN (
                                     'pending', 'provisioning', 'bootstrapping',
                                     'running', 'provision_failed',
                                     'teardown_pending', 'torn_down', 'suspended'
                                 )),
    provision_attempt        INT         NOT NULL DEFAULT 0,
    provision_started_at     TIMESTAMPTZ,
    provision_finished_at    TIMESTAMPTZ,
    last_health_check_at     TIMESTAMPTZ,
    health_status            TEXT
                                 CHECK (health_status IS NULL OR health_status IN
                                     ('healthy', 'degraded', 'down', 'unknown')),
    nself_version            TEXT,
    license_key              TEXT,
    stripe_subscription_id   TEXT,
    ls_subscription_id       TEXT,
    hourly_rate_cents        INT         NOT NULL DEFAULT 0,       -- at-cost passthrough
    ssl_status               TEXT        NOT NULL DEFAULT 'pending'
                                 CHECK (ssl_status IN ('pending', 'issued', 'failed', 'renewing')),
    custom_domain            TEXT,
    domain_verified          BOOLEAN     NOT NULL DEFAULT false,
    lets_encrypt_issued      BOOLEAN     NOT NULL DEFAULT false,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_cloud_instances_tenant_id
    ON np_cloud_instances (tenant_id);
CREATE INDEX IF NOT EXISTS idx_np_cloud_instances_status
    ON np_cloud_instances (status);
CREATE INDEX IF NOT EXISTS idx_np_cloud_instances_slug
    ON np_cloud_instances (slug);

-- RLS: tenant_id isolation
ALTER TABLE np_cloud_instances ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cloud_instances FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON np_cloud_instances
    USING (tenant_id = current_setting('app.current_tenant_id')::UUID);

-- ─────────────────────────────────────────────────────────────────
-- 3. np_cloud_billing_events — Stripe/LS metered records
--    Idempotency: UNIQUE on (instance_id, hour) for usage records
-- ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS np_cloud_billing_events (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID        NOT NULL REFERENCES np_cloud_tenants (id) ON DELETE CASCADE,
    instance_id          UUID        REFERENCES np_cloud_instances (id) ON DELETE SET NULL,
    event_type           TEXT        NOT NULL
                             CHECK (event_type IN (
                                 'subscription_created', 'subscription_canceled',
                                 'invoice_paid', 'invoice_failed',
                                 'usage_record_emitted',
                                 'trial_started', 'trial_ended',
                                 'refund_issued'
                             )),
    provider             TEXT        NOT NULL
                             CHECK (provider IN ('stripe', 'lemonsqueezy')),
    provider_event_id    TEXT,
    amount_cents         INT,
    currency             TEXT        NOT NULL DEFAULT 'usd',
    idempotency_key      TEXT        UNIQUE,                       -- (instance_id::text || ':' || hour::text) for usage records
    metadata             JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status               TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'processed', 'failed')),
    error_message        TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Composite idempotency: usage records keyed by (instance_id, billing_hour)
CREATE UNIQUE INDEX IF NOT EXISTS idx_np_cloud_billing_events_instance_hour
    ON np_cloud_billing_events (instance_id, (metadata->>'billing_hour'))
    WHERE event_type = 'usage_record_emitted' AND instance_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_np_cloud_billing_events_tenant_id
    ON np_cloud_billing_events (tenant_id);
CREATE INDEX IF NOT EXISTS idx_np_cloud_billing_events_instance_id
    ON np_cloud_billing_events (instance_id);

-- RLS: tenant_id isolation
ALTER TABLE np_cloud_billing_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cloud_billing_events FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON np_cloud_billing_events
    USING (tenant_id = current_setting('app.current_tenant_id')::UUID);

-- ─────────────────────────────────────────────────────────────────
-- 4. np_cloud_domains — custom domain bindings
-- ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS np_cloud_domains (
    id                       UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                UUID        NOT NULL REFERENCES np_cloud_tenants (id) ON DELETE CASCADE,
    instance_id              UUID        NOT NULL REFERENCES np_cloud_instances (id) ON DELETE CASCADE,
    hostname                 TEXT        NOT NULL,                 -- RFC 1123 validated
    cname_target             TEXT        NOT NULL,                 -- <slug>.nself.cloud
    status                   TEXT        NOT NULL DEFAULT 'pending'
                                 CHECK (status IN ('pending', 'verified', 'failed', 'removed')),
    dns_verified_at          TIMESTAMPTZ,
    ssl_issued_at            TIMESTAMPTZ,
    ssl_expires_at           TIMESTAMPTZ,
    ssl_status               TEXT        NOT NULL DEFAULT 'pending'
                                 CHECK (ssl_status IN ('pending', 'issued', 'failed', 'renewing')),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_np_cloud_domains_hostname
    ON np_cloud_domains (hostname);
CREATE INDEX IF NOT EXISTS idx_np_cloud_domains_tenant_id
    ON np_cloud_domains (tenant_id);
CREATE INDEX IF NOT EXISTS idx_np_cloud_domains_instance_id
    ON np_cloud_domains (instance_id);

-- RLS: tenant_id isolation
ALTER TABLE np_cloud_domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cloud_domains FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON np_cloud_domains
    USING (tenant_id = current_setting('app.current_tenant_id')::UUID);

-- ─────────────────────────────────────────────────────────────────
-- 5. np_cloud_invitations — team member invitations
-- ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS np_cloud_invitations (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID        NOT NULL REFERENCES np_cloud_tenants (id) ON DELETE CASCADE,
    instance_id          UUID        NOT NULL REFERENCES np_cloud_instances (id) ON DELETE CASCADE,
    invited_email        TEXT        NOT NULL,
    role                 TEXT        NOT NULL DEFAULT 'viewer'
                             CHECK (role IN ('admin', 'member', 'viewer')),
    invited_by_user_id   TEXT        NOT NULL,
    token                TEXT        NOT NULL UNIQUE,              -- secure random, used for accept link
    expires_at           TIMESTAMPTZ NOT NULL,
    accepted_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_cloud_invitations_tenant_id
    ON np_cloud_invitations (tenant_id);
CREATE INDEX IF NOT EXISTS idx_np_cloud_invitations_instance_id
    ON np_cloud_invitations (instance_id);
CREATE INDEX IF NOT EXISTS idx_np_cloud_invitations_token
    ON np_cloud_invitations (token);

-- RLS: tenant_id isolation
ALTER TABLE np_cloud_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cloud_invitations FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON np_cloud_invitations
    USING (tenant_id = current_setting('app.current_tenant_id')::UUID);

-- ─────────────────────────────────────────────────────────────────
-- 6. np_cloud_team_memberships — active team members per instance
-- ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS np_cloud_team_memberships (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        NOT NULL REFERENCES np_cloud_tenants (id) ON DELETE CASCADE,
    instance_id     UUID        NOT NULL REFERENCES np_cloud_instances (id) ON DELETE CASCADE,
    user_id         TEXT        NOT NULL,
    role            TEXT        NOT NULL DEFAULT 'viewer'
                        CHECK (role IN ('admin', 'member', 'viewer')),
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_np_cloud_memberships_instance_user
    ON np_cloud_team_memberships (instance_id, user_id);
CREATE INDEX IF NOT EXISTS idx_np_cloud_memberships_tenant_id
    ON np_cloud_team_memberships (tenant_id);

-- RLS: tenant_id isolation
ALTER TABLE np_cloud_team_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE np_cloud_team_memberships FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON np_cloud_team_memberships
    USING (tenant_id = current_setting('app.current_tenant_id')::UUID);

-- ─────────────────────────────────────────────────────────────────
-- Triggers: updated_at auto-maintenance on mutable tables
-- ─────────────────────────────────────────────────────────────────
CREATE OR REPLACE FUNCTION np_cloud_set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER np_cloud_tenants_updated_at
    BEFORE UPDATE ON np_cloud_tenants
    FOR EACH ROW EXECUTE FUNCTION np_cloud_set_updated_at();

CREATE TRIGGER np_cloud_instances_updated_at
    BEFORE UPDATE ON np_cloud_instances
    FOR EACH ROW EXECUTE FUNCTION np_cloud_set_updated_at();
