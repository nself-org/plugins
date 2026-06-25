package internal

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Models ------------------------------------------------------------------

// SearchIndex represents a row in np_search_indexes.
type SearchIndex struct {
	ID               string     `json:"id"`
	SourceAccountID  string     `json:"source_account_id"`
	Name             string     `json:"name"`
	Description      *string    `json:"description"`
	SourceTable      *string    `json:"source_table"`
	SourceIDColumn   string     `json:"source_id_column"`
	SearchableFields []string   `json:"searchable_fields"`
	FilterableFields []string   `json:"filterable_fields"`
	SortableFields   []string   `json:"sortable_fields"`
	RankingRules     string     `json:"ranking_rules"`
	Settings         string     `json:"settings"`
	Engine           string     `json:"engine"`
	Enabled          bool       `json:"enabled"`
	DocumentCount    int        `json:"document_count"`
	LastIndexedAt    *time.Time `json:"last_indexed_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// SearchDocument represents a row in np_search_documents.
type SearchDocument struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	IndexName       string    `json:"index_name"`
	SourceID        string    `json:"source_id"`
	Content         string    `json:"content"`
	IndexedAt       time.Time `json:"indexed_at"`
}

// --- Migration ---------------------------------------------------------------

// Migrate creates the required tables and indexes if they do not exist.
func Migrate(pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
		CREATE EXTENSION IF NOT EXISTS "pg_trgm";

		CREATE TABLE IF NOT EXISTS np_search_indexes (
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
			last_indexed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(source_account_id, name)
		);
		CREATE INDEX IF NOT EXISTS idx_np_search_indexes_account ON np_search_indexes(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_search_indexes_name ON np_search_indexes(name);
		CREATE INDEX IF NOT EXISTS idx_np_search_indexes_enabled ON np_search_indexes(enabled);

		CREATE TABLE IF NOT EXISTS np_search_documents (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			source_account_id VARCHAR(128) DEFAULT 'primary',
			index_name VARCHAR(255) NOT NULL,
			source_id VARCHAR(255) NOT NULL,
			content JSONB NOT NULL,
			search_vector tsvector,
			indexed_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(source_account_id, index_name, source_id)
		);
		CREATE INDEX IF NOT EXISTS idx_np_search_documents_account ON np_search_documents(source_account_id);
		CREATE INDEX IF NOT EXISTS idx_np_search_documents_index ON np_search_documents(index_name);
		CREATE INDEX IF NOT EXISTS idx_np_search_documents_source_id ON np_search_documents(source_id);
		CREATE INDEX IF NOT EXISTS idx_np_search_documents_vector ON np_search_documents USING GIN(search_vector);
		CREATE INDEX IF NOT EXISTS idx_np_search_documents_content ON np_search_documents USING GIN(content);
	`)
	return err
}

// --- Index CRUD --------------------------------------------------------------

// CreateIndex inserts a new search index and returns the created row.
