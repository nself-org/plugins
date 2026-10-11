-- Boot-kit legacy fixture (tests/boot/tables-upgrade.sh LEGACY pass): the TS-era search schema (unprefixed search_* tables)
-- an old install holds, one row each with fixed primary keys. Test data only. Ends with the rows the pass must keep.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- =====================================================================
-- Search Indexes
-- =====================================================================

CREATE TABLE IF NOT EXISTS search_indexes (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
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
  last_indexed_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);
CREATE INDEX IF NOT EXISTS idx_search_indexes_account ON search_indexes(source_account_id);
CREATE INDEX IF NOT EXISTS idx_search_indexes_name ON search_indexes(name);
CREATE INDEX IF NOT EXISTS idx_search_indexes_enabled ON search_indexes(enabled);

-- =====================================================================
-- Search Documents
-- =====================================================================

CREATE TABLE IF NOT EXISTS search_documents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  index_name VARCHAR(255) NOT NULL,
  source_id VARCHAR(255) NOT NULL,
  content JSONB NOT NULL,
  search_vector tsvector,
  indexed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, index_name, source_id)
);
CREATE INDEX IF NOT EXISTS idx_search_documents_account ON search_documents(source_account_id);
CREATE INDEX IF NOT EXISTS idx_search_documents_index ON search_documents(index_name);
CREATE INDEX IF NOT EXISTS idx_search_documents_source_id ON search_documents(source_id);
CREATE INDEX IF NOT EXISTS idx_search_documents_vector ON search_documents USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS idx_search_documents_content ON search_documents USING GIN(content);

-- =====================================================================
-- Search Synonyms
-- =====================================================================

CREATE TABLE IF NOT EXISTS search_synonyms (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  index_name VARCHAR(255) NOT NULL,
  word VARCHAR(255) NOT NULL,
  synonyms TEXT[] NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, index_name, word)
);
CREATE INDEX IF NOT EXISTS idx_search_synonyms_account ON search_synonyms(source_account_id);
CREATE INDEX IF NOT EXISTS idx_search_synonyms_index ON search_synonyms(index_name);
CREATE INDEX IF NOT EXISTS idx_search_synonyms_word ON search_synonyms(word);

-- =====================================================================
-- Search Query Analytics
-- =====================================================================

CREATE TABLE IF NOT EXISTS search_queries (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  index_name VARCHAR(255),
  query_text TEXT NOT NULL,
  filters JSONB,
  result_count INTEGER DEFAULT 0,
  took_ms INTEGER,
  user_id VARCHAR(255),
  clicked_result_id VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_search_queries_account ON search_queries(source_account_id);
CREATE INDEX IF NOT EXISTS idx_search_queries_index ON search_queries(index_name);
CREATE INDEX IF NOT EXISTS idx_search_queries_text ON search_queries USING GIN(to_tsvector('english', query_text));
CREATE INDEX IF NOT EXISTS idx_search_queries_created ON search_queries(created_at);

-- =====================================================================
-- Webhook Events
-- =====================================================================

CREATE TABLE IF NOT EXISTS search_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128),
  payload JSONB,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_search_webhook_events_account ON search_webhook_events(source_account_id);
CREATE INDEX IF NOT EXISTS idx_search_webhook_events_type ON search_webhook_events(event_type);
CREATE INDEX IF NOT EXISTS idx_search_webhook_events_processed ON search_webhook_events(processed);
CREATE INDEX IF NOT EXISTS idx_search_webhook_events_created ON search_webhook_events(created_at);

INSERT INTO search_indexes (id, name, searchable_fields) VALUES ('00000000-0000-4000-8000-0000000000a1', 'legacy-index', '{title,body}');
INSERT INTO search_documents (id, index_name, source_id, content) VALUES ('00000000-0000-4000-8000-0000000000a2', 'legacy-index', 'doc-1', '{"title":"legacy"}');
INSERT INTO search_synonyms (id, index_name, word, synonyms) VALUES ('00000000-0000-4000-8000-0000000000a3', 'legacy-index', 'car', '{auto,vehicle}');
INSERT INTO search_queries (id, index_name, query_text) VALUES ('00000000-0000-4000-8000-0000000000a4', 'legacy-index', 'legacy query');
INSERT INTO search_webhook_events (id, event_type) VALUES ('legacy-search-event', 'index.created');
