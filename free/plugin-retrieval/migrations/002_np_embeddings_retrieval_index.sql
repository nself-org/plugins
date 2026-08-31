-- plugin-retrieval: BGE-M3 embeddings + RRF retrieval index tables (E7)
-- Multi-App Isolation Convention: source_account_id TEXT NOT NULL DEFAULT 'primary'
-- BGE-M3 output dimension: 1024

-- np_embeddings: stores BGE-M3 vector embeddings with source content reference
CREATE TABLE IF NOT EXISTS np_embeddings (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    content_id       TEXT NOT NULL,
    content_type     TEXT NOT NULL,
    embedding        vector(1024),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_embeddings_account
    ON np_embeddings (source_account_id);

-- tsvector index for full-text search on content_type
CREATE INDEX IF NOT EXISTS idx_np_embeddings_content_type_gin
    ON np_embeddings USING GIN (to_tsvector('english', content_type));

-- IVFFlat ANN index for cosine distance search on 1024-dim BGE-M3 vectors
CREATE INDEX IF NOT EXISTS idx_np_embeddings_ivfflat
    ON np_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists=100);

-- np_retrieval_index: RRF metadata + tsvector for hybrid search scoring
CREATE TABLE IF NOT EXISTS np_retrieval_index (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id TEXT NOT NULL DEFAULT 'primary',
    embedding_id     UUID REFERENCES np_embeddings(id) ON DELETE CASCADE,
    rrf_score        FLOAT8,
    tsvector_content tsvector,
    query_hash       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_retrieval_index_account
    ON np_retrieval_index (source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_retrieval_index_embedding_id
    ON np_retrieval_index (embedding_id);

CREATE INDEX IF NOT EXISTS idx_np_retrieval_index_tsvector
    ON np_retrieval_index USING GIN (tsvector_content);

CREATE INDEX IF NOT EXISTS idx_np_retrieval_index_query_hash
    ON np_retrieval_index (query_hash);
