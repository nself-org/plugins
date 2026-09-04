# Cloudflare Plugin

> Cloudflare zone, DNS, R2, cache, and analytics management.

**Tier:** Free (MIT) — no license required.

## Install

```bash
nself plugin install cloudflare
nself build
```

## Description

Cloudflare zone, DNS, R2, cache, and analytics management.

Manage zones, DNS records, R2 buckets, cache purges, and analytics ingestion through one endpoint. The plugin caches read-heavy responses and proxies writes directly to Cloudflare's API, the `CF_API_TOKEN` is held only on the server side.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `CF_API_TOKEN` | Yes | — | Required env var |
| `DATABASE_URL` | Yes | — | Required env var |
| `CF_PLUGIN_PORT` | No | — | Optional override |
| `CF_PLUGIN_HOST` | No | — | Optional override |
| `CF_LOG_LEVEL` | No | — | Optional override |
| `CF_APP_IDS` | No | — | Optional override |
| `CF_ZONE_IDS` | No | — | Optional override |
| `CF_R2_ACCESS_KEY` | No | — | Optional override |
| `CF_R2_SECRET_KEY` | No | — | Optional override |
| `CF_ACCOUNT_ID` | No | — | Optional override |
| `CF_SYNC_INTERVAL` | No | — | Optional override |

Reference vault credentials. Never hardcode secrets.

## Ports

- Default port: `3024`
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_`):

- `np_cf_zones`
- `np_cf_dns_records`
- `np_cf_r2_buckets`
- `np_cf_cache_purge_log`
- `np_cf_analytics`
- `np_cf_webhook_events`

All tables use `source_account_id` for multi-app isolation where applicable.

## REST API

Public endpoints exposed by the plugin. Internal admin endpoints exist but are not part of the public surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/` | Plugin index / capability list |

Refer to the plugin's OpenAPI spec (under `free/cloudflare/`) for the full route list.

## Examples

List zones:

```bash
curl -H 'Authorization: Bearer $TOKEN' https://api.example.com/cloudflare/zones
```

Add a DNS record:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/cloudflare/zones/zone_xxx/records -d '{"type":"A","name":"app","content":"1.2.3.4"}'
```

Purge cache for a path:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/cloudflare/zones/zone_xxx/cache/purge -d '{"files":["https://example.com/style.css"]}'
```


## Source

MIT licensed, source included in this repository: [`free/cloudflare/`](https://github.com/nself-org/plugins/tree/main/free/cloudflare)

## See Also

- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the pricing matrix
- `.github/docs/bundles.md` for the public-facing bundle guide
- `plugin.json` in this directory for the canonical manifest
