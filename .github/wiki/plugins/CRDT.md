# CRDT Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install crdt` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> CRDT offline-first primitives.

## Install

```bash
nself plugin install crdt
```

No license key required.

## Description

CRDT offline-first primitives. Self-hosted Yjs (y-websocket protocol) and automerge sync server with Postgres persistence. Drop-in replacement for Liveblocks/PartyKit with zero extra infra.

Category: `infrastructure`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `CRDT_ENGINE` | `-` | - |
| `CRDT_MAX_DOC_SIZE_MB` | `-` | - |
| `CRDT_RETENTION_DAYS` | `-` | - |
| `CRDT_REDIS_URL` | `-` | - |
| `CRDT_PORT` | `8211` | - |
| `DATABASE_URL` | `-` | PostgreSQL connection string |
| `LOG_LEVEL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 8211 | CRDT service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_crdt_documents`
- `np_crdt_updates`

## REST API

```
POST   /crdt/compact/{docID}
DELETE /crdt/doc/{docID}
GET    /crdt/doc/{docID}
GET    /crdt/docs
POST   /crdt/sync/{docID}
GET    /crdt/yjs/{docID}
GET    /health
```

## Examples

### Health check

```bash
curl http://localhost:8211/health
```

## Source

[`plugins/crdt/`](https://github.com/nself-org/plugins/tree/main/crdt)

Manifest: [`plugins/crdt/plugin.json`](https://github.com/nself-org/plugins/tree/main/crdt/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
