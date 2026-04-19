# Search Plugin

Full-text search engine with PostgreSQL FTS and MeiliSearch support. Provides index management, document indexing, autocomplete suggestions, synonym management, and search analytics.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Webhooks](#webhooks)
- [Features](#features)
- [Troubleshooting](#troubleshooting)

---

## Overview

| Field | Value |
|-------|-------|
| **Version** | 1.0.0 |
| **Category** | infrastructure |
| **Port** | 3302 |
| **License** | Source-Available |
| **Min nself Version** | 0.4.8 |
| **Multi-App** | Yes (`source_account_id`) |

The Search plugin provides a unified search API backed by PostgreSQL full-text search (tsvector/tsquery) with optional MeiliSearch support. It manages search indexes that map to source database tables, supports document indexing with automatic search vector generation, trigram-based autocomplete suggestions, synonym expansion, faceted search, and query analytics with zero-result tracking.

---

## Quick Start

```bash
nself plugin install search
nself plugin search init
nself plugin search server
nself plugin search indexes create products -t shopify_products -f title,body_html,vendor
nself plugin search reindex products
nself plugin search search "blue shirt"
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SEARCH_PLUGIN_PORT` | `3302` | Server port |
| `SEARCH_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `SEARCH_ENGINE` | `postgres` | Search engine (`postgres` or `meilisearch`) |
| `SEARCH_MEILISEARCH_URL` | - | MeiliSearch URL (required if engine is `meilisearch`) |
| `SEARCH_MEILISEARCH_API_KEY` | - | MeiliSearch API key (required if engine is `meilisearch`) |
| `SEARCH_DEFAULT_LIMIT` | `20` | Default result limit per query |
| `SEARCH_MAX_RESULTS` | `1000` | Maximum results per query |
| `SEARCH_REINDEX_BATCH_SIZE` | `500` | Batch size for reindex operations (1-10000) |
| `SEARCH_ANALYTICS_ENABLED` | `true` | Enable search query analytics |
| `SEARCH_ANALYTICS_RETENTION_DAYS` | `90` | Days to retain analytics data |
| `SEARCH_API_KEY` | - | API key for authentication |
| `SEARCH_RATE_LIMIT_MAX` | `100` | Rate limit max requests |
| `SEARCH_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window |

---

## CLI Commands

| Command | Description | Options |
|---------|-------------|---------|
| `init` | Initialize database schema and extensions | - |
| `server` | Start the API server | `-p, --port <port>` |
| `status` | Show plugin status with index details | - |
| `reindex <index>` | Reindex documents from source table | `-f, --full` (clear existing), `-b, --batch-size <size>` |
| `search <query>` | Search across indexes | `-i, --indexes <indexes>` (comma-separated), `-l, --limit <limit>` |
| `indexes <action> [name]` | Manage indexes (list, create, delete) | `-t, --table <table>`, `-f, --fields <fields>` |
| `synonyms <action> <index> [word]` | Manage synonyms (list, add, delete) | `-s, --synonyms <synonyms>`, `-i, --id <id>` |
| `analytics` | View search analytics | `-d, --days <days>` |

---

## REST API

### Index Management

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/indexes` | Create a new search index |
| `GET` | `/v1/indexes` | List all search indexes |
| `GET` | `/v1/indexes/:name` | Get index details |
| `PUT` | `/v1/indexes/:name` | Update index settings |
| `DELETE` | `/v1/indexes/:name` | Delete an index and its documents |

### Document Management

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/indexes/:name/documents` | Batch index documents (max 1000 per batch) |
| `PUT` | `/v1/indexes/:name/documents/:id` | Update a single document |
| `DELETE` | `/v1/indexes/:name/documents/:id` | Delete a document |

### Search and Suggestions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/search` | Full-text search with filters, facets, and highlighting |
| `GET` | `/v1/suggest` | Autocomplete suggestions via trigram similarity |

### Reindex

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/indexes/:name/reindex` | Reindex from source table |

### Synonyms

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/indexes/:name/synonyms` | Add a synonym mapping |
| `GET` | `/v1/indexes/:name/synonyms` | List synonyms for an index |
| `DELETE` | `/v1/indexes/:name/synonyms/:id` | Delete a synonym |

### Analytics

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/analytics/top-queries` | Top queries by frequency |
| `GET` | `/v1/analytics/no-results` | Queries that returned zero results |

### Maintenance and Health

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/sync` | Cleanup old analytics data |
| `POST` | `/webhook` | Receive webhook events |
| `GET` | `/v1/status` | Plugin status with index summaries |
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (verifies database) |
| `GET` | `/live` | Liveness check with stats |

---

## Database Schema

### `search_indexes`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `name` | VARCHAR(255) | Index name (unique per account) |
| `description` | TEXT | Index description |
| `source_table` | VARCHAR(255) | Source database table for reindexing |
| `source_id_column` | VARCHAR(255) | ID column in source table (default: `id`) |
| `searchable_fields` | TEXT[] | Fields included in search vector |
| `filterable_fields` | TEXT[] | Fields available for filtering |
| `sortable_fields` | TEXT[] | Fields available for sorting |
| `ranking_rules` | JSONB | Ranking rule configuration |
| `settings` | JSONB | Additional index settings |
| `engine` | VARCHAR(32) | Search engine (`postgres` or `meilisearch`) |
| `enabled` | BOOLEAN | Whether index is active |
| `document_count` | INTEGER | Number of indexed documents |
| `last_indexed_at` | TIMESTAMPTZ | Last indexing timestamp |
| `created_at` | TIMESTAMPTZ | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | Last update |

**Indexes:** account, name, enabled. **Unique constraint:** `(source_account_id, name)`.

### `search_documents`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `index_name` | VARCHAR(255) | Parent index name |
| `source_id` | VARCHAR(255) | Original document ID from source table |
| `content` | JSONB | Full document content |
| `search_vector` | TSVECTOR | PostgreSQL full-text search vector |
| `indexed_at` | TIMESTAMPTZ | Indexing timestamp |

**Indexes:** GIN on `search_vector`, GIN on `content` (JSONB), account, index_name, source_id. **Unique constraint:** `(source_account_id, index_name, source_id)`.

### `search_synonyms`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `index_name` | VARCHAR(255) | Parent index name |
| `word` | VARCHAR(255) | Primary word |
| `synonyms` | TEXT[] | Array of synonym words |
| `created_at` | TIMESTAMPTZ | Creation timestamp |

**Indexes:** account, index_name, word. **Unique constraint:** `(source_account_id, index_name, word)`.

### `search_queries`

| Column | Type | Description |
|--------|------|-------------|
| `id` | UUID | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `index_name` | VARCHAR(255) | Index searched |
| `query_text` | TEXT | Search query text |
| `filters` | JSONB | Applied filters |
| `result_count` | INTEGER | Number of results returned |
| `took_ms` | INTEGER | Query processing time in milliseconds |
| `user_id` | VARCHAR(255) | User who performed the search |
| `clicked_result_id` | VARCHAR(255) | Result the user clicked (for relevance tracking) |
| `created_at` | TIMESTAMPTZ | Query timestamp |

**Indexes:** GIN on `to_tsvector('english', query_text)`, account, index_name, created_at.

### `search_webhook_events`

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key |
| `source_account_id` | VARCHAR(128) | Multi-app isolation |
| `event_type` | VARCHAR(128) | Event type |
| `payload` | JSONB | Event payload |
| `processed` | BOOLEAN | Whether event has been processed |
| `processed_at` | TIMESTAMPTZ | Processing timestamp |
| `error` | TEXT | Error message if processing failed |
| `created_at` | TIMESTAMPTZ | Event timestamp |

**Indexes:** account, event_type, processed, created_at.

---

## Webhooks

### Supported Events

| Event | Description |
|-------|-------------|
| `index.created` | New search index created |
| `index.updated` | Search index settings updated |
| `index.deleted` | Search index removed |
| `document.indexed` | Document added to index |
| `document.updated` | Document updated in index |
| `document.deleted` | Document removed from index |

---

## Features

- **Dual engine support** with PostgreSQL FTS (default) and optional MeiliSearch backend
- **Full-text search** using `websearch_to_tsquery` with relevance ranking via `ts_rank_cd`
- **Trigram similarity** autocomplete suggestions using `pg_trgm` extension
- **Faceted search** with automatic JSONB field aggregation
- **Search highlighting** with `ts_headline` for result snippets
- **Batch reindexing** from any PostgreSQL source table with configurable batch sizes
- **Synonym management** with upsert support per index
- **Search analytics** tracking query frequency, response times, and zero-result rates
- **Analytics cleanup** with configurable retention period (default 90 days)
- **Multi-app isolation** via `source_account_id` on all tables

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Search returns no results | Ensure documents are indexed: run `reindex <index>` or `POST /v1/indexes/:name/documents` |
| Full-text search not working | Verify `pg_trgm` extension is installed: `CREATE EXTENSION IF NOT EXISTS pg_trgm` |
| MeiliSearch connection fails | Check `SEARCH_MEILISEARCH_URL` and `SEARCH_MEILISEARCH_API_KEY` are set |
| Reindex fails with "no source table" | Create index with `--table` option pointing to a valid PostgreSQL table |
| Analytics endpoint returns 404 | Set `SEARCH_ANALYTICS_ENABLED=true` (default) |
| Slow search queries | Add GIN indexes on `search_vector` and `content` columns |
| Default limit too low | Adjust `SEARCH_DEFAULT_LIMIT` (max allowed: `SEARCH_MAX_RESULTS`) |
