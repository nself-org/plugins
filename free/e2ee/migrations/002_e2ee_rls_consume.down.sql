-- 002_e2ee_rls_consume.down.sql
-- Reverses 002_e2ee_rls_consume.sql.

DROP POLICY IF EXISTS np_e2ee_prekey_bundles_served_insert ON np_e2ee_prekey_bundles_served;
DROP POLICY IF EXISTS np_e2ee_kyber_prekeys_consume ON np_e2ee_kyber_prekeys;
DROP POLICY IF EXISTS np_e2ee_one_time_prekeys_consume ON np_e2ee_one_time_prekeys;
