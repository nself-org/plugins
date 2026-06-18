-- 001_e2ee_init.down.sql
-- Reverses 001_e2ee_init.sql. Drops all np_e2ee_* key-directory tables.
-- WARNING: dropping these tables discards every published PUBLIC prekey; clients
-- must re-publish identity + prekeys after a rollback. No private key material is
-- ever stored here, so no irrecoverable secret is lost.

DROP TABLE IF EXISTS np_e2ee_audit_log;
DROP TABLE IF EXISTS np_e2ee_safety_numbers;
DROP TABLE IF EXISTS np_e2ee_verification_states;
DROP TABLE IF EXISTS np_e2ee_prekey_bundles_served;
DROP TABLE IF EXISTS np_e2ee_kyber_prekeys;
DROP TABLE IF EXISTS np_e2ee_one_time_prekeys;
DROP TABLE IF EXISTS np_e2ee_signed_prekeys;
DROP TABLE IF EXISTS np_e2ee_identity_keys;
