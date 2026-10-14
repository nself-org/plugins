-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema (the old Go DDL) before the upgrade. Test data only.
INSERT INTO np_push_devices (device_token, platform, app_id) VALUES ('seed-token', 'ios', 'default');
INSERT INTO np_push_outbox (device_token, platform, payload, dedupe_hash) VALUES ('seed-token', 'ios', '{}', 'seed-hash');
