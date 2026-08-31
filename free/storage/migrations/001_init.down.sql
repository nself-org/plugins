-- plugin-storage: rollback initial schema
DROP TABLE IF EXISTS np_storage_metadata CASCADE;
DROP TABLE IF EXISTS np_storage_objects CASCADE;
DROP TABLE IF EXISTS np_storage_buckets CASCADE;
