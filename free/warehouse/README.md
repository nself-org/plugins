# warehouse

Streams nSelf Postgres (`np_*` tables) to ClickHouse, BigQuery, or Snowflake.

## Requirements

- nSelf CLI v1.0.9+
- `cdc` plugin installed and running (peer dependency)
- Free (MIT), no license required

## Install

```bash
nself plugin install cdc
nself plugin install warehouse
```

## Configuration

| Env var | Required | Default | Description |
|---|---|---|---|
| `WAREHOUSE_DRIVER` | Yes | — | `clickhouse`, `bigquery`, or `snowflake` |
| `WAREHOUSE_CLICKHOUSE_DSN` | If driver=clickhouse | — | `clickhouse://user:pass@host:9000/db` |
| `WAREHOUSE_BQ_PROJECT` | If driver=bigquery | — | GCP project ID |
| `WAREHOUSE_BQ_DATASET` | If driver=bigquery | — | BigQuery dataset name |
| `WAREHOUSE_BQ_CREDENTIALS_JSON` | If driver=bigquery | — | Service account JSON (raw string) |
| `WAREHOUSE_SNOWFLAKE_DSN` | If driver=snowflake | — | Snowflake DSN |
| `WAREHOUSE_BATCH_SIZE` | No | 1000 | Rows per flush |
| `WAREHOUSE_FLUSH_INTERVAL` | No | 30s | Max time between flushes |
| `WAREHOUSE_TABLES` | No | ALL | Comma-separated `np_*` table names to export |

## API

All endpoints require `x-hasura-role: admin`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/warehouse/status` | Lag, row counts, last export time per table |
| `POST` | `/warehouse/backfill` | `{"table":"np_foo","since":"2024-01-01T00:00:00Z"}` |
| `GET` | `/warehouse/schema` | Current mirror schema per target |
| `POST` | `/warehouse/pause` | Pause CDC streaming |
| `POST` | `/warehouse/resume` | Resume CDC streaming |

## Backfill

```bash
curl -X POST http://localhost:8210/warehouse/backfill \
  -H 'x-hasura-role: admin' \
  -d '{"table":"np_messages"}'
```

Backfill runs asynchronously. Progress is visible in `/warehouse/status` row counts
and in `np_warehouse_watermarks`.
