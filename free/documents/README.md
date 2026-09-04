# Documents Plugin

> Document management and generation service with templates, versioning, and sharing.

**Tier:** Free (MIT) — no license required.

## Install

```bash
nself plugin install documents
nself build
```

## Description

Document management and generation service with templates, versioning, and sharing.

Templates support Markdown, HTML, and a small DSL for variable substitution. Generated documents are versioned, can be shared via signed URLs, and emit webhook events on every state change. Storage backends are pluggable (local, MinIO, S3).

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Required env var |
| `DOCS_PLUGIN_PORT` | No | — | Optional override |
| `DOCS_APP_IDS` | No | — | Optional override |
| `DOCS_PDF_ENGINE` | No | — | Optional override |
| `DOCS_DEFAULT_TEMPLATE_ENGINE` | No | — | Optional override |
| `DOCS_STORAGE_PROVIDER` | No | — | Optional override |
| `DOCS_MAX_DOCUMENT_SIZE_MB` | No | — | Optional override |

Reference vault credentials. Never hardcode secrets.

## Ports

- Default port: `3106`
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_`):

- `np_documents_documents`
- `np_documents_templates`
- `np_documents_versions`
- `np_documents_shares`
- `np_documents_webhook_events`

All tables use `source_account_id` for multi-app isolation where applicable.

## REST API

Public endpoints exposed by the plugin. Internal admin endpoints exist but are not part of the public surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/` | Plugin index / capability list |

Refer to the plugin's OpenAPI spec (under `paid/documents/`) for the full route list.

## Examples

Create a document from a template:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/documents -d '{"template_id":"tpl_xxx","data":{"name":"Acme"}}'
```

List versions:

```bash
curl -H 'Authorization: Bearer $TOKEN' https://api.example.com/documents/doc_xxx/versions
```

Share a document:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/documents/doc_xxx/shares -d '{"expires_in_seconds":3600}'
```


## Source

MIT licensed, source included in this repository: [`free/documents/`](https://github.com/nself-org/plugins/tree/main/free/documents)

## See Also

- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the 7-tier pricing matrix
- `.github/docs/bundles.md` for the public-facing bundle guide
- `plugin.json` in this directory for the canonical manifest
