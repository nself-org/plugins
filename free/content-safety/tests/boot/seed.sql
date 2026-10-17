-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written before the upgrade, fixed ids so row survival is
-- checked by primary key. Test data only.
INSERT INTO np_cs_evidence (id, type, reason, source, workspace_id) VALUES ('00000000-0000-4000-8000-000000000001', 'message', 'seed reason', 'seed', 'seed-ws');
INSERT INTO np_cs_legal_holds (id, name) VALUES ('00000000-0000-4000-8000-000000000002', 'seed hold');
INSERT INTO np_cs_evidence_exports (id) VALUES ('00000000-0000-4000-8000-000000000003');
INSERT INTO np_cs_spam_rules (id, name, rule_type) VALUES ('00000000-0000-4000-8000-000000000004', 'seed rule', 'keyword');
INSERT INTO np_cs_spam_configs (workspace_id) VALUES ('seed-ws');
INSERT INTO np_cs_rate_limit_violations (id, user_id, limit_type) VALUES ('00000000-0000-4000-8000-000000000006', 'seed-user', 'message');
INSERT INTO np_cs_raid_events (id, workspace_id) VALUES ('00000000-0000-4000-8000-000000000007', 'seed-ws');
INSERT INTO np_cs_lockdowns (id, workspace_id) VALUES ('00000000-0000-4000-8000-000000000008', 'seed-ws');
INSERT INTO np_cs_abuse_scores (user_id) VALUES ('seed-user');
