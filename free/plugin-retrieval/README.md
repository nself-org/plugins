# plugin-retrieval

Hybrid retrieval plugin for nSelf. Combines pgvector ANN (cosine similarity) and tsvector
BM25 (full-text search) using Reciprocal Rank Fusion (RRF) to produce ranked results that
outperform either method alone.

**Port:** 3825 | **License:** Free (MIT) | **Bundle:** ɳClaw (AI-CP)

---

## RRF Algorithm

RRF score for each document: `score = Σ 1/(k + rank)` where k=60 (Cormack et al. 2009).

A document appearing in both the vector list (rank 1) and the text list (rank 2) scores:
`1/(60+1) + 1/(60+2) = 0.01639 + 0.01613 = 0.03252`

A document appearing only in the vector list at rank 1 scores:
`1/(60+1) = 0.01639`

The fusion consistently outperforms single-method retrieval for semantic + keyword queries.

---

## API

### POST /search

```json
{
  "query": "optional text query",
  "embedding": [0.1, 0.2, ...],
  "top_k": 10,
  "source_account_id": "primary"
}
```

Provide `embedding` for vector search, `query` for text search, or both for hybrid.
Returns results sorted by RRF score descending.

### POST /index

```json
{
  "id": "doc-1",
  "title": "Document title",
  "content": "Full text content",
  "embedding": [0.1, 0.2, ...],
  "source_account_id": "primary"
}
```

Upserts document + embedding. `embedding` is optional (text-only indexing supported).

### GET /health

Returns `{"status":"ok"}` or `{"status":"unhealthy"}` (503).

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `NSELF_DB_URL` | Yes | — | PostgreSQL connection string (with pgvector enabled) |
| `NSELF_LICENSE_KEY` | No | — | Unused; reserved for future entitlement checks |
| `NSELF_RETRIEVAL_PORT` | No | `3825` | HTTP server port |

---

## Database Tables

| Table | Purpose |
|-------|---------|
| `np_retrieval_documents` | Text corpus (tsvector index on content) |
| `np_retrieval_embeddings` | Vector embeddings (IVFFlat index, 1536 dims) |

Both tables use `source_account_id` (Multi-App Isolation Convention).
Hasura row filters enforce `source_account_id = X-Hasura-Source-Account-Id`.

---

## Installation

```bash
nself plugin install plugin-retrieval
```

---

## License

MIT license — no purchase required.
