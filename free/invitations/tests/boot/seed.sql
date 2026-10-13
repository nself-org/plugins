-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade.
-- Test data only. The source_account_id column does not exist in the basis schema; the upgrade adds it.
INSERT INTO np_invitations_invitations (id, email, token, invited_by) VALUES ('seed-invitation', 'seed@example.invalid', 'seed-token-not-real', 'seed-user');
