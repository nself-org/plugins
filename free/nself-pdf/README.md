# nself-pdf

Server-side PDF generation from HTML templates (Handlebars/Nunjucks) with MinIO output and Hasura Action trigger

> **Status: planned.** This plugin is specced (see `plugin.json`) but not yet implemented.

## Details

- **Category:** content
- **Tier:** pro
- **Language:** go
- **Port:** 3851
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `PDF_GENERATION_PORT` | — | — |
| `PDF_GENERATION_MINIO_ENDPOINT` | — | — |
| `PDF_GENERATION_MINIO_ACCESS_KEY` | — | — |
| `PDF_GENERATION_MINIO_SECRET_KEY` | — | — |
| `PDF_GENERATION_TEMPLATES_BUCKET` | — | — |
| `PDF_GENERATION_OUTPUT_BUCKET` | — | — |
| `PDF_GENERATION_SHARED_SECRET` | — | — |
| `PDF_GENERATION_MAX_CONCURRENT` | — | — |
| `PDF_GENERATION_TIMEOUT_SECONDS` | — | — |
| `PDF_AUDIT_ENABLED` | — | — |

## Dependencies

Requires: `minio` · Optional: `database`

## Install

```bash
nself plugin install nself-pdf
```
