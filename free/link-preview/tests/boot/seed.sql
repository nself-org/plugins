-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only.
INSERT INTO np_link_preview_cache (id, url, title) VALUES ('seed-preview', 'https://example.invalid/seed', 'Seed');
