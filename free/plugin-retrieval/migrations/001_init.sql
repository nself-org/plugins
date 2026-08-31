-- plugin-retrieval: hybrid retrieval tables
-- Multi-App Isolation Convention: source_account_id TEXT NOT NULL DEFAULT 'primary'

-- Enable pgvector extension (idempotent)
CREATE EXTENSION IF NOT EXISTS vector;

-- np_retrieval_documents: text corpus for tsvector BM25 search
CREATE TABLE IF NOT EXISTS np_retrieval_documents (
    id               TEXT NOT NULL,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    title            TEXT NOT NULL DEFAULT '',
    content          TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);

CREATE INDEX IF NOT EXISTS idx_np_retrieval_documents_account
    ON np_retrieval_documents (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_retrieval_documents_fts
    ON np_retrieval_documents USING GIN (to_tsvector('english', content));

-- np_retrieval_embeddings: pgvector ANN index (1536 dims for OpenAI ada-002 compat)
CREATE TABLE IF NOT EXISTS np_retrieval_embeddings (
    document_id      TEXT NOT NULL,
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    embedding        vector(1536) NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (document_id, source_account_id),
    FOREIGN KEY (document_id, source_account_id)
        REFERENCES np_retrieval_documents (id, source_account_id)
        ON DELETE CASCADE
);

-- IVFFlat index for ANN search (lists=100 for up to 1M vectors per account)
CREATE INDEX IF NOT EXISTS idx_np_retrieval_embeddings_ivfflat
    ON np_retrieval_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists=100);
