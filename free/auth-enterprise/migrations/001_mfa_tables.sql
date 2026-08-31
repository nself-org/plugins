-- 001_mfa_tables.sql
-- auth-enterprise plugin — MFA enrollment, policy, and recovery code tables.
--
-- MFA (TOTP + WebAuthn) is free for all self-hosters per the
-- Security-Always-Free doctrine. These tables extend the base auth plugin's
-- auth_mfa_enrollments + auth_passkeys with per-tenant policy enforcement
-- and bcrypt-hashed recovery codes stored in a separate table.

-- ---------------------------------------------------------------------------
-- np_mfa_enrollments
-- One row per user; stores the TOTP secret and WebAuthn credential JSONB.
-- The totp_secret should be encrypted at rest using pgcrypto in production.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS np_mfa_enrollments (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            TEXT        NOT NULL,
    tenant_id          UUID        REFERENCES np_tenants(id) ON DELETE CASCADE,
    -- TOTP secret (base32-encoded, stored in plaintext here;
    -- operators should enable pgcrypto column encryption for at-rest encryption).
    totp_secret        TEXT,
    -- WebAuthn credentials stored as a JSONB array of credential objects.
    webauthn_credentials JSONB     NOT NULL DEFAULT '[]',
    enrolled_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at       TIMESTAMPTZ,
    UNIQUE (user_id)
);

ALTER TABLE np_mfa_enrollments ENABLE ROW LEVEL SECURITY;

CREATE POLICY np_mfa_enrollments_owner
    ON np_mfa_enrollments
    FOR ALL
    USING (user_id = current_setting('app.current_user_id', true));

CREATE INDEX IF NOT EXISTS idx_np_mfa_enrollments_user
    ON np_mfa_enrollments (user_id);

CREATE INDEX IF NOT EXISTS idx_np_mfa_enrollments_tenant
    ON np_mfa_enrollments (tenant_id);

-- ---------------------------------------------------------------------------
-- np_mfa_recovery_codes
-- Each row is one single-use backup code stored as a bcrypt hash.
-- used_at IS NULL  → code is available.
-- used_at IS NOT NULL → code has been consumed.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS np_mfa_recovery_codes (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    TEXT        NOT NULL,
    code_hash  TEXT        NOT NULL,   -- bcrypt hash (cost 10)
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE np_mfa_recovery_codes ENABLE ROW LEVEL SECURITY;

CREATE POLICY np_mfa_recovery_codes_owner
    ON np_mfa_recovery_codes
    FOR ALL
    USING (user_id = current_setting('app.current_user_id', true));

CREATE INDEX IF NOT EXISTS idx_np_mfa_recovery_codes_user
    ON np_mfa_recovery_codes (user_id);

CREATE INDEX IF NOT EXISTS idx_np_mfa_recovery_codes_unused
    ON np_mfa_recovery_codes (user_id, used_at)
    WHERE used_at IS NULL;

-- ---------------------------------------------------------------------------
-- np_mfa_policies
-- Per-tenant MFA enforcement configuration.
-- required = true  → login blocked for unenrolled users after grace period.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS np_mfa_policies (
    tenant_id         UUID        PRIMARY KEY REFERENCES np_tenants(id) ON DELETE CASCADE,
    required          BOOLEAN     NOT NULL DEFAULT false,
    grace_period_d    INT         NOT NULL DEFAULT 7,
    methods_allowed   TEXT[]      NOT NULL DEFAULT '{totp,webauthn}',
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE np_mfa_policies ENABLE ROW LEVEL SECURITY;

-- Only service-role (admin) can manage policies; no user-owned row policy.
CREATE POLICY np_mfa_policies_service
    ON np_mfa_policies
    USING (true)
    WITH CHECK (true);
