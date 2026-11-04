-- 004_public_rls_consume.sql
-- 002_e2ee_rls_consume.sql's policies on the public tables 003 creates (D-0832); 002 is released and frozen, and ran
-- against the np_e2ee copies. Same policies, same names: a same-app requester may consume an unconsumed one-time or
-- Kyber prekey and record its own served-bundle audit row. Rationale: see 002.
-- Idempotent: DROP POLICY IF EXISTS before each CREATE POLICY.

-- Allow any same-app authenticated user to consume an unconsumed one-time prekey.
-- WITH CHECK forces the post-image to remain in-app and to be the consumed state,
-- so this policy cannot be used to un-consume or to move a row to another app.
DROP POLICY IF EXISTS np_e2ee_one_time_prekeys_consume ON public.np_e2ee_one_time_prekeys;
CREATE POLICY np_e2ee_one_time_prekeys_consume ON public.np_e2ee_one_time_prekeys
    FOR UPDATE
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND is_consumed = false
    )
    WITH CHECK (
        source_account_id = current_setting('app.source_account_id', true)
        AND is_consumed = true
    );

-- Same for Kyber (ML-KEM-1024) one-time prekeys.
DROP POLICY IF EXISTS np_e2ee_kyber_prekeys_consume ON public.np_e2ee_kyber_prekeys;
CREATE POLICY np_e2ee_kyber_prekeys_consume ON public.np_e2ee_kyber_prekeys
    FOR UPDATE
    USING (
        source_account_id = current_setting('app.source_account_id', true)
        AND is_consumed = false
    )
    WITH CHECK (
        source_account_id = current_setting('app.source_account_id', true)
        AND is_consumed = true
    );

-- The served-bundle audit row is INSERTed by the REQUESTER (not the key owner),
-- so 001's owner-only default would deny it. Allow a same-app requester to
-- record their own served-bundle audit row.
DROP POLICY IF EXISTS np_e2ee_prekey_bundles_served_insert ON public.np_e2ee_prekey_bundles_served;
CREATE POLICY np_e2ee_prekey_bundles_served_insert ON public.np_e2ee_prekey_bundles_served
    FOR INSERT
    WITH CHECK (
        source_account_id = current_setting('app.source_account_id', true)
        AND requested_by = current_setting('app.current_user_id', true)
    );
