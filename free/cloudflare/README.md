# Cloudflare Plugin

> Cloudflare zone, DNS, R2, cache, and analytics management. **Pro plugin — requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Any bundle | $0.99/mo | $9.99/yr | If in bundle |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes |

**Minimum tier:** Basic (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

Not currently part of a named bundle (unbundled per F04-PLUGIN-INVENTORY). Common companion plugin for any nSelf deployment fronted by Cloudflare DNS or R2. Available via any per-bundle subscription or ɳSelf+.

Or get all bundles + all apps via **ɳSelf+** ($3.99/mo or $39.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install cloudflare
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

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

Refer to the plugin's OpenAPI spec (under `paid/cloudflare/`) for the full route list.

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

Source-available (license required to run): [`plugins-pro/paid/cloudflare/`](https://github.com/nself-org/plugins-pro/tree/main/paid/cloudflare)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the pricing matrix
- `.github/docs/bundles.md` for the public-facing bundle guide
- `plugin.json` in this directory for the canonical manifest
