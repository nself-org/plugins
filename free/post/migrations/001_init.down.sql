-- Down migration for 001_init.sql — nself-post initial schema
-- Drop in reverse dependency order (queue FK depends on accounts)
DROP TABLE IF EXISTS np_post_queue;
DROP TABLE IF EXISTS np_post_accounts;
