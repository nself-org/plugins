/**
 * Search Plugin Types
 * Complete type definitions for search operations
 */

export type SearchEngine = 'postgres' | 'meilisearch';

export interface SearchPluginConfig {
  port: number;
  host: string;
  engine: SearchEngine;
  meilisearchUrl?: string;
  meilisearchApiKey?: string;
  defaultLimit: number;
  maxResults: number;
  reindexBatchSize: number;
  analyticsEnabled: boolean;
  analyticsRetentionDays: number;
  database: {
    host: string;
    port: number;
    database: string;
    user: string;
    password: string;
    ssl?: boolean;
  };
}

// =============================================================================
// Search Index Types
// =============================================================================

export interface SearchIndexRecord {
  id: string;
  source_account_id: string;
  name: string;
  description: string | null;
  source_table: string | null;
  source_id_column: string;
  searchable_fields: string[];
  filterable_fields: string[];
  sortable_fields: string[];
  ranking_rules: string[];
  settings: Record<string, unknown>;
  engine: SearchEngine;
  enabled: boolean;
  document_count: number;
  last_indexed_at: Date | null;
  created_at: Date;
  updated_at: Date;
}

export interface CreateIndexRequest {
  name: string;
  description?: string;
  source_table?: string;
  source_id_column?: string;
  searchable_fields: string[];
  filterable_fields?: string[];
  sortable_fields?: string[];
  ranking_rules?: string[];
  settings?: Record<string, unknown>;
  engine?: SearchEngine;
}

export interface UpdateIndexRequest {
  description?: string;
  searchable_fields?: string[];
  filterable_fields?: string[];
  sortable_fields?: string[];
  ranking_rules?: string[];
  settings?: Record<string, unknown>;
  enabled?: boolean;
}

// =============================================================================
// Document Types
// =============================================================================

export interface SearchDocumentRecord {
  id: string;
  source_account_id: string;
  index_name: string;
  source_id: string;
  content: Record<string, unknown>;
  search_vector: string | null;
  indexed_at: Date;
}

export interface IndexDocumentRequest {
  id: string;
  [key: string]: unknown;
}

export interface IndexDocumentsRequest {
  documents: IndexDocumentRequest[];
}

// =============================================================================
// Search Types
// =============================================================================

export interface SearchRequest {
  q: string;
  indexes?: string[];
  filter?: Record<string, unknown>;
  facets?: string[];
  sort?: string[];
  limit?: number;
  offset?: number;
  highlight?: boolean;
  attributesToRetrieve?: string[];
}

export interface SearchHit {
  id: string;
  index: string;
  score: number;
  content: Record<string, unknown>;
  highlights?: Record<string, string[]>;
}

export interface SearchFacet {
  field: string;
  values: Array<{
    value: string;
    count: number;
  }>;
}

export interface SearchResponse {
  hits: SearchHit[];
  total: number;
  limit: number;
  offset: number;
  query: string;
  processingTimeMs: number;
  facets?: SearchFacet[];
}

// =============================================================================
// Synonym Types
// =============================================================================

export interface SearchSynonymRecord {
  id: string;
  source_account_id: string;
  index_name: string;
  word: string;
  synonyms: string[];
  created_at: Date;
}

export interface CreateSynonymRequest {
  word: string;
  synonyms: string[];
}

// =============================================================================
// Analytics Types
// =============================================================================

export interface SearchQueryRecord {
  id: string;
  source_account_id: string;
  index_name: string | null;
  query_text: string;
  filters: Record<string, unknown> | null;
  result_count: number;
  took_ms: number;
  user_id: string | null;
  clicked_result_id: string | null;
  created_at: Date;
}

export interface TopQuery {
  query: string;
  count: number;
  avg_results: number;
  avg_time_ms: number;
}

export interface NoResultQuery {
  query: string;
  count: number;
  last_searched: Date;
}

export interface SearchStats {
  total_queries: number;
  unique_queries: number;
  avg_results: number;
  avg_time_ms: number;
  zero_results_rate: number;
  top_queries: TopQuery[];
  no_result_queries: NoResultQuery[];
}

// =============================================================================
// Webhook Types
// =============================================================================

export interface SearchWebhookEventRecord {
  id: string;
  source_account_id: string;
  event_type: string;
  payload: Record<string, unknown>;
  processed: boolean;
  processed_at: Date | null;
  error: string | null;
  created_at: Date;
}

// =============================================================================
// Autocomplete Types
// =============================================================================

export interface SuggestRequest {
  q: string;
  indexes?: string[];
  limit?: number;
  attributesToRetrieve?: string[];
}

export interface Suggestion {
  value: string;
  index: string;
  count?: number;
}

export interface SuggestResponse {
  suggestions: Suggestion[];
  query: string;
  processingTimeMs: number;
}

// =============================================================================
// Reindex Types
// =============================================================================

export interface ReindexOptions {
  batchSize?: number;
  fullReindex?: boolean;
}

export interface ReindexResult {
  indexed: number;
  failed: number;
  duration: number;
  errors: string[];
}

// =============================================================================
// Status Types
// =============================================================================

export interface IndexStatus {
  name: string;
  enabled: boolean;
  document_count: number;
  last_indexed_at: Date | null;
}

export interface SearchStatus {
  plugin: string;
  version: string;
  engine: SearchEngine;
  indexes: IndexStatus[];
  stats?: {
    total_queries: number;
    avg_time_ms: number;
  };
}
