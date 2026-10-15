-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- 001_jobs.down.sql
-- Reverse of 001_jobs.sql

-- Drop in FK-safe order: DLQ references np_jobqueue_jobs, circuit breakers are independent.
DROP TABLE IF EXISTS np_job_dlq;
DROP TABLE IF EXISTS np_jobqueue_jobs;
DROP TABLE IF EXISTS np_circuit_breakers;
