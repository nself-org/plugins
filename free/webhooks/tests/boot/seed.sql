-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Parent rows first. Test data only.
INSERT INTO np_webhooks_endpoints (id, url, secret, events) VALUES ('00000000-0000-4000-8000-0000000000f1', 'https://example.invalid/seed', 'whsec_seed', '{user.created}');
INSERT INTO np_webhooks_deliveries (id, endpoint_id, event_type, payload) VALUES ('00000000-0000-4000-8000-0000000000f2', '00000000-0000-4000-8000-0000000000f1', 'user.created', '{}');
