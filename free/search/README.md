# Search Plugin

Full-text search engine plugin for nself with PostgreSQL FTS and MeiliSearch support.

## Features

- **PostgreSQL Full-Text Search**: Built-in FTS using tsvector and tsquery
- **MeiliSearch Support**: Optional MeiliSearch backend for advanced search
- **Multi-Index Search**: Search across multiple indexes simultaneously
- **Faceted Search**: Filter and aggregate results by fields
- **Autocomplete**: Prefix-based suggestions using trigram similarity
- **Synonym Management**: Define word synonyms per index
- **Search Analytics**: Track queries, response times, and zero-results
- **Multi-App Support**: Isolated search per source_account_id
- **Webhook Events**: Track index and document changes

## Quick Start

### 1. Installation

```bash
cd plugins/search/ts
npm install
npm run build
```

### 2. Configuration

```bash
# Required
export DATABASE_URL="postgresql://user:pass@localhost:5432/nself"

# Optional
export SEARCH_PLUGIN_PORT=3302
export SEARCH_ENGINE=postgres  # or meilisearch
export SEARCH_API_KEY=your-secret-key
```

### 3. Initialize Schema

```bash
npm start -- init
```

### 4. Start Server

```bash
npm start -- server
```

## CLI Commands

```bash
# Initialize schema
nself-search init

# Start server
nself-search server [-p 3302]

# View status
nself-search status

# Create index
nself-search indexes create products -t products -f "name,description"

# List indexes
nself-search indexes list

# Reindex from source table
nself-search reindex products [--full] [--batch-size 500]

# Search
nself-search search "query text" [-i index1,index2] [-l 10]

# Manage synonyms
nself-search synonyms add products shoes --synonyms "sneakers,trainers,footwear"
nself-search synonyms list products

# View analytics
nself-search analytics [-d 30]
```

## API Endpoints

### Health Checks
- `GET /health` - Basic health check
- `GET /ready` - Database readiness check
- `GET /live` - Liveness check with stats
- `GET /v1/status` - Full status with indexes

### Index Management
- `POST /v1/indexes` - Create index
- `GET /v1/indexes` - List all indexes
- `GET /v1/indexes/:name` - Get index details
- `PUT /v1/indexes/:name` - Update index settings
- `DELETE /v1/indexes/:name` - Delete index

### Document Management
- `POST /v1/indexes/:name/documents` - Index documents (batch up to 1000)
- `PUT /v1/indexes/:name/documents/:id` - Update single document
- `DELETE /v1/indexes/:name/documents/:id` - Remove document
- `POST /v1/indexes/:name/reindex` - Reindex from source table

### Search Operations
- `POST /v1/search` - Search across indexes
- `GET /v1/suggest` - Autocomplete suggestions

### Synonym Management
- `POST /v1/indexes/:name/synonyms` - Add synonym
- `GET /v1/indexes/:name/synonyms` - List synonyms
- `DELETE /v1/indexes/:name/synonyms/:id` - Remove synonym

### Analytics
- `GET /v1/analytics/top-queries` - Most common searches
- `GET /v1/analytics/no-results` - Zero-result queries

### Maintenance
- `POST /v1/sync` - Cleanup old analytics

## Database Schema

### Tables

#### search_indexes
- `id` (UUID, PK)
- `source_account_id` (VARCHAR)
- `name` (VARCHAR) - Unique index name
- `description` (TEXT)
- `source_table` (VARCHAR) - Optional source table
- `source_id_column` (VARCHAR) - ID column in source (default: 'id')
- `searchable_fields` (TEXT[]) - Fields to include in FTS
- `filterable_fields` (TEXT[]) - Fields for filtering
- `sortable_fields` (TEXT[]) - Fields for sorting
- `ranking_rules` (JSONB) - Search ranking configuration
- `settings` (JSONB) - Additional settings
- `engine` (VARCHAR) - postgres or meilisearch
- `enabled` (BOOLEAN)
- `document_count` (INTEGER)
- `last_indexed_at` (TIMESTAMPTZ)

#### search_documents
- `id` (UUID, PK)
- `source_account_id` (VARCHAR)
- `index_name` (VARCHAR)
- `source_id` (VARCHAR) - Original document ID
- `content` (JSONB) - Document content
- `search_vector` (tsvector) - PostgreSQL FTS vector
- `indexed_at` (TIMESTAMPTZ)

Indexes: GIN on `search_vector`, GIN on `content`

#### search_synonyms
- `id` (UUID, PK)
- `source_account_id` (VARCHAR)
- `index_name` (VARCHAR)
- `word` (VARCHAR)
- `synonyms` (TEXT[])

#### search_queries
- `id` (UUID, PK)
- `source_account_id` (VARCHAR)
- `index_name` (VARCHAR)
- `query_text` (TEXT)
- `filters` (JSONB)
- `result_count` (INTEGER)
- `took_ms` (INTEGER)
- `user_id` (VARCHAR)
- `clicked_result_id` (VARCHAR)

## Search Implementation

### PostgreSQL FTS

1. **Document Indexing**: Builds `tsvector` from searchable fields using `to_tsvector('english', content)`
2. **Query Parsing**: Uses `websearch_to_tsquery` for natural query syntax
3. **Ranking**: Results ranked using `ts_rank_cd` for positional relevance
4. **Highlighting**: Uses `ts_headline` for matched text snippets
5. **Faceting**: Aggregates on filterable fields with GROUP BY
6. **Prefix Search**: Trigram similarity for autocomplete

### Search Request Example

```bash
curl -X POST http://localhost:3302/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "q": "search query",
    "indexes": ["products", "articles"],
    "filter": {"category": "electronics"},
    "facets": ["brand", "price_range"],
    "sort": ["created_at:desc"],
    "limit": 20,
    "offset": 0,
    "highlight": true
  }'
```

### Index Documents Example

```bash
curl -X POST http://localhost:3302/v1/indexes/products/documents \
  -H "Content-Type: application/json" \
  -d '{
    "documents": [
      {
        "id": "prod_123",
        "name": "Wireless Mouse",
        "description": "Ergonomic wireless mouse with USB receiver",
        "price": 29.99,
        "category": "electronics"
      }
    ]
  }'
```

## Environment Variables

### Required
- `DATABASE_URL` - PostgreSQL connection string

### Optional
- `SEARCH_PLUGIN_PORT` (default: 3302) - Server port
- `SEARCH_ENGINE` (default: postgres) - Search backend: postgres or meilisearch
- `SEARCH_MEILISEARCH_URL` - MeiliSearch URL (required if engine=meilisearch)
- `SEARCH_MEILISEARCH_API_KEY` - MeiliSearch API key (required if engine=meilisearch)
- `SEARCH_DEFAULT_LIMIT` (default: 20) - Default result limit
- `SEARCH_MAX_RESULTS` (default: 1000) - Maximum results per query
- `SEARCH_REINDEX_BATCH_SIZE` (default: 500) - Batch size for reindexing
- `SEARCH_ANALYTICS_ENABLED` (default: true) - Enable query analytics
- `SEARCH_ANALYTICS_RETENTION_DAYS` (default: 90) - Days to retain analytics
- `SEARCH_API_KEY` - API key for authentication
- `SEARCH_RATE_LIMIT_MAX` (default: 100) - Max requests per window
- `SEARCH_RATE_LIMIT_WINDOW_MS` (default: 60000) - Rate limit window

## Development

```bash
# Install dependencies
npm install

# Type check
npm run typecheck

# Build
npm run build

# Watch mode
npm run watch

# Dev server
npm run dev
```

## License

MIT
