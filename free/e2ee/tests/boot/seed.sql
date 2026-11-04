-- Upgrade-proof seed (D-0832). The basis (codemod) build created the key-directory tables only in np_e2ee (001 ran
-- under the pinned search_path and e2ee has no Go DDL), so the public tables are basis-absent: the converted boot
-- must create them. The np_e2ee copies (extra-tables) get one row each and must keep it. Runs as superuser (RLS bypassed).
INSERT INTO np_e2ee.np_e2ee_identity_keys (id, user_id, device_id, identity_key_public, registration_id) VALUES ('00000000-0000-4000-8000-0000000000e1', 'd0832-user', 'dev1', '\x00'::bytea, 1);
INSERT INTO np_e2ee.np_e2ee_signed_prekeys (id, user_id, device_id, key_id, public_key, signature) VALUES ('00000000-0000-4000-8000-0000000000e2', 'd0832-user', 'dev1', 1, '\x00'::bytea, '\x00'::bytea);
INSERT INTO np_e2ee.np_e2ee_one_time_prekeys (id, user_id, device_id, key_id, public_key) VALUES ('00000000-0000-4000-8000-0000000000e3', 'd0832-user', 'dev1', 1, '\x00'::bytea);
INSERT INTO np_e2ee.np_e2ee_kyber_prekeys (id, user_id, device_id, key_id, kyber_public_key, kyber_signature) VALUES ('00000000-0000-4000-8000-0000000000e4', 'd0832-user', 'dev1', 1, '\x00'::bytea, '\x00'::bytea);
INSERT INTO np_e2ee.np_e2ee_prekey_bundles_served (id, target_user_id, target_device_id, requested_by) VALUES ('00000000-0000-4000-8000-0000000000e5', 'd0832-user', 'dev1', 'd0832-peer');
INSERT INTO np_e2ee.np_e2ee_verification_states (id, user_id, peer_user_id) VALUES ('00000000-0000-4000-8000-0000000000e6', 'd0832-user', 'd0832-peer');
INSERT INTO np_e2ee.np_e2ee_safety_numbers (id, user_id, peer_user_id, safety_number) VALUES ('00000000-0000-4000-8000-0000000000e7', 'd0832-user', 'd0832-peer', '00000');
INSERT INTO np_e2ee.np_e2ee_audit_log (id, user_id, event_type) VALUES ('00000000-0000-4000-8000-0000000000e8', 'd0832-user', 'seed');
