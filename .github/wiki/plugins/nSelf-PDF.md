# nSelf PDF Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | planned ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install nself-pdf` |
| **Requirements** | nSelf >=1.1.0 |

<!-- END GENERATED:catalog -->

> Server-side PDF generation from HTML templates (Handlebars/Nunjucks) with MinIO output and Hasura Action trigger.

## Install

```bash
nself plugin install nself-pdf
```

No license key required.

## Description

Server-side PDF generation from HTML templates (Handlebars/Nunjucks) with MinIO output and Hasura Action trigger

Category: `documents`. Current version: `0.1.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `PDF_GENERATION_PORT` | `3851` | - |
| `PDF_GENERATION_MINIO_ENDPOINT` | `-` | - |
| `PDF_GENERATION_MINIO_ACCESS_KEY` | `-` | - |
| `PDF_GENERATION_MINIO_SECRET_KEY` | `-` | - |
| `PDF_GENERATION_TEMPLATES_BUCKET` | `-` | - |
| `PDF_GENERATION_OUTPUT_BUCKET` | `-` | - |
| `PDF_GENERATION_SHARED_SECRET` | `-` | - |
| `PDF_GENERATION_MAX_CONCURRENT` | `-` | - |
| `PDF_GENERATION_TIMEOUT_SECONDS` | `-` | - |
| `PDF_AUDIT_ENABLED` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3851 | nSelf PDF service port |

## Examples

```bash
nself plugin install nself-pdf
```

## Source

[`plugins/nself-pdf/`](https://github.com/nself-org/plugins/tree/main/nself-pdf)

Manifest: [`plugins/nself-pdf/plugin.json`](https://github.com/nself-org/plugins/tree/main/nself-pdf/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
