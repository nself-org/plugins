-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
DROP TABLE IF EXISTS np_mediap_subtitles;
DROP TABLE IF EXISTS np_mediap_hls_manifests;
DROP TABLE IF EXISTS np_mediap_job_outputs;
DROP TABLE IF EXISTS np_mediap_jobs;
DROP TABLE IF EXISTS np_mediap_encoding_profiles;
