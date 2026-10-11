-- Migration 002: the search tables and indexes (what internal/db.go Migrate created at start-up before boot-applied
-- migrations). Runs after 001 so that on an install holding the old search_* tables the renamed np_search_* tables
-- are kept (CREATE ... IF NOT EXISTS then adds only what is missing). Idempotent, no transaction control.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" SCHEMA public;
CREATE EXTENSION IF NOT EXISTS "pg_trgm" SCHEMA public;

CREATE TABLE IF NOT EXISTS public.np_search_indexes (
    id UUID PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    name VARCHAR(255) NOT NULL,
    description TEXT,
    source_table VARCHAR(255),
    source_id_column VARCHAR(255) DEFAULT 'id',
    searchable_fields TEXT[] NOT NULL,
    filterable_fields TEXT[] DEFAULT '{}',
    sortable_fields TEXT[] DEFAULT '{}',
    ranking_rules JSONB DEFAULT '["words","typo","proximity","attribute","sort","exactness"]',
    settings JSONB DEFAULT '{}',
    engine VARCHAR(32) DEFAULT 'postgres',
    enabled BOOLEAN DEFAULT true,
    document_count INTEGER DEFAULT 0,
    last_indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_account_id, name)
);
CREATE INDEX IF NOT EXISTS idx_np_search_indexes_account ON public.np_search_indexes(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_search_indexes_name ON public.np_search_indexes(name);
CREATE INDEX IF NOT EXISTS idx_np_search_indexes_enabled ON public.np_search_indexes(enabled);

CREATE TABLE IF NOT EXISTS public.np_search_documents (
    id UUID PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    index_name VARCHAR(255) NOT NULL,
    source_id VARCHAR(255) NOT NULL,
    content JSONB NOT NULL,
    search_vector tsvector,
    indexed_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(source_account_id, index_name, source_id)
);
CREATE INDEX IF NOT EXISTS idx_np_search_documents_account ON public.np_search_documents(source_account_id);
CREATE INDEX IF NOT EXISTS idx_np_search_documents_index ON public.np_search_documents(index_name);
CREATE INDEX IF NOT EXISTS idx_np_search_documents_source_id ON public.np_search_documents(source_id);
CREATE INDEX IF NOT EXISTS idx_np_search_documents_vector ON public.np_search_documents USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS idx_np_search_documents_content ON public.np_search_documents USING GIN(content);
