package internal

import (
	"time"
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

// --- Index CRUD --------------------------------------------------------------

// CreateIndex inserts a new search index and returns the created row.
