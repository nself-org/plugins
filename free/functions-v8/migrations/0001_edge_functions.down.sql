-- 0001_edge_functions.down.sql
-- Reverse of 0001_edge_functions.sql

-- Drop unique index explicitly first (index on filtered expression — dropped with table,
-- but listed here for clarity about rollback ordering).
-- Then drop tables in FK-safe order: child tables before parent.

DROP TABLE IF EXISTS np_function_logs;
DROP TABLE IF EXISTS np_edge_function_env;
DROP TABLE IF EXISTS np_edge_function_versions;
DROP TABLE IF EXISTS np_edge_functions;
