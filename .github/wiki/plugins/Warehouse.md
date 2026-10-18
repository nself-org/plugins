# Warehouse Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install warehouse` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> Data warehouse sync, exports nself table data to BigQuery, Snowflake, or Redshift on a configurable schedule.

## Install

```bash
nself plugin install warehouse
```

No license key required.

## Description

Data warehouse sync, exports nself table data to BigQuery, Snowflake, or Redshift on a configurable schedule.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `WAREHOUSE_PORT` | `8210` | - |
| `WAREHOUSE_DESTINATION` | `-` | - |
| `WAREHOUSE_BQ_PROJECT` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 8210 | Warehouse service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_warehouse_sync_jobs`
- `np_warehouse_tables`

## REST API

```
GET    /health
```

## Examples

### Health check

```bash
curl http://localhost:8210/health
```

## Source

[`plugins/warehouse/`](https://github.com/nself-org/plugins/tree/main/warehouse)

Manifest: [`plugins/warehouse/plugin.json`](https://github.com/nself-org/plugins/tree/main/warehouse/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
