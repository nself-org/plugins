-- 001_jobs.down.sql
-- Reverse of 001_jobs.sql

-- Drop in FK-safe order: DLQ references np_jobs, circuit breakers are independent.
DROP TABLE IF EXISTS np_job_dlq;
DROP TABLE IF EXISTS np_jobs;
DROP TABLE IF EXISTS np_circuit_breakers;
