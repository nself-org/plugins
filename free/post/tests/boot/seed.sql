-- Boot-kit seed (tests/boot/tables-upgrade.sh): rows written through the basis schema before the upgrade. Parent rows first. Test data only.
INSERT INTO np_post_accounts (id, source_account_id, platform, name) VALUES ('00000000-0000-0000-0000-0000000000a1', 'primary', 'twitter', 'seed-account');
INSERT INTO np_post_queue (id, source_account_id, platform, account_id, title, content) VALUES ('00000000-0000-0000-0000-0000000000b1', 'primary', 'twitter', '00000000-0000-0000-0000-0000000000a1', 'seed title', 'seed content');
