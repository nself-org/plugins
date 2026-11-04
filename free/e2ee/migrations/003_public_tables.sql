-- 003_public_tables.sql
-- nself e2ee plugin: the key-directory tables in public, where the runtime queries them (D-0832).
-- 001_e2ee_init.sql ran with search_path pinned to np_e2ee, so its tables landed in that schema and the runtime (which
-- queries public) found none. 001 is released and frozen (migration-immutable): this file repeats it with every table
-- qualified, same columns, RLS (ENABLE + FORCE) and policies. Doctrine, isolation and table mapping: see 001.
-- The np_e2ee copies are left in place: nothing is dropped or moved. 004 repeats 002's consume policies.
-- Idempotent: CREATE ... IF NOT EXISTS, and DROP POLICY IF EXISTS before each CREATE POLICY.


-- ===========================================================================
-- np_e2ee_identity_keys — long-term Ed25519/X25519 PUBLIC identity key per device
-- ===========================================================================
CREATE TABLE IF NOT EXISTS public.np_e2ee_identity_keys (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id   TEXT        NOT NULL DEFAULT 'primary',
    user_id             TEXT        NOT NULL,
    device_id           TEXT        NOT NULL,
    identity_key_public BYTEA       NOT NULL,
    registration_id     INTEGER     NOT NULL,
    is_active           BOOLEAN     NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at        TIMESTAMPTZ,
    UNIQUE (source_account_id, user_id, device_id)
);

ALTER TABLE public.np_e2ee_identity_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_e2ee_identity_keys FORCE ROW LEVEL SECURITY;

-- Owner read/write of own identity rows.
DROP POLICY IF EXISTS np_e2ee_identity_keys_owner ON public.np_e2ee_identity_keys;
CREATE POLICY np_e2ee_identity_keys_owner ON public.np_e2ee_identity_keys
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND user_id = current_setting('app.current_user_id', true)
    );

-- Public-fetch policy: any user in the same app may READ another user's PUBLIC
-- identity key (so Bob can build a bundle for Alice). Only public columns exist
-- in this table, so no PII leaks. SELECT only.
DROP POLICY IF EXISTS np_e2ee_identity_keys_public_read ON public.np_e2ee_identity_keys;
CREATE POLICY np_e2ee_identity_keys_public_read ON public.np_e2ee_identity_keys
    FOR SELECT
    USING (source_account_id = current_setting('app.source_account_id', true));

CREATE INDEX IF NOT EXISTS idx_np_e2ee_identity_keys_user
    ON public.np_e2ee_identity_keys (source_account_id, user_id, device_id);

-- ===========================================================================
-- np_e2ee_signed_prekeys — medium-term signed prekey (PUBLIC + Ed25519 signature)
-- ===========================================================================
CREATE TABLE IF NOT EXISTS public.np_e2ee_signed_prekeys (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    device_id         TEXT        NOT NULL,
    key_id            INTEGER     NOT NULL,
    public_key        BYTEA       NOT NULL,
    signature         BYTEA       NOT NULL,
    is_active         BOOLEAN     NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at        TIMESTAMPTZ,
    UNIQUE (source_account_id, user_id, device_id, key_id)
);

ALTER TABLE public.np_e2ee_signed_prekeys ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_e2ee_signed_prekeys FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS np_e2ee_signed_prekeys_owner ON public.np_e2ee_signed_prekeys;
CREATE POLICY np_e2ee_signed_prekeys_owner ON public.np_e2ee_signed_prekeys
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND user_id = current_setting('app.current_user_id', true)
    );

DROP POLICY IF EXISTS np_e2ee_signed_prekeys_public_read ON public.np_e2ee_signed_prekeys;
CREATE POLICY np_e2ee_signed_prekeys_public_read ON public.np_e2ee_signed_prekeys
    FOR SELECT
    USING (source_account_id = current_setting('app.source_account_id', true));

CREATE INDEX IF NOT EXISTS idx_np_e2ee_signed_prekeys_active
    ON public.np_e2ee_signed_prekeys (source_account_id, user_id, device_id, is_active)
    WHERE is_active = true;

-- ===========================================================================
-- np_e2ee_one_time_prekeys — classic X25519 one-time PUBLIC prekeys (consumed once)
-- ===========================================================================
CREATE TABLE IF NOT EXISTS public.np_e2ee_one_time_prekeys (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    device_id         TEXT        NOT NULL,
    key_id            INTEGER     NOT NULL,
    public_key        BYTEA       NOT NULL,
    is_consumed       BOOLEAN     NOT NULL DEFAULT false,
    consumed_at       TIMESTAMPTZ,
    consumed_by       TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_account_id, user_id, device_id, key_id)
);

ALTER TABLE public.np_e2ee_one_time_prekeys ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_e2ee_one_time_prekeys FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS np_e2ee_one_time_prekeys_owner ON public.np_e2ee_one_time_prekeys;
CREATE POLICY np_e2ee_one_time_prekeys_owner ON public.np_e2ee_one_time_prekeys
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND user_id = current_setting('app.current_user_id', true)
    );

-- Public read of UNCONSUMED prekeys so an initiator can claim one.
DROP POLICY IF EXISTS np_e2ee_one_time_prekeys_public_read ON public.np_e2ee_one_time_prekeys;
CREATE POLICY np_e2ee_one_time_prekeys_public_read ON public.np_e2ee_one_time_prekeys
    FOR SELECT
    USING (source_account_id = current_setting('app.source_account_id', true));

-- Fast path: pick the next unconsumed OTPK for a target device.
CREATE INDEX IF NOT EXISTS idx_np_e2ee_one_time_prekeys_unconsumed
    ON public.np_e2ee_one_time_prekeys (source_account_id, user_id, device_id, key_id)
    WHERE is_consumed = false;

-- ===========================================================================
-- np_e2ee_kyber_prekeys — Kyber-1024 (ML-KEM-1024) one-time PUBLIC prekeys
-- This is the post-quantum prekey table. Signature is Ed25519 over the
-- Kyber public key, verifiable against the device identity key.
-- ===========================================================================
CREATE TABLE IF NOT EXISTS public.np_e2ee_kyber_prekeys (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    device_id         TEXT        NOT NULL,
    key_id            INTEGER     NOT NULL,
    kyber_public_key  BYTEA       NOT NULL,
    kyber_signature   BYTEA       NOT NULL,
    is_consumed       BOOLEAN     NOT NULL DEFAULT false,
    consumed_at       TIMESTAMPTZ,
    consumed_by       TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_account_id, user_id, device_id, key_id)
);

ALTER TABLE public.np_e2ee_kyber_prekeys ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_e2ee_kyber_prekeys FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS np_e2ee_kyber_prekeys_owner ON public.np_e2ee_kyber_prekeys;
CREATE POLICY np_e2ee_kyber_prekeys_owner ON public.np_e2ee_kyber_prekeys
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND user_id = current_setting('app.current_user_id', true)
    );

DROP POLICY IF EXISTS np_e2ee_kyber_prekeys_public_read ON public.np_e2ee_kyber_prekeys;
CREATE POLICY np_e2ee_kyber_prekeys_public_read ON public.np_e2ee_kyber_prekeys
    FOR SELECT
    USING (source_account_id = current_setting('app.source_account_id', true));

CREATE INDEX IF NOT EXISTS idx_np_e2ee_kyber_prekeys_unconsumed
    ON public.np_e2ee_kyber_prekeys (source_account_id, user_id, device_id, key_id)
    WHERE is_consumed = false;

-- ===========================================================================
-- np_e2ee_prekey_bundles_served — audit record of which bundle was served
-- (which OTPK/Kyber prekey was handed to which initiator, for replay analysis).
-- ===========================================================================
CREATE TABLE IF NOT EXISTS public.np_e2ee_prekey_bundles_served (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id    TEXT        NOT NULL DEFAULT 'primary',
    target_user_id       TEXT        NOT NULL,
    target_device_id     TEXT        NOT NULL,
    requested_by         TEXT        NOT NULL,
    signed_prekey_id     INTEGER,
    one_time_prekey_id   INTEGER,
    kyber_prekey_id      INTEGER,
    served_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE public.np_e2ee_prekey_bundles_served ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_e2ee_prekey_bundles_served FORCE ROW LEVEL SECURITY;

-- Requester or target owner may read their own served-bundle audit rows.
DROP POLICY IF EXISTS np_e2ee_prekey_bundles_served_owner ON public.np_e2ee_prekey_bundles_served;
CREATE POLICY np_e2ee_prekey_bundles_served_owner ON public.np_e2ee_prekey_bundles_served
    FOR SELECT
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND (
            requested_by = current_setting('app.current_user_id', true)
            OR target_user_id = current_setting('app.current_user_id', true)
        )
    );

CREATE INDEX IF NOT EXISTS idx_np_e2ee_bundles_served_target
    ON public.np_e2ee_prekey_bundles_served (source_account_id, target_user_id, target_device_id);

-- ===========================================================================
-- np_e2ee_verification_states — per-peer verification state (safety-number flow)
-- ===========================================================================
CREATE TABLE IF NOT EXISTS public.np_e2ee_verification_states (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    peer_user_id      TEXT        NOT NULL,
    state             TEXT        NOT NULL DEFAULT 'unverified',
    state_data        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_account_id, user_id, peer_user_id)
);

ALTER TABLE public.np_e2ee_verification_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_e2ee_verification_states FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS np_e2ee_verification_states_owner ON public.np_e2ee_verification_states;
CREATE POLICY np_e2ee_verification_states_owner ON public.np_e2ee_verification_states
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND user_id = current_setting('app.current_user_id', true)
    );

CREATE INDEX IF NOT EXISTS idx_np_e2ee_verification_states_user
    ON public.np_e2ee_verification_states (source_account_id, user_id, peer_user_id);

-- ===========================================================================
-- np_e2ee_safety_numbers — computed safety number per (user, peer) pair
-- ===========================================================================
CREATE TABLE IF NOT EXISTS public.np_e2ee_safety_numbers (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    peer_user_id      TEXT        NOT NULL,
    safety_number     TEXT        NOT NULL,
    is_verified       BOOLEAN     NOT NULL DEFAULT false,
    verified_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_account_id, user_id, peer_user_id)
);

ALTER TABLE public.np_e2ee_safety_numbers ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_e2ee_safety_numbers FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS np_e2ee_safety_numbers_owner ON public.np_e2ee_safety_numbers;
CREATE POLICY np_e2ee_safety_numbers_owner ON public.np_e2ee_safety_numbers
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND user_id = current_setting('app.current_user_id', true)
    );

CREATE INDEX IF NOT EXISTS idx_np_e2ee_safety_numbers_user
    ON public.np_e2ee_safety_numbers (source_account_id, user_id, peer_user_id);

-- ===========================================================================
-- np_e2ee_audit_log — APPEND-ONLY security event log.
-- No UPDATE/DELETE policy is defined => with FORCE RLS those operations are
-- denied for every non-superuser role. Only INSERT + owner SELECT are allowed.
-- ===========================================================================
CREATE TABLE IF NOT EXISTS public.np_e2ee_audit_log (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT        NOT NULL DEFAULT 'primary',
    user_id           TEXT        NOT NULL,
    event_type        TEXT        NOT NULL,
    event_data        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ip_address        INET,
    user_agent        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE public.np_e2ee_audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.np_e2ee_audit_log FORCE ROW LEVEL SECURITY;

-- Append-only: INSERT allowed within own scope.
DROP POLICY IF EXISTS np_e2ee_audit_log_insert ON public.np_e2ee_audit_log;
CREATE POLICY np_e2ee_audit_log_insert ON public.np_e2ee_audit_log
    FOR INSERT
    WITH CHECK (
        source_account_id = current_setting('app.source_account_id', true)
        AND user_id = current_setting('app.current_user_id', true)
    );

-- Owner may read own audit rows. (No UPDATE/DELETE policy => denied.)
DROP POLICY IF EXISTS np_e2ee_audit_log_select ON public.np_e2ee_audit_log;
CREATE POLICY np_e2ee_audit_log_select ON public.np_e2ee_audit_log
    FOR SELECT
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND user_id = current_setting('app.current_user_id', true)
    );

CREATE INDEX IF NOT EXISTS idx_np_e2ee_audit_log_user
    ON public.np_e2ee_audit_log (source_account_id, user_id, created_at DESC);
