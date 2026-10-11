-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Parent rows first. Test data only.
INSERT INTO np_jobs_queues (id, name) VALUES ('seed-queue', 'seed-queue-name');
INSERT INTO np_jobs_jobs (id, queue, status) VALUES ('seed-job', 'seed-queue-name', 'completed');
INSERT INTO np_jobs_history (id, job_id, attempt, success) VALUES ('seed-history', 'seed-job', 1, true);
