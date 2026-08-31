# ROM Discovery Plugin

> ROM metadata database, search, discovery, automated download orchestration, and multi-source scraping for nTV. **Pro plugin — requires license.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Any bundle | $0.99/mo | $9.99/yr | If in bundle |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes |

**Minimum tier:** Basic (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

Not currently included in any of the five product bundles (ɳClaw, ClawDE, nTV, nFamily, nChat). Pairs with `retro-gaming` to deliver a complete ROM library with metadata and on-demand download.

Or get all bundles + all apps via **ɳSelf+** ($49.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install rom-discovery
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

ROM metadata database and download orchestration. The plugin maintains a catalog of ROMs keyed by platform, region, and hash, tracks popularity scores from external sources, and orchestrates downloads through an internal queue. It complements `retro-gaming` by filling the library side of the nTV gaming experience.

Scrapers run on a configurable cron schedule (opt-in) and pull metadata from archive.org and related sources. Quality and popularity filters prevent the queue from flooding with low-value items. Downloads are rate-limited by count and aggregate size, and respect a user-configurable concurrency cap.

The plugin records every scraper run in a job history table so operators can inspect what was fetched, when, and at what cost. Cancelling a queued download removes the item without affecting items already in flight, and rejected items leave behind a reason so the next scraper run can skip them.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Main Postgres connection |
| `ROM_DISCOVERY_PLUGIN_PORT` | No | `3034` | Service port |
| `ROM_DISCOVERY_ENABLE_SCRAPERS` | No | `false` | Opt-in scraper activation |
| `ROM_DISCOVERY_SCRAPER_SCHEDULE` | No | `0 3 * * *` | Cron schedule for scraper runs |
| `ROM_DISCOVERY_DEFAULT_QUALITY_MIN` | No | `50` | Minimum quality score to accept |
| `ROM_DISCOVERY_DEFAULT_POPULARITY_MIN` | No | `0` | Minimum popularity score |
| `ROM_DISCOVERY_MAX_CONCURRENT_DOWNLOADS` | No | `3` | Parallel download cap |
| `ROM_DISCOVERY_MAX_DOWNLOAD_SIZE_MB` | No | `2048` | Per-download size cap |
| `ROM_DISCOVERY_RETRO_GAMING_URL` | No | — | Base URL of the companion `retro-gaming` plugin |
| `ROM_DISCOVERY_CDN_URL` | No | — | Optional CDN for downloads |
| `LOG_LEVEL` | No | `info` | Log verbosity |

Reference vault credentials. Never hardcode secrets.

## Ports

- Default port: `3125`
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_romdisc_`):

- `np_romdisc_metadata` — canonical ROM metadata (title, platform, region, hash)
- `np_romdisc_download_queue` — pending and in-flight downloads
- `np_romdisc_scraper_jobs` — scraper run history with success/failure reason
- `np_romdisc_popularity` — popularity scores aggregated from external sources

All tables use `source_account_id` for multi-app isolation.

## REST API

Public endpoints. Internal admin routes are excluded from this surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/search` | Search ROM metadata by title, platform, tag |
| GET | `/platforms` | List supported platforms with ROM counts |
| GET | `/scrapers` | List scraper job history |
| POST | `/scrapers/run` | Trigger a scraper job manually |
| GET | `/downloads` | Inspect the download queue |
| POST | `/downloads` | Enqueue a ROM download |
| DELETE | `/downloads/:id` | Cancel a queued download |

## Examples

Search for a title:

```bash
curl -H 'Authorization: Bearer $TOKEN' \
  'https://api.example.com/rom-discovery/search?title=Contra&platform=NES'
```

List supported platforms with counts:

```bash
curl -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/rom-discovery/platforms
```

Enqueue a download:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/rom-discovery/downloads \
  -d '{"metadata_id":"meta_xxx"}'
```

Trigger a scraper run on demand:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/rom-discovery/scrapers/run \
  -d '{"source":"archive.org","platform":"NES"}'
```

Cancel a queued download:

```bash
curl -X DELETE -H 'Authorization: Bearer $TOKEN' \
  https://api.example.com/rom-discovery/downloads/dl_xxx
```

## Operational Notes

Scrapers are **disabled by default** (`ROM_DISCOVERY_ENABLE_SCRAPERS=false`). Operators must explicitly opt in. When enabled, the scraper schedule defaults to `0 3 * * *` (daily at 03:00 server time). Each scraper run is recorded in `np_romdisc_scraper_jobs` with start time, end time, items fetched, items rejected, and any error reason.

Quality and popularity floors apply at enqueue time, not at scrape time, so changes to the thresholds take effect on the next download attempt without requiring a re-scrape. Items rejected by the floors stay in the metadata table and can be promoted later by raising the floor or manually enqueueing a download.

The companion `retro-gaming` plugin is reachable at `ROM_DISCOVERY_RETRO_GAMING_URL` so completed downloads can be registered as ROM library entries automatically. Without that URL, downloaded ROMs land in object storage and must be linked into the library by hand.

## Source

Source-available (license required to run): [`plugins-pro/paid/rom-discovery/`](https://github.com/nself-org/plugins-pro/tree/main/paid/rom-discovery)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- [[plugin-retro-gaming]] — companion plugin for ROM library, save states, play sessions, controllers
- [[plugin-game-metadata]] — IGDB-backed metadata enrichment service
- [[plugin-tmdb]] — broader media metadata for nTV libraries
- [[plugin-cron]] — scheduled job runner used by the scraper schedule
- [[Pricing]] — tier comparison
- [[Plugins]] — full plugin index

← [[Plugins]] | [[Home]] →
