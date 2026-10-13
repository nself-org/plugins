-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only, fixed ids so row survival is checked by primary key.
INSERT INTO np_phi_columns (id, table_name, column_name, phi_category) VALUES ('00000000-0000-4000-8000-000000000001', 'seed_patients', 'seed_name', 'name');
INSERT INTO np_phi_audit_log (id, table_name, column_names, row_count, purpose) VALUES ('00000000-0000-4000-8000-000000000002', 'seed_patients', ARRAY['seed_name'], 1, 'treatment');
INSERT INTO np_baa_records (id, signed_by) VALUES ('00000000-0000-4000-8000-000000000003', 'seed signer');
