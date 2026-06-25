package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Size-cap exception: single DB operation — 55L scan loop with struct mapping; splitting would fragment a single SQL query across files.
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
