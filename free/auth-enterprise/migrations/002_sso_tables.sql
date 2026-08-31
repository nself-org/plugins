-- 002_sso_tables.sql
-- auth-enterprise plugin — SSO provider registry, session log, and CSRF state cache.
--
-- SSO (SAML 2.0 + OIDC) is gated by the NSELF_SSO=true feature flag and requires
-- a ɳSelf+ license. Tables live under the np_ prefix for consistency with the
-- rest of the nSelf schema.

-- ---------------------------------------------------------------------------
-- np_sso_providers
-- One row per configured IdP (Google Workspace, Okta, Entra ID, or custom).
-- metadata JSONB stores protocol-specific config (SSOURL, entityIDs, client
-- credentials, discovery URL, etc.) — never log this column.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS np_sso_providers (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        REFERENCES np_tenants(id) ON DELETE CASCADE,
    protocol        TEXT        NOT NULL CHECK (protocol IN ('saml', 'oidc')),
    idp_name        TEXT        NOT NULL,   -- google | okta | entra | custom
    -- JSONB blob: SAMLProvider or OIDCProvider fields (excluding client_secret).
    -- The Go handler encrypts client_secret at rest before writing here.
    metadata        JSONB       NOT NULL DEFAULT '{}',
    -- Maps canonical claim names (email, name, role) to IdP-specific claim keys.
    attribute_map   JSONB       NOT NULL DEFAULT '{}',
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE np_sso_providers ENABLE ROW LEVEL SECURITY;

-- Only service-role (admin) can manage provider records.
CREATE POLICY np_sso_providers_service
    ON np_sso_providers
    USING (true)
    WITH CHECK (true);

CREATE INDEX IF NOT EXISTS idx_np_sso_providers_tenant
    ON np_sso_providers (tenant_id);

CREATE INDEX IF NOT EXISTS idx_np_sso_providers_tenant_enabled
    ON np_sso_providers (tenant_id, enabled)
    WHERE enabled = TRUE;

-- ---------------------------------------------------------------------------
-- np_sso_sessions
-- Audit log of every SSO-authenticated login.
-- One row per login event — never updated after insert.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS np_sso_sessions (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        TEXT        NOT NULL,
    -- FK to np_sso_providers; NULL if provider was deleted after session was created.
    provider_id    UUID        REFERENCES np_sso_providers(id) ON DELETE SET NULL,
    -- Subject identifier from the IdP (NameID for SAML, sub for OIDC).
    idp_subject    TEXT        NOT NULL,
    signed_in_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE np_sso_sessions ENABLE ROW LEVEL SECURITY;

-- Users can read their own session history; service role can read all.
CREATE POLICY np_sso_sessions_owner
    ON np_sso_sessions
    FOR SELECT
    USING (user_id = current_setting('app.current_user_id', true));

CREATE POLICY np_sso_sessions_service_insert
    ON np_sso_sessions
    FOR INSERT
    WITH CHECK (true);

CREATE INDEX IF NOT EXISTS idx_np_sso_sessions_user
    ON np_sso_sessions (user_id);

CREATE INDEX IF NOT EXISTS idx_np_sso_sessions_provider
    ON np_sso_sessions (provider_id);

CREATE INDEX IF NOT EXISTS idx_np_sso_sessions_signed_in
    ON np_sso_sessions (signed_in_at DESC);

-- ---------------------------------------------------------------------------
-- np_sso_state_cache
-- Short-lived CSRF state tokens for OIDC and SAML flows.
--
-- Flow:
--   1. /auth/sso/oidc/begin  or /auth/sso/saml/<id>/begin
--      → INSERT state token + provider_id + expires_at (5 min)
--   2. Callback receives state= parameter
--      → DELETE FROM np_sso_state_cache WHERE token=$1 AND expires_at > NOW()
--        RETURNING provider_id  -- exactly-once consumption
--
-- Rows with expires_at < NOW() are garbage-collected by the nightly vacuum or
-- an explicit DELETE run on each inbound callback (best-effort cleanup).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS np_sso_state_cache (
    -- 'state' matches the OAuth2/SAML state parameter name used by the handlers.
    state       TEXT        PRIMARY KEY,
    provider_id UUID        NOT NULL REFERENCES np_sso_providers(id) ON DELETE CASCADE,
    -- tenant_id is denormalized here so the callback handler can retrieve it
    -- without a JOIN to np_sso_providers (avoids extra round-trip on hot path).
    tenant_id   UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '10 minutes')
);

ALTER TABLE np_sso_state_cache ENABLE ROW LEVEL SECURITY;

-- State cache is managed by the service role only (no user-owned rows).
CREATE POLICY np_sso_state_cache_service
    ON np_sso_state_cache
    USING (true)
    WITH CHECK (true);

-- Index on expires_at so DELETE-WHERE-expires_at-gt-NOW() is fast.
CREATE INDEX IF NOT EXISTS idx_np_sso_state_cache_expires
    ON np_sso_state_cache (expires_at);
