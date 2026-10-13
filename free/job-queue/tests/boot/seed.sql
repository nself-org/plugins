-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only, fixed ids so row survival is checked by primary key (parent rows first).
INSERT INTO np_jobs (id, queue, job_type) VALUES ('00000000-0000-4000-8000-000000000001', 'default', 'seed.job');
INSERT INTO np_job_dlq (id, job_id, final_error) VALUES ('00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000001', 'seed error');
INSERT INTO np_circuit_breakers (name) VALUES ('seed-circuit');
