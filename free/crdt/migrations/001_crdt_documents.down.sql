-- 001_crdt_documents.down.sql
-- Reverse of 001_crdt_documents.sql

-- Drop in FK-safe order: child table first, then parent.
DROP TABLE IF EXISTS np_crdt_updates;
DROP TABLE IF EXISTS np_crdt_documents;
