-- WARNING: UNMAINTAINED DOWN MIGRATION. DO NOT RUN.
-- Historical SQL only; it may not reverse the current up migration and may destroy data.
-- See .github/wiki/Plugin-Development.md (Migration rollback policy).
-- plugin-storage: rollback initial schema
DROP TABLE IF EXISTS np_storage_metadata CASCADE;
DROP TABLE IF EXISTS np_storage_objects CASCADE;
DROP TABLE IF EXISTS np_storage_buckets CASCADE;
