-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only. The basis tables have no source_account_id column; the upgrade adds it with its default.
INSERT INTO np_backup_jobs (id, status, type) VALUES ('00000000-0000-0000-0000-0000000000b1', 'completed', 'full');
INSERT INTO np_backup_schedules (id, cron_expr) VALUES ('00000000-0000-0000-0000-0000000000b2', '0 3 * * *');
