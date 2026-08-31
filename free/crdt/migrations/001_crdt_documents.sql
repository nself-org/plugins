-- CRDT plugin: document state + update log tables
-- Migration: 001_crdt_documents

CREATE TABLE IF NOT EXISTS np_crdt_documents (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  doc_id            TEXT        NOT NULL UNIQUE,
  engine            TEXT        NOT NULL CHECK (engine IN ('yjs', 'automerge')),
  state             BYTEA       NOT NULL,
  updates_count     INT         NOT NULL DEFAULT 0,
  source_account_id TEXT        NOT NULL DEFAULT 'primary',
  last_modified     TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_crdt_documents_doc_id
  ON np_crdt_documents (doc_id);

CREATE INDEX IF NOT EXISTS idx_np_crdt_documents_account
  ON np_crdt_documents (source_account_id);

CREATE TABLE IF NOT EXISTS np_crdt_updates (
  id                BIGSERIAL   PRIMARY KEY,
  doc_id            TEXT        NOT NULL REFERENCES np_crdt_documents(doc_id) ON DELETE CASCADE,
  update_data       BYTEA       NOT NULL,
  client_id         TEXT,
  source_account_id TEXT        NOT NULL DEFAULT 'primary',
  applied_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_np_crdt_updates_account
  ON np_crdt_updates (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_crdt_updates_doc_applied
  ON np_crdt_updates (doc_id, applied_at DESC);
