-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Parent rows first. Test data only.
INSERT INTO np_storage_buckets (id, name) VALUES ('00000000-0000-4000-8000-000000000001', 'seed-bucket');
INSERT INTO np_storage_objects (id, bucket_id, key) VALUES ('00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-000000000001', 'seed/object.txt');
INSERT INTO np_storage_metadata (object_id, key, value) VALUES ('00000000-0000-4000-8000-000000000002', 'seed-key', 'seed-value');
