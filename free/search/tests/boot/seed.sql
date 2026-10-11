-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Parent rows first. Test data only.
INSERT INTO np_search_indexes (id, name, searchable_fields) VALUES ('00000000-0000-4000-8000-0000000000d1', 'seed-index', '{title}');
INSERT INTO np_search_documents (id, index_name, source_id, content) VALUES ('00000000-0000-4000-8000-0000000000d2', 'seed-index', 'seed-doc', '{"title":"seed"}');
