-- Purpose: Drop all np_web3_* tables (reverse of 001_np_web3_schema_up.sql).
-- Inputs: None.
-- Outputs: All 12 np_web3_* tables and their indexes removed.
-- Constraints: Cascades included; drop order respects FK dependencies.

BEGIN;

DROP TABLE IF EXISTS np_web3_events CASCADE;
DROP TABLE IF EXISTS np_web3_transactions CASCADE;
DROP TABLE IF EXISTS np_web3_votes CASCADE;
DROP TABLE IF EXISTS np_web3_proposals CASCADE;
DROP TABLE IF EXISTS np_web3_daos CASCADE;
DROP TABLE IF EXISTS np_web3_gate_checks CASCADE;
DROP TABLE IF EXISTS np_web3_token_gates CASCADE;
DROP TABLE IF EXISTS np_web3_token_balances CASCADE;
DROP TABLE IF EXISTS np_web3_nfts CASCADE;
DROP TABLE IF EXISTS np_web3_collections CASCADE;
DROP TABLE IF EXISTS np_web3_tokens CASCADE;
DROP TABLE IF EXISTS np_web3_wallets CASCADE;

COMMIT;
