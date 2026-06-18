-- 002_e2ee_rls_consume.sql
-- CR-C BLOCK fix (P3 2026-06-18): activate RLS for the prekey-bundle flow.
--
-- Background: handlers now run every request inside a transaction that sets
--   app.source_account_id + app.current_user_id (GUCs) from the AUTHENTICATED
--   gateway-forwarded principal, so the policies in 001 finally enforce.
--
-- The one gap 001 left: a prekey BUNDLE is fetched by a *different* user than
-- the key owner (Bob fetches Alice's bundle). 001 only granted owner-scoped
-- write + public SELECT, so the atomic "consume one-time / Kyber prekey" UPDATE
-- (flipping is_consumed) by a non-owner requester would be DENIED under FORCE
-- RLS, breaking the bundle flow. These policies allow a same-app authenticated
-- requester to consume (mark is_consumed) an UNCONSUMED prekey of any user in
-- the same source_account — and nothing else. Owner-only mutation of all other
-- columns / unconsumed→unconsumed is still governed by the 001 owner policy.

-- Allow any same-app authenticated user to consume an unconsumed one-time prekey.
-- WITH CHECK forces the post-image to remain in-app and to be the consumed state,
-- so this policy cannot be used to un-consume or to move a row to another app.
CREATE POLICY np_e2ee_one_time_prekeys_consume ON np_e2ee_one_time_prekeys
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
CREATE POLICY np_e2ee_kyber_prekeys_consume ON np_e2ee_kyber_prekeys
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
CREATE POLICY np_e2ee_prekey_bundles_served_insert ON np_e2ee_prekey_bundles_served
    FOR INSERT
    WITH CHECK (
        source_account_id = current_setting('app.source_account_id', true)
        AND requested_by = current_setting('app.current_user_id', true)
    );
