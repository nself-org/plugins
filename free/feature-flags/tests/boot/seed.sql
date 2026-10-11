-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Only tables the basis build creates: np_feature_flags_audit is absent from the basis (see tests/boot/basis-absent).
-- Test data only.
INSERT INTO np_feature_flags_flags (id, key, name) VALUES ('00000000-0000-4000-8000-0000000000e1', 'seed-flag', 'Seed flag');
INSERT INTO np_feature_flags_segments (id, name) VALUES ('00000000-0000-4000-8000-0000000000e2', 'seed-segment');
