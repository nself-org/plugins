package internal

import (
	"context"
	"encoding/json"
	"fmt"
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
func CreateIndex(ctx context.Context, pool *pgxpool.Pool, req CreateIndexRequest) (*SearchIndex, error) {
	rankingRules := `["words","typo","proximity","attribute","sort","exactness"]`
	if len(req.RankingRules) > 0 {
		b, _ := json.Marshal(req.RankingRules)
		rankingRules = string(b)
	}
	settings := "{}"
	if req.Settings != nil {
		b, _ := json.Marshal(req.Settings)
		settings = string(b)
	}
	engine := "postgres"
	if req.Engine != "" {
		engine = req.Engine
	}
	sourceIDCol := "id"
	if req.SourceIDColumn != "" {
		sourceIDCol = req.SourceIDColumn
	}
	filterableFields := req.FilterableFields
	if filterableFields == nil {
		filterableFields = []string{}
	}
	sortableFields := req.SortableFields
	if sortableFields == nil {
		sortableFields = []string{}
	}

	sourceAccountID := resolveSourceAccountID(ctx)

	var idx SearchIndex
	err := pool.QueryRow(ctx, `
		INSERT INTO np_search_indexes (
			source_account_id, name, description, source_table, source_id_column,
			searchable_fields, filterable_fields, sortable_fields, ranking_rules,
			settings, engine
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb, $11)
		RETURNING id, source_account_id, name, description, source_table, source_id_column,
			searchable_fields, filterable_fields, sortable_fields,
			ranking_rules::text, settings::text, engine, enabled,
			document_count, last_indexed_at, created_at, updated_at`,
		sourceAccountID, req.Name, req.Description, req.SourceTable, sourceIDCol,
		req.SearchableFields, filterableFields, sortableFields, rankingRules,
		settings, engine,
	).Scan(
		&idx.ID, &idx.SourceAccountID, &idx.Name, &idx.Description, &idx.SourceTable,
		&idx.SourceIDColumn, &idx.SearchableFields, &idx.FilterableFields,
		&idx.SortableFields, &idx.RankingRules, &idx.Settings, &idx.Engine,
		&idx.Enabled, &idx.DocumentCount, &idx.LastIndexedAt, &idx.CreatedAt, &idx.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &idx, nil
}

// ListIndexes returns all indexes for the current source account.
func ListIndexes(ctx context.Context, pool *pgxpool.Pool) ([]SearchIndex, error) {
	sourceAccountID := resolveSourceAccountID(ctx)

	rows, err := pool.Query(ctx, `
		SELECT id, source_account_id, name, description, source_table, source_id_column,
			searchable_fields, filterable_fields, sortable_fields,
			ranking_rules::text, settings::text, engine, enabled,
			document_count, last_indexed_at, created_at, updated_at
		FROM np_search_indexes
		WHERE source_account_id = $1
		ORDER BY created_at DESC`, sourceAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchIndex
	for rows.Next() {
		var idx SearchIndex
		if err := rows.Scan(
			&idx.ID, &idx.SourceAccountID, &idx.Name, &idx.Description, &idx.SourceTable,
			&idx.SourceIDColumn, &idx.SearchableFields, &idx.FilterableFields,
			&idx.SortableFields, &idx.RankingRules, &idx.Settings, &idx.Engine,
			&idx.Enabled, &idx.DocumentCount, &idx.LastIndexedAt, &idx.CreatedAt, &idx.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, idx)
	}
	return results, rows.Err()
}

// GetIndex returns a single index by name.
func GetIndex(ctx context.Context, pool *pgxpool.Pool, name string) (*SearchIndex, error) {
	sourceAccountID := resolveSourceAccountID(ctx)

	var idx SearchIndex
	err := pool.QueryRow(ctx, `
		SELECT id, source_account_id, name, description, source_table, source_id_column,
			searchable_fields, filterable_fields, sortable_fields,
			ranking_rules::text, settings::text, engine, enabled,
			document_count, last_indexed_at, created_at, updated_at
		FROM np_search_indexes
		WHERE source_account_id = $1 AND name = $2`, sourceAccountID, name,
	).Scan(
		&idx.ID, &idx.SourceAccountID, &idx.Name, &idx.Description, &idx.SourceTable,
		&idx.SourceIDColumn, &idx.SearchableFields, &idx.FilterableFields,
		&idx.SortableFields, &idx.RankingRules, &idx.Settings, &idx.Engine,
		&idx.Enabled, &idx.DocumentCount, &idx.LastIndexedAt, &idx.CreatedAt, &idx.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &idx, nil
}

// DeleteIndex removes an index and its associated documents.
func DeleteIndex(ctx context.Context, pool *pgxpool.Pool, name string) (bool, error) {
	sourceAccountID := resolveSourceAccountID(ctx)

	// Delete documents first.
	_, err := pool.Exec(ctx,
		`DELETE FROM np_search_documents WHERE source_account_id = $1 AND index_name = $2`,
		sourceAccountID, name)
	if err != nil {
		return false, err
	}

	// Delete the index.
	tag, err := pool.Exec(ctx,
		`DELETE FROM np_search_indexes WHERE source_account_id = $1 AND name = $2`,
		sourceAccountID, name)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// UpdateDocumentCount refreshes the document_count and last_indexed_at on an index.
func UpdateDocumentCount(ctx context.Context, pool *pgxpool.Pool, name string) error {
	sourceAccountID := resolveSourceAccountID(ctx)

	_, err := pool.Exec(ctx, `
		UPDATE np_search_indexes
		SET document_count = (
			SELECT COUNT(*) FROM np_search_documents
			WHERE source_account_id = $1 AND index_name = $2
		),
		last_indexed_at = NOW()
		WHERE source_account_id = $1 AND name = $2`, sourceAccountID, name)
	return err
}

// --- Document CRUD -----------------------------------------------------------

// IndexDocument upserts a document into the search index, building the tsvector
// from the index's searchable fields.
func IndexDocument(ctx context.Context, pool *pgxpool.Pool, indexName string, doc DocumentRequest) (*SearchDocument, error) {
	sourceAccountID := resolveSourceAccountID(ctx)

	// Fetch the index to get searchable fields.
	idx, err := GetIndex(ctx, pool, indexName)
	if err != nil {
		return nil, fmt.Errorf("index not found: %s", indexName)
	}
	if !idx.Enabled {
		return nil, fmt.Errorf("index is disabled: %s", indexName)
	}

	// Build search text from searchable fields.
	searchText := buildSearchText(idx.SearchableFields, doc.Fields)

	contentJSON, err := json.Marshal(doc.Fields)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal content: %w", err)
	}

	var d SearchDocument
	err = pool.QueryRow(ctx, `
		INSERT INTO np_search_documents (
			source_account_id, index_name, source_id, content, search_vector
		) VALUES ($1, $2, $3, $4::jsonb, to_tsvector('english', $5))
		ON CONFLICT (source_account_id, index_name, source_id) DO UPDATE SET
			content = EXCLUDED.content,
			search_vector = EXCLUDED.search_vector,
			indexed_at = NOW()
		RETURNING id, source_account_id, index_name, source_id, content::text, indexed_at`,
		sourceAccountID, indexName, doc.ID, string(contentJSON), searchText,
	).Scan(&d.ID, &d.SourceAccountID, &d.IndexName, &d.SourceID, &d.Content, &d.IndexedAt)
	if err != nil {
		return nil, err
	}

	_ = UpdateDocumentCount(ctx, pool, indexName)
	return &d, nil
}

// DeleteDocument removes a document from a search index.
func DeleteDocument(ctx context.Context, pool *pgxpool.Pool, indexName, sourceID string) (bool, error) {
	sourceAccountID := resolveSourceAccountID(ctx)

	tag, err := pool.Exec(ctx,
		`DELETE FROM np_search_documents WHERE source_account_id = $1 AND index_name = $2 AND source_id = $3`,
		sourceAccountID, indexName, sourceID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() > 0 {
		_ = UpdateDocumentCount(ctx, pool, indexName)
		return true, nil
	}
	return false, nil
}

// --- Helpers -----------------------------------------------------------------

// buildSearchText concatenates the values of the specified fields into a single string
// suitable for to_tsvector.
func buildSearchText(searchableFields []string, fields map[string]interface{}) string {
	var text string
	for _, f := range searchableFields {
		v, ok := fields[f]
		if !ok || v == nil {
			continue
		}
		s := fmt.Sprintf("%v", v)
		if text != "" {
			text += " "
		}
		text += s
	}
	return text
}

// sourceAccountKey is the context key for multi-app isolation.
type sourceAccountKey struct{}

// WithSourceAccount returns a context carrying the given source_account_id.
func WithSourceAccount(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sourceAccountKey{}, id)
}

// resolveSourceAccountID extracts the source_account_id from context or defaults to "primary".
func resolveSourceAccountID(ctx context.Context) string {
	if v, ok := ctx.Value(sourceAccountKey{}).(string); ok && v != "" {
		return v
	}
	return "primary"
}
