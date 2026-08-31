-- 001_soc2_tables.down.sql
-- Reverse of 001_soc2_tables.sql

-- Drop in declaration order (no FK references between these three tables).
DROP TABLE IF EXISTS np_change_log;
DROP TABLE IF EXISTS np_access_reviews;
DROP TABLE IF EXISTS np_vendor_registry;
