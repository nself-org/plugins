/**
 * Search Database Operations
 * Complete CRUD operations for search indexes, documents, and analytics
 */

import { createDatabase, createLogger, type Database } from '@nself/plugin-utils';
import type {
  SearchIndexRecord,
  SearchDocumentRecord,
  SearchSynonymRecord,
  CreateIndexRequest,
  UpdateIndexRequest,
  IndexDocumentRequest,
  SearchRequest,
  SearchHit,
  SearchResponse,
  SearchFacet,
  TopQuery,
  NoResultQuery,
  SearchStats,
  Suggestion,
  SuggestResponse,
  ReindexResult,
  ReindexOptions,
} from './types.js';

const logger = createLogger('search:db');

export class SearchDatabase {
  private db: Database;
  private readonly sourceAccountId: string;

  constructor(db?: Database, sourceAccountId = 'primary') {
    this.db = db ?? createDatabase();
    this.sourceAccountId = this.normalizeSourceAccountId(sourceAccountId);
  }

  forSourceAccount(sourceAccountId: string): SearchDatabase {
    return new SearchDatabase(this.db, sourceAccountId);
  }

  getCurrentSourceAccountId(): string {
    return this.sourceAccountId;
  }

  private normalizeSourceAccountId(value: string): string {
    const normalized = value
      .toLowerCase()
      .replace(/[^a-z0-9_-]+/g, '-')
      .replace(/^-+|-+$/g, '');

    return normalized.length > 0 ? normalized : 'primary';
  }

  async connect(): Promise<void> {
    await this.db.connect();
  }

  async disconnect(): Promise<void> {
    await this.db.disconnect();
  }

  async query<T extends Record<string, unknown>>(sql: string, params?: unknown[]): Promise<{ rows: T[]; rowCount: number | null }> {
    return this.db.query<T>(sql, params);
  }

  async execute(sql: string, params?: unknown[]): Promise<number> {
    return this.db.execute(sql, params);
  }

  // =========================================================================
  // Schema Management
  // =========================================================================

  async initializeSchema(): Promise<void> {
    logger.info('Initializing search schema...');

    const schema = `
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
    `;

    await this.db.execute(schema);
    logger.info('Search schema initialized successfully');
  }

  // =========================================================================
  // Index Management
  // =========================================================================

  async createIndex(request: CreateIndexRequest): Promise<SearchIndexRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO search_indexes (
        source_account_id, name, description, source_table, source_id_column,
        searchable_fields, filterable_fields, sortable_fields, ranking_rules,
        settings, engine
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
      RETURNING *`,
      [
        this.sourceAccountId,
        request.name,
        request.description ?? null,
        request.source_table ?? null,
        request.source_id_column ?? 'id',
        request.searchable_fields,
        request.filterable_fields ?? [],
        request.sortable_fields ?? [],
        JSON.stringify(request.ranking_rules ?? ['words', 'typo', 'proximity', 'attribute', 'sort', 'exactness']),
        JSON.stringify(request.settings ?? {}),
        request.engine ?? 'postgres',
      ]
    );

    return result.rows[0] as unknown as SearchIndexRecord;
  }

  async getIndex(name: string): Promise<SearchIndexRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM search_indexes WHERE source_account_id = $1 AND name = $2',
      [this.sourceAccountId, name]
    );

    return (result.rows[0] ?? null) as unknown as SearchIndexRecord | null;
  }

  async listIndexes(): Promise<SearchIndexRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM search_indexes WHERE source_account_id = $1 ORDER BY created_at DESC',
      [this.sourceAccountId]
    );

    return result.rows as unknown as SearchIndexRecord[];
  }

  async updateIndex(name: string, updates: UpdateIndexRequest): Promise<SearchIndexRecord | null> {
    const sets: string[] = [];
    const params: unknown[] = [this.sourceAccountId, name];
    let paramIndex = 3;

    if (updates.description !== undefined) {
      sets.push(`description = $${paramIndex++}`);
      params.push(updates.description);
    }
    if (updates.searchable_fields !== undefined) {
      sets.push(`searchable_fields = $${paramIndex++}`);
      params.push(updates.searchable_fields);
    }
    if (updates.filterable_fields !== undefined) {
      sets.push(`filterable_fields = $${paramIndex++}`);
      params.push(updates.filterable_fields);
    }
    if (updates.sortable_fields !== undefined) {
      sets.push(`sortable_fields = $${paramIndex++}`);
      params.push(updates.sortable_fields);
    }
    if (updates.ranking_rules !== undefined) {
      sets.push(`ranking_rules = $${paramIndex++}`);
      params.push(JSON.stringify(updates.ranking_rules));
    }
    if (updates.settings !== undefined) {
      sets.push(`settings = $${paramIndex++}`);
      params.push(JSON.stringify(updates.settings));
    }
    if (updates.enabled !== undefined) {
      sets.push(`enabled = $${paramIndex++}`);
      params.push(updates.enabled);
    }

    if (sets.length === 0) {
      return this.getIndex(name);
    }

    sets.push(`updated_at = NOW()`);

    const result = await this.query<Record<string, unknown>>(
      `UPDATE search_indexes SET ${sets.join(', ')}
       WHERE source_account_id = $1 AND name = $2
       RETURNING *`,
      params
    );

    return (result.rows[0] ?? null) as unknown as SearchIndexRecord | null;
  }

  async deleteIndex(name: string): Promise<boolean> {
    // Delete documents first
    await this.execute(
      'DELETE FROM search_documents WHERE source_account_id = $1 AND index_name = $2',
      [this.sourceAccountId, name]
    );

    // Delete synonyms
    await this.execute(
      'DELETE FROM search_synonyms WHERE source_account_id = $1 AND index_name = $2',
      [this.sourceAccountId, name]
    );

    // Delete index
    const count = await this.execute(
      'DELETE FROM search_indexes WHERE source_account_id = $1 AND name = $2',
      [this.sourceAccountId, name]
    );

    return count > 0;
  }

  async updateIndexDocumentCount(name: string): Promise<void> {
    await this.execute(
      `UPDATE search_indexes
       SET document_count = (
         SELECT COUNT(*) FROM search_documents
         WHERE source_account_id = $1 AND index_name = $2
       ),
       last_indexed_at = NOW()
       WHERE source_account_id = $1 AND name = $2`,
      [this.sourceAccountId, name]
    );
  }

  // =========================================================================
  // Document Management
  // =========================================================================

  private buildSearchVector(index: SearchIndexRecord, content: Record<string, unknown>): string {
    const parts: string[] = [];

    for (const field of index.searchable_fields) {
      const value = content[field];
      if (value !== null && value !== undefined) {
        parts.push(String(value));
      }
    }

    return parts.join(' ');
  }

  async indexDocument(indexName: string, document: IndexDocumentRequest): Promise<SearchDocumentRecord> {
    const index = await this.getIndex(indexName);
    if (!index) {
      throw new Error(`Index not found: ${indexName}`);
    }

    if (!index.enabled) {
      throw new Error(`Index is disabled: ${indexName}`);
    }

    const { id, ...content } = document;
    const searchText = this.buildSearchVector(index, content);

    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO search_documents (
        source_account_id, index_name, source_id, content, search_vector
      ) VALUES ($1, $2, $3, $4, to_tsvector('english', $5))
      ON CONFLICT (source_account_id, index_name, source_id) DO UPDATE SET
        content = EXCLUDED.content,
        search_vector = EXCLUDED.search_vector,
        indexed_at = NOW()
      RETURNING *`,
      [this.sourceAccountId, indexName, id, JSON.stringify(content), searchText]
    );

    await this.updateIndexDocumentCount(indexName);

    return result.rows[0] as unknown as SearchDocumentRecord;
  }

  async indexDocuments(indexName: string, documents: IndexDocumentRequest[]): Promise<number> {
    let indexed = 0;

    for (const doc of documents) {
      try {
        await this.indexDocument(indexName, doc);
        indexed++;
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unknown error';
        logger.error('Failed to index document', { indexName, docId: doc.id, error: message });
      }
    }

    return indexed;
  }

  async getDocument(indexName: string, sourceId: string): Promise<SearchDocumentRecord | null> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM search_documents WHERE source_account_id = $1 AND index_name = $2 AND source_id = $3',
      [this.sourceAccountId, indexName, sourceId]
    );

    return (result.rows[0] ?? null) as unknown as SearchDocumentRecord | null;
  }

  async deleteDocument(indexName: string, sourceId: string): Promise<boolean> {
    const count = await this.execute(
      'DELETE FROM search_documents WHERE source_account_id = $1 AND index_name = $2 AND source_id = $3',
      [this.sourceAccountId, indexName, sourceId]
    );

    if (count > 0) {
      await this.updateIndexDocumentCount(indexName);
    }

    return count > 0;
  }

  // =========================================================================
  // Search Operations (PostgreSQL FTS)
  // =========================================================================

  async search(request: SearchRequest): Promise<SearchResponse> {
    const startTime = Date.now();
    const limit = Math.min(request.limit ?? 20, 1000);
    const offset = request.offset ?? 0;

    // Build query
    const tsquery = this.buildTsQuery(request.q);
    const indexes = request.indexes ?? (await this.listIndexes()).map(idx => idx.name);

    if (indexes.length === 0) {
      return {
        hits: [],
        total: 0,
        limit,
        offset,
        query: request.q,
        processingTimeMs: Date.now() - startTime,
      };
    }

    // Build filter conditions
    const filterConditions: string[] = [];
    const params: unknown[] = [this.sourceAccountId];
    let paramIndex = 2;

    // Add index filter
    filterConditions.push(`index_name = ANY($${paramIndex++})`);
    params.push(indexes);

    // Add user filters (JSON-based)
    if (request.filter) {
      for (const [key, value] of Object.entries(request.filter)) {
        filterConditions.push(`content->>'${key}' = $${paramIndex++}`);
        params.push(String(value));
      }
    }

    const whereClause = filterConditions.length > 0
      ? `WHERE source_account_id = $1 AND ${filterConditions.join(' AND ')}`
      : 'WHERE source_account_id = $1';

    // Execute search
    const searchSql = `
      SELECT
        id,
        index_name,
        source_id,
        content,
        ts_rank_cd(search_vector, websearch_to_tsquery('english', $${paramIndex})) as score
        ${request.highlight ? `, ts_headline('english', content::text, websearch_to_tsquery('english', $${paramIndex}), 'MaxWords=50, MinWords=20') as headline` : ''}
      FROM search_documents
      ${whereClause}
        AND search_vector @@ websearch_to_tsquery('english', $${paramIndex})
      ORDER BY score DESC
      LIMIT $${paramIndex + 1} OFFSET $${paramIndex + 2}
    `;

    params.push(tsquery, limit, offset);

    const result = await this.query<{
      id: string;
      index_name: string;
      source_id: string;
      content: Record<string, unknown>;
      score: number;
      headline?: string;
    }>(searchSql, params);

    // Count total
    const countSql = `
      SELECT COUNT(*) as total
      FROM search_documents
      ${whereClause}
        AND search_vector @@ websearch_to_tsquery('english', $${paramIndex})
    `;
    const countParams = params.slice(0, -2); // Remove limit and offset
    const countResult = await this.query<{ total: string }>(countSql, countParams);
    const total = parseInt(countResult.rows[0]?.total ?? '0', 10);

    // Build hits
    const hits: SearchHit[] = result.rows.map(row => ({
      id: row.source_id,
      index: row.index_name,
      score: row.score,
      content: row.content,
      highlights: request.highlight && row.headline ? { _highlight: [row.headline] } : undefined,
    }));

    // Build facets if requested
    let facets: SearchFacet[] | undefined;
    if (request.facets && request.facets.length > 0) {
      facets = await this.buildFacets(whereClause, params.slice(0, -3), request.facets, tsquery);
    }

    const processingTimeMs = Date.now() - startTime;

    return {
      hits,
      total,
      limit,
      offset,
      query: request.q,
      processingTimeMs,
      facets,
    };
  }

  private buildTsQuery(query: string): string {
    // Clean and prepare query for websearch_to_tsquery
    return query.trim();
  }

  private async buildFacets(
    whereClause: string,
    baseParams: unknown[],
    facetFields: string[],
    tsquery: string
  ): Promise<SearchFacet[]> {
    const facets: SearchFacet[] = [];
    const paramIndex = baseParams.length + 1;

    for (const field of facetFields) {
      const facetSql = `
        SELECT
          content->>'${field}' as value,
          COUNT(*) as count
        FROM search_documents
        ${whereClause}
          AND search_vector @@ websearch_to_tsquery('english', $${paramIndex})
          AND content->>'${field}' IS NOT NULL
        GROUP BY content->>'${field}'
        ORDER BY count DESC
        LIMIT 20
      `;

      const result = await this.query<{ value: string; count: string }>(
        facetSql,
        [...baseParams, tsquery]
      );

      facets.push({
        field,
        values: result.rows.map(row => ({
          value: row.value,
          count: parseInt(row.count, 10),
        })),
      });
    }

    return facets;
  }

  async suggest(query: string, indexes?: string[], limit = 10): Promise<SuggestResponse> {
    const startTime = Date.now();
    const indexNames = indexes ?? (await this.listIndexes()).map(idx => idx.name);

    if (indexNames.length === 0) {
      return {
        suggestions: [],
        query,
        processingTimeMs: Date.now() - startTime,
      };
    }

    // Use trigram similarity for autocomplete
    const sql = `
      SELECT DISTINCT
        word,
        index_name,
        similarity(word, $3) as score
      FROM (
        SELECT
          unnest(string_to_array(regexp_replace(content::text, '[^a-zA-Z0-9 ]', ' ', 'g'), ' ')) as word,
          index_name
        FROM search_documents
        WHERE source_account_id = $1 AND index_name = ANY($2)
      ) words
      WHERE word ILIKE $3 || '%' AND length(word) > 2
      ORDER BY score DESC, word
      LIMIT $4
    `;

    const result = await this.query<{ word: string; index_name: string; score: number }>(
      sql,
      [this.sourceAccountId, indexNames, query, limit]
    );

    const suggestions: Suggestion[] = result.rows.map(row => ({
      value: row.word,
      index: row.index_name,
    }));

    return {
      suggestions,
      query,
      processingTimeMs: Date.now() - startTime,
    };
  }

  // =========================================================================
  // Reindex Operations
  // =========================================================================

  async reindexFromSource(indexName: string, options: ReindexOptions = {}): Promise<ReindexResult> {
    const startTime = Date.now();
    const index = await this.getIndex(indexName);

    if (!index) {
      throw new Error(`Index not found: ${indexName}`);
    }

    if (!index.source_table) {
      throw new Error(`Index has no source table: ${indexName}`);
    }

    const batchSize = options.batchSize ?? 500;
    let indexed = 0;
    let failed = 0;
    const errors: string[] = [];

    try {
      // Clear existing documents if full reindex
      if (options.fullReindex) {
        await this.execute(
          'DELETE FROM search_documents WHERE source_account_id = $1 AND index_name = $2',
          [this.sourceAccountId, indexName]
        );
      }

      // Fetch documents from source table
      const sourceIdCol = index.source_id_column;
      const fields = [sourceIdCol, ...index.searchable_fields, ...index.filterable_fields];
      const uniqueFields = Array.from(new Set(fields));

      const sql = `SELECT ${uniqueFields.join(', ')} FROM ${index.source_table}`;
      const result = await this.query<Record<string, unknown>>(sql);

      // Index in batches
      const documents = result.rows;
      for (let i = 0; i < documents.length; i += batchSize) {
        const batch = documents.slice(i, i + batchSize);

        for (const doc of batch) {
          try {
            const docToIndex: IndexDocumentRequest = {
              id: String(doc[sourceIdCol]),
              ...doc,
            };
            await this.indexDocument(indexName, docToIndex);
            indexed++;
          } catch (error) {
            failed++;
            const message = error instanceof Error ? error.message : 'Unknown error';
            errors.push(`Failed to index document ${doc[sourceIdCol]}: ${message}`);
            logger.error('Failed to index document', { doc, error: message });
          }
        }

        logger.info(`Indexed ${Math.min(i + batchSize, documents.length)}/${documents.length} documents`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      errors.push(`Reindex failed: ${message}`);
      logger.error('Reindex failed', { error: message });
    }

    const duration = Date.now() - startTime;

    return {
      indexed,
      failed,
      duration,
      errors,
    };
  }

  // =========================================================================
  // Synonym Management
  // =========================================================================

  async addSynonym(indexName: string, word: string, synonyms: string[]): Promise<SearchSynonymRecord> {
    const result = await this.query<Record<string, unknown>>(
      `INSERT INTO search_synonyms (source_account_id, index_name, word, synonyms)
       VALUES ($1, $2, $3, $4)
       ON CONFLICT (source_account_id, index_name, word) DO UPDATE SET
         synonyms = EXCLUDED.synonyms
       RETURNING *`,
      [this.sourceAccountId, indexName, word, synonyms]
    );

    return result.rows[0] as unknown as SearchSynonymRecord;
  }

  async getSynonyms(indexName: string): Promise<SearchSynonymRecord[]> {
    const result = await this.query<Record<string, unknown>>(
      'SELECT * FROM search_synonyms WHERE source_account_id = $1 AND index_name = $2 ORDER BY word',
      [this.sourceAccountId, indexName]
    );

    return result.rows as unknown as SearchSynonymRecord[];
  }

  async deleteSynonym(indexName: string, id: string): Promise<boolean> {
    const count = await this.execute(
      'DELETE FROM search_synonyms WHERE source_account_id = $1 AND index_name = $2 AND id = $3',
      [this.sourceAccountId, indexName, id]
    );

    return count > 0;
  }

  // =========================================================================
  // Analytics
  // =========================================================================

  async recordQuery(
    indexName: string | null,
    query: string,
    resultCount: number,
    tookMs: number,
    filters?: Record<string, unknown>,
    userId?: string
  ): Promise<void> {
    await this.execute(
      `INSERT INTO search_queries (
        source_account_id, index_name, query_text, filters, result_count, took_ms, user_id
      ) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
      [
        this.sourceAccountId,
        indexName,
        query,
        filters ? JSON.stringify(filters) : null,
        resultCount,
        tookMs,
        userId ?? null,
      ]
    );
  }

  async getTopQueries(limit = 20, days = 30): Promise<TopQuery[]> {
    const result = await this.query<{
      query: string;
      count: string;
      avg_results: string;
      avg_time_ms: string;
    }>(
      `SELECT
        query_text as query,
        COUNT(*) as count,
        AVG(result_count) as avg_results,
        AVG(took_ms) as avg_time_ms
      FROM search_queries
      WHERE source_account_id = $1
        AND created_at > NOW() - INTERVAL '${days} days'
      GROUP BY query_text
      ORDER BY count DESC
      LIMIT $2`,
      [this.sourceAccountId, limit]
    );

    return result.rows.map(row => ({
      query: row.query,
      count: parseInt(row.count, 10),
      avg_results: parseFloat(row.avg_results),
      avg_time_ms: parseFloat(row.avg_time_ms),
    }));
  }

  async getNoResultQueries(limit = 20, days = 30): Promise<NoResultQuery[]> {
    const result = await this.query<{
      query: string;
      count: string;
      last_searched: Date;
    }>(
      `SELECT
        query_text as query,
        COUNT(*) as count,
        MAX(created_at) as last_searched
      FROM search_queries
      WHERE source_account_id = $1
        AND result_count = 0
        AND created_at > NOW() - INTERVAL '${days} days'
      GROUP BY query_text
      ORDER BY count DESC
      LIMIT $2`,
      [this.sourceAccountId, limit]
    );

    return result.rows.map(row => ({
      query: row.query,
      count: parseInt(row.count, 10),
      last_searched: row.last_searched,
    }));
  }

  async getSearchStats(days = 30): Promise<SearchStats> {
    const statsResult = await this.query<{
      total_queries: string;
      unique_queries: string;
      avg_results: string;
      avg_time_ms: string;
      zero_results_rate: string;
    }>(
      `SELECT
        COUNT(*) as total_queries,
        COUNT(DISTINCT query_text) as unique_queries,
        AVG(result_count) as avg_results,
        AVG(took_ms) as avg_time_ms,
        (SUM(CASE WHEN result_count = 0 THEN 1 ELSE 0 END)::float / COUNT(*)) as zero_results_rate
      FROM search_queries
      WHERE source_account_id = $1
        AND created_at > NOW() - INTERVAL '${days} days'`,
      [this.sourceAccountId]
    );

    const stats = statsResult.rows[0];
    const topQueries = await this.getTopQueries(10, days);
    const noResultQueries = await this.getNoResultQueries(10, days);

    return {
      total_queries: parseInt(stats?.total_queries ?? '0', 10),
      unique_queries: parseInt(stats?.unique_queries ?? '0', 10),
      avg_results: parseFloat(stats?.avg_results ?? '0'),
      avg_time_ms: parseFloat(stats?.avg_time_ms ?? '0'),
      zero_results_rate: parseFloat(stats?.zero_results_rate ?? '0'),
      top_queries: topQueries,
      no_result_queries: noResultQueries,
    };
  }

  async cleanupOldAnalytics(retentionDays: number): Promise<number> {
    const count = await this.execute(
      `DELETE FROM search_queries
       WHERE source_account_id = $1
         AND created_at < NOW() - INTERVAL '${retentionDays} days'`,
      [this.sourceAccountId]
    );

    return count;
  }

  // =========================================================================
  // Webhook Events
  // =========================================================================

  async insertWebhookEvent(eventType: string, payload: Record<string, unknown>): Promise<void> {
    await this.execute(
      `INSERT INTO search_webhook_events (id, source_account_id, event_type, payload)
       VALUES ($1, $2, $3, $4)`,
      [
        `evt_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
        this.sourceAccountId,
        eventType,
        JSON.stringify(payload),
      ]
    );
  }

  async markEventProcessed(eventId: string, error?: string): Promise<void> {
    await this.execute(
      `UPDATE search_webhook_events
       SET processed = true, processed_at = NOW(), error = $2
       WHERE id = $1`,
      [eventId, error ?? null]
    );
  }
}
