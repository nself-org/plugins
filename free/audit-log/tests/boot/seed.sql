-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only. The tenant_id column does not exist in the basis schema; the upgrade adds it.
INSERT INTO np_auditlog_events (id, actor_type, event_type) VALUES ('seed-event', 'system', 'seed.event');
