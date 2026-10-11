-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Parent rows first. Test data only.
INSERT INTO np_tokens_signing_keys (id, name, key_material_encrypted) VALUES ('00000000-0000-4000-8000-000000000001', 'seed-key', 'seed-material');
INSERT INTO np_tokens_issued (signing_key_id, token_hash, user_id, content_id, expires_at) VALUES ('00000000-0000-4000-8000-000000000001', 'seed-hash', 'seed-user', 'seed-content', NOW() + INTERVAL '1 day');
INSERT INTO np_tokens_encryption_keys (content_id, key_material_encrypted, key_iv, key_uri) VALUES ('seed-content', 'seed-material', 'seed-iv', 'seed://uri');
INSERT INTO np_tokens_entitlements (user_id, content_id) VALUES ('seed-user', 'seed-content');
INSERT INTO np_tokens_webhook_events (id, event_type, payload) VALUES ('seed-event', 'seed.event', '{}');
