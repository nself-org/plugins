-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- The basis tables have no source_account_id column yet; the upgrade adds it. Test data only.
INSERT INTO np_notifications_templates (id, name, channel) VALUES ('seed-template', 'seed-template-name', 'email');
INSERT INTO np_notifications_notifications (id, channel, recipient, template) VALUES ('seed-notification', 'email', 'seed@example.invalid', 'seed-template-name');
INSERT INTO np_notifications_preferences (user_id) VALUES ('seed-user');
