# Search Plugin

Full-text search engine with PostgreSQL FTS and MeiliSearch support for nself. Provides powerful search capabilities with analytics, autocomplete, and synonym management.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Search Engines](#search-engines)
- [Analytics](#analytics)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Search plugin provides comprehensive full-text search capabilities for your nself application. It supports two backend engines:

- **PostgreSQL FTS** - Built-in full-text search using PostgreSQL's native capabilities
- **MeiliSearch** - External search engine for advanced features and performance

### Key Features

- **Multiple Search Engines** - Choose between PostgreSQL FTS or MeiliSearch
- **Full-Text Search** - Powerful search with relevance ranking and highlighting
- **Autocomplete** - Fast suggestions using trigram similarity
- **Synonym Support** - Define word synonyms for better search results
- **Faceted Search** - Filter and aggregate search results by attributes
- **Search Analytics** - Track queries, popular searches, and zero-result queries
- **Index Management** - Create, configure, and manage multiple search indexes
- **Source Table Reindexing** - Automatically sync documents from database tables
- **Multi-Account Support** - Isolate search data per account

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Search Indexes | Index configurations and metadata | `np_search_indexes` |
| Search Documents | Indexed documents with search vectors | `np_search_documents` |
| Search Synonyms | Word synonyms for query expansion | `np_search_synonyms` |
| Search Queries | Query analytics and history | `np_search_queries` |
| Webhook Events | Inbound webhook event log | `np_search_webhook_events` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install search

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "SEARCH_ENGINE=postgres" >> .env
echo "SEARCH_PLUGIN_PORT=3302" >> .env

# Initialize database schema
nself plugin search init

# Create a search index
nself plugin search indexes create products \
  --table products \
  --fields "name,description,category"

# Reindex documents from source table
nself plugin search reindex products --full

# Start server
nself plugin search server --port 3302

# Search
nself plugin search search "laptop" --indexes products --limit 10
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `SEARCH_PLUGIN_PORT` | No | `3302` | HTTP server port |
| `SEARCH_ENGINE` | No | `postgres` | Search engine: `postgres` or `meilisearch` |
| `SEARCH_MEILISEARCH_URL` | Conditional | - | MeiliSearch URL (required if engine=meilisearch) |
| `SEARCH_MEILISEARCH_API_KEY` | Conditional | - | MeiliSearch API key (required if engine=meilisearch) |
| `SEARCH_DEFAULT_LIMIT` | No | `20` | Default result limit per query |
| `SEARCH_MAX_RESULTS` | No | `1000` | Maximum results returned per query |
| `SEARCH_REINDEX_BATCH_SIZE` | No | `500` | Batch size for reindexing operations |
| `SEARCH_ANALYTICS_ENABLED` | No | `true` | Enable search query analytics |
| `SEARCH_ANALYTICS_RETENTION_DAYS` | No | `90` | Days to retain analytics data |
| `SEARCH_API_KEY` | No | - | API key for authentication (optional) |
| `SEARCH_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `SEARCH_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Search Engine
SEARCH_ENGINE=postgres
# For MeiliSearch:
# SEARCH_ENGINE=meilisearch
# SEARCH_MEILISEARCH_URL=http://localhost:7700
# SEARCH_MEILISEARCH_API_KEY=your_master_key

# Server
SEARCH_PLUGIN_PORT=3302
LOG_LEVEL=info

# Search Settings
SEARCH_DEFAULT_LIMIT=20
SEARCH_MAX_RESULTS=1000
SEARCH_REINDEX_BATCH_SIZE=500

# Analytics
SEARCH_ANALYTICS_ENABLED=true
SEARCH_ANALYTICS_RETENTION_DAYS=90

# Security (optional)
SEARCH_API_KEY=your_api_key_here
SEARCH_RATE_LIMIT_MAX=100
SEARCH_RATE_LIMIT_WINDOW_MS=60000
```

### MeiliSearch Setup

If using MeiliSearch engine:

```bash
# Install MeiliSearch
brew install meilisearch

# Start MeiliSearch
meilisearch --master-key=your_master_key

# Configure plugin
export SEARCH_ENGINE=meilisearch
export SEARCH_MEILISEARCH_URL=http://localhost:7700
export SEARCH_MEILISEARCH_API_KEY=your_master_key
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin search init

# Check plugin status
nself plugin search status

# Start HTTP server
nself plugin search server
nself plugin search server --port 3302
```

### Index Management

```bash
# List all indexes
nself plugin search indexes list

# Create a new index
nself plugin search indexes create products \
  --table products \
  --fields "name,description,category"

# Delete an index
nself plugin search indexes delete products
```

### Reindexing

```bash
# Reindex all documents from source table
nself plugin search reindex products

# Full reindex (clear existing documents first)
nself plugin search reindex products --full

# Custom batch size
nself plugin search reindex products --batch-size 1000
```

### Search Operations

```bash
# Search across indexes
nself plugin search search "laptop"

# Search specific indexes
nself plugin search search "laptop" --indexes products,articles

# Limit results
nself plugin search search "laptop" --limit 20
```

### Synonym Management

```bash
# List synonyms for an index
nself plugin search synonyms list products

# Add synonym
nself plugin search synonyms add products laptop \
  --synonyms "notebook,computer,pc"

# Delete synonym
nself plugin search synonyms delete products --id <synonym-id>
```

### Analytics

```bash
# View search analytics (last 30 days)
nself plugin search analytics

# Custom time range
nself plugin search analytics --days 7
```

---

## REST API

The plugin exposes a REST API when running the server.

### Base URL

```
http://localhost:3302
```

### Health & Status

#### Health Check

```http
GET /health
```

**Response:**
```json
{
  "status": "ok",
  "plugin": "search",
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### Readiness Check

```http
GET /ready
```

**Response:**
```json
{
  "ready": true,
  "plugin": "search",
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### Liveness Check

```http
GET /live
```

**Response:**
```json
{
  "alive": true,
  "plugin": "search",
  "version": "1.0.0",
  "engine": "postgres",
  "uptime": 3600.5,
  "memory": {
    "rss": 123456789,
    "heapTotal": 12345678,
    "heapUsed": 9876543
  },
  "stats": {
    "indexes": 3,
    "documents": 15420
  },
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### Status

```http
GET /v1/status
```

**Response:**
```json
{
  "plugin": "search",
  "version": "1.0.0",
  "engine": "postgres",
  "status": "running",
  "indexes": [
    {
      "name": "products",
      "enabled": true,
      "document_count": 1245,
      "last_indexed_at": "2026-02-11T12:00:00.000Z"
    }
  ],
  "stats": {
    "total_queries": 5420,
    "avg_time_ms": 45.3
  },
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

### Index Management

#### Create Index

```http
POST /v1/indexes
Content-Type: application/json

{
  "name": "products",
  "description": "Product catalog search",
  "source_table": "products",
  "source_id_column": "id",
  "searchable_fields": ["name", "description", "category"],
  "filterable_fields": ["category", "brand", "price"],
  "sortable_fields": ["created_at", "price", "name"],
  "ranking_rules": ["words", "typo", "proximity", "attribute", "sort", "exactness"],
  "settings": {},
  "engine": "postgres"
}
```

**Response:** `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "name": "products",
  "description": "Product catalog search",
  "source_table": "products",
  "source_id_column": "id",
  "searchable_fields": ["name", "description", "category"],
  "filterable_fields": ["category", "brand", "price"],
  "sortable_fields": ["created_at", "price", "name"],
  "ranking_rules": ["words", "typo", "proximity", "attribute", "sort", "exactness"],
  "settings": {},
  "engine": "postgres",
  "enabled": true,
  "document_count": 0,
  "last_indexed_at": null,
  "created_at": "2026-02-11T12:00:00.000Z",
  "updated_at": "2026-02-11T12:00:00.000Z"
}
```

#### List Indexes

```http
GET /v1/indexes
```

**Response:**
```json
{
  "indexes": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "products",
      "enabled": true,
      "document_count": 1245,
      "last_indexed_at": "2026-02-11T12:00:00.000Z"
    }
  ],
  "count": 1
}
```

#### Get Index

```http
GET /v1/indexes/:name
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "products",
  "description": "Product catalog search",
  "source_table": "products",
  "searchable_fields": ["name", "description", "category"],
  "enabled": true,
  "document_count": 1245
}
```

#### Update Index

```http
PUT /v1/indexes/:name
Content-Type: application/json

{
  "description": "Updated description",
  "searchable_fields": ["name", "description", "category", "tags"],
  "enabled": true
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "products",
  "description": "Updated description",
  "searchable_fields": ["name", "description", "category", "tags"],
  "updated_at": "2026-02-11T12:00:00.000Z"
}
```

#### Delete Index

```http
DELETE /v1/indexes/:name
```

**Response:**
```json
{
  "deleted": true
}
```

### Document Management

#### Index Documents (Batch)

```http
POST /v1/indexes/:name/documents
Content-Type: application/json

{
  "documents": [
    {
      "id": "prod_001",
      "name": "Laptop Pro",
      "description": "High-performance laptop",
      "category": "Electronics",
      "price": 1299.99
    },
    {
      "id": "prod_002",
      "name": "Mouse Wireless",
      "description": "Ergonomic wireless mouse",
      "category": "Accessories",
      "price": 29.99
    }
  ]
}
```

**Response:**
```json
{
  "indexed": 2,
  "total": 2,
  "failed": 0
}
```

#### Index Single Document

```http
PUT /v1/indexes/:name/documents/:id
Content-Type: application/json

{
  "name": "Laptop Pro",
  "description": "High-performance laptop",
  "category": "Electronics",
  "price": 1299.99
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440001",
  "source_account_id": "primary",
  "index_name": "products",
  "source_id": "prod_001",
  "content": {
    "name": "Laptop Pro",
    "description": "High-performance laptop",
    "category": "Electronics",
    "price": 1299.99
  },
  "indexed_at": "2026-02-11T12:00:00.000Z"
}
```

#### Delete Document

```http
DELETE /v1/indexes/:name/documents/:id
```

**Response:**
```json
{
  "deleted": true
}
```

### Reindexing

#### Reindex from Source Table

```http
POST /v1/indexes/:name/reindex
Content-Type: application/json

{
  "fullReindex": true,
  "batchSize": 500
}
```

**Response:**
```json
{
  "indexed": 1245,
  "failed": 0,
  "duration": 12450,
  "errors": []
}
```

### Search Operations

#### Search

```http
POST /v1/search
Content-Type: application/json

{
  "q": "laptop",
  "indexes": ["products"],
  "limit": 20,
  "offset": 0,
  "filter": {
    "category": "Electronics"
  },
  "facets": ["category", "brand"],
  "highlight": true
}
```

**Response:**
```json
{
  "hits": [
    {
      "id": "prod_001",
      "index": "products",
      "score": 0.9523,
      "content": {
        "name": "Laptop Pro",
        "description": "High-performance laptop",
        "category": "Electronics",
        "price": 1299.99
      },
      "highlights": {
        "_highlight": ["High-performance <em>laptop</em>"]
      }
    }
  ],
  "total": 15,
  "limit": 20,
  "offset": 0,
  "query": "laptop",
  "processingTimeMs": 45,
  "facets": [
    {
      "field": "category",
      "values": [
        { "value": "Electronics", "count": 12 },
        { "value": "Accessories", "count": 3 }
      ]
    }
  ]
}
```

#### Autocomplete / Suggestions

```http
GET /v1/suggest?q=lap&limit=10
```

**Response:**
```json
{
  "suggestions": [
    {
      "value": "laptop",
      "index": "products"
    },
    {
      "value": "lapto",
      "index": "products"
    }
  ],
  "query": "lap",
  "processingTimeMs": 12
}
```

### Synonym Management

#### Add Synonym

```http
POST /v1/indexes/:name/synonyms
Content-Type: application/json

{
  "word": "laptop",
  "synonyms": ["notebook", "computer", "pc"]
}
```

**Response:** `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440002",
  "source_account_id": "primary",
  "index_name": "products",
  "word": "laptop",
  "synonyms": ["notebook", "computer", "pc"],
  "created_at": "2026-02-11T12:00:00.000Z"
}
```

#### List Synonyms

```http
GET /v1/indexes/:name/synonyms
```

**Response:**
```json
{
  "synonyms": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "word": "laptop",
      "synonyms": ["notebook", "computer", "pc"]
    }
  ],
  "count": 1
}
```

#### Delete Synonym

```http
DELETE /v1/indexes/:name/synonyms/:id
```

**Response:**
```json
{
  "deleted": true
}
```

### Analytics

#### Top Queries

```http
GET /v1/analytics/top-queries?limit=20&days=30
```

**Response:**
```json
{
  "queries": [
    {
      "query": "laptop",
      "count": 245,
      "avg_results": 15.3,
      "avg_time_ms": 42.5
    },
    {
      "query": "mouse",
      "count": 189,
      "avg_results": 23.1,
      "avg_time_ms": 38.2
    }
  ],
  "count": 2
}
```

#### No-Result Queries

```http
GET /v1/analytics/no-results?limit=20&days=30
```

**Response:**
```json
{
  "queries": [
    {
      "query": "xyz product",
      "count": 15,
      "last_searched": "2026-02-11T12:00:00.000Z"
    }
  ],
  "count": 1
}
```

### Sync Endpoint

#### Cleanup Old Analytics

```http
POST /v1/sync
```

**Response:**
```json
{
  "message": "Analytics cleanup completed",
  "records_deleted": 1245,
  "retention_days": 90
}
```

### Webhook Endpoint

```http
POST /webhook
Content-Type: application/json

{
  "type": "index.created",
  "data": {
    "index_name": "products"
  }
}
```

**Response:**
```json
{
  "received": true,
  "type": "index.created"
}
```

---

## Webhook Events

The plugin emits webhook events for search operations:

| Event | Description | Payload |
|-------|-------------|---------|
| `index.created` | New search index created | `{ index_name, source_table }` |
| `index.updated` | Search index settings updated | `{ index_name, changes }` |
| `index.deleted` | Search index removed | `{ index_name }` |
| `document.indexed` | Document added to index | `{ index_name, document_id }` |
| `document.updated` | Document updated in index | `{ index_name, document_id }` |
| `document.deleted` | Document removed from index | `{ index_name, document_id }` |

---

## Database Schema

### np_search_indexes

Stores search index configurations.

```sql
CREATE TABLE np_search_indexes (
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

CREATE INDEX idx_search_indexes_account ON np_search_indexes(source_account_id);
CREATE INDEX idx_search_indexes_name ON np_search_indexes(name);
CREATE INDEX idx_search_indexes_enabled ON np_search_indexes(enabled);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | uuid_generate_v4() | Unique identifier |
| `source_account_id` | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| `name` | VARCHAR(255) | No | - | Index name (unique per account) |
| `description` | TEXT | Yes | NULL | Human-readable description |
| `source_table` | VARCHAR(255) | Yes | NULL | Source table for reindexing |
| `source_id_column` | VARCHAR(255) | No | 'id' | Primary key column name |
| `searchable_fields` | TEXT[] | No | - | Fields to include in full-text search |
| `filterable_fields` | TEXT[] | No | {} | Fields available for filtering |
| `sortable_fields` | TEXT[] | No | {} | Fields available for sorting |
| `ranking_rules` | JSONB | No | ["words",...] | Relevance ranking rules |
| `settings` | JSONB | No | {} | Engine-specific settings |
| `engine` | VARCHAR(32) | No | 'postgres' | Search engine (postgres/meilisearch) |
| `enabled` | BOOLEAN | No | true | Index enabled/disabled |
| `document_count` | INTEGER | No | 0 | Total indexed documents |
| `last_indexed_at` | TIMESTAMP | Yes | NULL | Last successful reindex time |
| `created_at` | TIMESTAMP | No | NOW() | Index creation time |
| `updated_at` | TIMESTAMP | No | NOW() | Last update time |

### np_search_documents

Stores indexed documents with search vectors.

```sql
CREATE TABLE np_search_documents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  index_name VARCHAR(255) NOT NULL,
  source_id VARCHAR(255) NOT NULL,
  content JSONB NOT NULL,
  np_search_vector tsvector,
  indexed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, index_name, source_id)
);

CREATE INDEX idx_search_documents_account ON np_search_documents(source_account_id);
CREATE INDEX idx_search_documents_index ON np_search_documents(index_name);
CREATE INDEX idx_search_documents_source_id ON np_search_documents(source_id);
CREATE INDEX idx_search_documents_vector ON np_search_documents USING GIN(np_search_vector);
CREATE INDEX idx_search_documents_content ON np_search_documents USING GIN(content);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | uuid_generate_v4() | Unique identifier |
| `source_account_id` | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| `index_name` | VARCHAR(255) | No | - | Associated index name |
| `source_id` | VARCHAR(255) | No | - | Original document ID |
| `content` | JSONB | No | - | Document data |
| `np_search_vector` | tsvector | Yes | NULL | PostgreSQL FTS vector |
| `indexed_at` | TIMESTAMP | No | NOW() | Document indexing time |

### np_search_synonyms

Stores word synonyms for query expansion.

```sql
CREATE TABLE np_search_synonyms (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  index_name VARCHAR(255) NOT NULL,
  word VARCHAR(255) NOT NULL,
  synonyms TEXT[] NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, index_name, word)
);

CREATE INDEX idx_search_synonyms_account ON np_search_synonyms(source_account_id);
CREATE INDEX idx_search_synonyms_index ON np_search_synonyms(index_name);
CREATE INDEX idx_search_synonyms_word ON np_search_synonyms(word);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | uuid_generate_v4() | Unique identifier |
| `source_account_id` | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| `index_name` | VARCHAR(255) | No | - | Associated index name |
| `word` | VARCHAR(255) | No | - | Word to expand |
| `synonyms` | TEXT[] | No | - | Array of synonyms |
| `created_at` | TIMESTAMP | No | NOW() | Creation time |

### np_search_queries

Stores search query analytics.

```sql
CREATE TABLE np_search_queries (
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

CREATE INDEX idx_search_queries_account ON np_search_queries(source_account_id);
CREATE INDEX idx_search_queries_index ON np_search_queries(index_name);
CREATE INDEX idx_search_queries_text ON np_search_queries USING GIN(to_tsvector('english', query_text));
CREATE INDEX idx_search_queries_created ON np_search_queries(created_at);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | UUID | No | uuid_generate_v4() | Unique identifier |
| `source_account_id` | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| `index_name` | VARCHAR(255) | Yes | NULL | Index queried (NULL for multi-index) |
| `query_text` | TEXT | No | - | Search query string |
| `filters` | JSONB | Yes | NULL | Applied filters |
| `result_count` | INTEGER | No | 0 | Number of results returned |
| `took_ms` | INTEGER | Yes | NULL | Query execution time (ms) |
| `user_id` | VARCHAR(255) | Yes | NULL | User who performed search |
| `clicked_result_id` | VARCHAR(255) | Yes | NULL | Clicked result (for CTR tracking) |
| `created_at` | TIMESTAMP | No | NOW() | Query time |

### np_search_webhook_events

Stores inbound webhook events.

```sql
CREATE TABLE np_search_webhook_events (
  id VARCHAR(255) PRIMARY KEY,
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128),
  payload JSONB,
  processed BOOLEAN DEFAULT false,
  processed_at TIMESTAMP WITH TIME ZONE,
  error TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_search_webhook_events_account ON np_search_webhook_events(source_account_id);
CREATE INDEX idx_search_webhook_events_type ON np_search_webhook_events(event_type);
CREATE INDEX idx_search_webhook_events_processed ON np_search_webhook_events(processed);
CREATE INDEX idx_search_webhook_events_created ON np_search_webhook_events(created_at);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| `id` | VARCHAR(255) | No | - | Event ID |
| `source_account_id` | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| `event_type` | VARCHAR(128) | Yes | NULL | Webhook event type |
| `payload` | JSONB | Yes | NULL | Event payload |
| `processed` | BOOLEAN | No | false | Processing status |
| `processed_at` | TIMESTAMP | Yes | NULL | Processing completion time |
| `error` | TEXT | Yes | NULL | Error message if failed |
| `created_at` | TIMESTAMP | No | NOW() | Event received time |

---

## Search Engines

### PostgreSQL FTS

PostgreSQL full-text search uses native `tsvector` and `tsquery` features.

**Advantages:**
- No external dependencies
- Built into PostgreSQL
- Good performance for small to medium datasets
- Supports multiple languages
- Trigram similarity for fuzzy matching

**Configuration:**
```bash
SEARCH_ENGINE=postgres
```

**Features:**
- Full-text search with relevance ranking
- Autocomplete using trigram similarity (pg_trgm)
- Highlighting with `ts_headline`
- Stop words and stemming
- Multiple language dictionaries

### MeiliSearch

MeiliSearch is a fast, open-source search engine with advanced features.

**Advantages:**
- Extremely fast (built in Rust)
- Typo tolerance
- Advanced ranking
- Better autocomplete
- Faceted search
- Geo-search support

**Configuration:**
```bash
SEARCH_ENGINE=meilisearch
SEARCH_MEILISEARCH_URL=http://localhost:7700
SEARCH_MEILISEARCH_API_KEY=your_master_key
```

**Setup:**
```bash
# Install MeiliSearch
curl -L https://install.meilisearch.com | sh

# Start MeiliSearch
meilisearch --master-key=your_master_key
```

---

## Analytics

When `SEARCH_ANALYTICS_ENABLED=true`, the plugin tracks:

- **Total Queries** - Total number of searches
- **Unique Queries** - Distinct query strings
- **Average Results** - Average number of results per query
- **Average Response Time** - Average query execution time
- **Zero Results Rate** - Percentage of queries with no results
- **Top Queries** - Most popular search terms
- **No-Result Queries** - Searches with zero results (for improvement)

### Viewing Analytics

```bash
# CLI
nself plugin search analytics --days 30

# API
curl http://localhost:3302/v1/analytics/top-queries?days=30
curl http://localhost:3302/v1/analytics/no-results?days=30
```

### Analytics Retention

Analytics data is automatically cleaned up based on `SEARCH_ANALYTICS_RETENTION_DAYS`.

```bash
# Trigger cleanup manually
curl -X POST http://localhost:3302/v1/sync
```

---

## Examples

### Example 1: Product Search with Filters

```bash
# Create index
nself plugin search indexes create products \
  --table products \
  --fields "name,description,category,brand"

# Reindex
nself plugin search reindex products --full

# Search with API
curl -X POST http://localhost:3302/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "q": "laptop",
    "indexes": ["products"],
    "filter": {
      "category": "Electronics",
      "brand": "Apple"
    },
    "limit": 10,
    "highlight": true
  }'
```

### Example 2: Multi-Index Search

```sql
-- Search across multiple indexes
SELECT * FROM np_search_documents
WHERE index_name IN ('products', 'articles', 'support')
  AND np_search_vector @@ websearch_to_tsquery('english', 'laptop repair')
ORDER BY ts_rank_cd(np_search_vector, websearch_to_tsquery('english', 'laptop repair')) DESC
LIMIT 20;
```

### Example 3: Autocomplete

```bash
# Via CLI
nself plugin search search "lap" --limit 5

# Via API
curl http://localhost:3302/v1/suggest?q=lap&limit=5
```

### Example 4: Faceted Search

```bash
curl -X POST http://localhost:3302/v1/search \
  -H "Content-Type: application/json" \
  -d '{
    "q": "laptop",
    "indexes": ["products"],
    "facets": ["category", "brand", "price_range"],
    "limit": 20
  }'
```

### Example 5: Top Searches This Week

```sql
SELECT
  query_text,
  COUNT(*) as np_search_count,
  AVG(result_count) as avg_results,
  AVG(took_ms) as avg_time_ms
FROM np_search_queries
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY query_text
ORDER BY np_search_count DESC
LIMIT 20;
```

### Example 6: Zero-Result Queries (Need Attention)

```sql
SELECT
  query_text,
  COUNT(*) as attempts,
  MAX(created_at) as last_attempt
FROM np_search_queries
WHERE result_count = 0
  AND created_at > NOW() - INTERVAL '30 days'
GROUP BY query_text
ORDER BY attempts DESC
LIMIT 20;
```

---

## Troubleshooting

### Common Issues

#### "Extension pg_trgm not found"

```
ERROR: Extension "pg_trgm" not found
```

**Solution:** Install PostgreSQL trigram extension:

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
```

If you don't have permissions, ask your database administrator to install it.

#### "No results found" for valid queries

```
Search returns 0 results even for known documents
```

**Solutions:**

1. Check that documents are indexed:
   ```bash
   nself plugin search status
   ```

2. Verify search vector is populated:
   ```sql
   SELECT index_name, COUNT(*) as doc_count
   FROM np_search_documents
   WHERE np_search_vector IS NOT NULL
   GROUP BY index_name;
   ```

3. Reindex the index:
   ```bash
   nself plugin search reindex products --full
   ```

#### "MeiliSearch connection failed"

```
Error: Failed to connect to MeiliSearch
```

**Solutions:**

1. Verify MeiliSearch is running:
   ```bash
   curl http://localhost:7700/health
   ```

2. Check API key:
   ```bash
   echo $SEARCH_MEILISEARCH_API_KEY
   ```

3. Verify URL:
   ```bash
   echo $SEARCH_MEILISEARCH_URL
   ```

#### "Rate limit exceeded"

```
Error: Too many requests
```

**Solution:** Increase rate limits:

```bash
SEARCH_RATE_LIMIT_MAX=200
SEARCH_RATE_LIMIT_WINDOW_MS=60000
```

#### "Reindex timeout"

```
Reindex operation times out for large datasets
```

**Solutions:**

1. Reduce batch size:
   ```bash
   nself plugin search reindex products --batch-size 100
   ```

2. Increase Node.js memory:
   ```bash
   NODE_OPTIONS="--max-old-space-size=4096" nself plugin search reindex products
   ```

3. Run reindex in background:
   ```bash
   nohup nself plugin search reindex products --full > reindex.log 2>&1 &
   ```

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
LOG_LEVEL=debug nself plugin search server
```

### Performance Optimization

For large datasets:

```sql
-- Create additional indexes for common filters
CREATE INDEX idx_search_documents_category
  ON np_search_documents((content->>'category'));

CREATE INDEX idx_search_documents_brand
  ON np_search_documents((content->>'brand'));

-- Analyze tables
ANALYZE np_search_documents;
ANALYZE np_search_indexes;
```

---

*Last Updated: February 11, 2026*
*Plugin Version: 1.0.0*
*nself Version: 0.4.8+*
