# Plugin Retrieval Plugin

> Hybrid retrieval plugin: pgvector ANN + tsvector BM25 merged with Reciprocal Rank Fusion (RRF). **Free — MIT licensed.**

## Install

```bash
nself plugin install plugin-retrieval
```

No license key required.

## Description

Hybrid retrieval plugin: pgvector ANN + tsvector BM25 merged with Reciprocal Rank Fusion (RRF). Provides the search backend for ɳClaw memory and nself-ai-mcp search/recall tools.

Category: `data`. Current version: `0.1.0`.

## Ports

| Port | Purpose |
|------|---------|
| 3825 | Plugin Retrieval service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_retrieval_documents`
- `np_retrieval_embeddings`

## Examples

```bash
nself plugin install plugin-retrieval
```

## Source

[`plugins/plugin-retrieval/`](https://github.com/nself-org/plugins/tree/main/plugin-retrieval)

Manifest: [`plugins/plugin-retrieval/plugin.json`](https://github.com/nself-org/plugins/tree/main/plugin-retrieval/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
