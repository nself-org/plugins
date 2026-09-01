# Tmdb Plugin

> Media metadata enrichment from TMDB, IMDb, TVDB, and MusicBrainz with auto-matching, a manual review queue, and multi-provider support. **Free MIT plugin.**

## Tier required

| Tier | Monthly | Annual | Includes this plugin? |
|------|---------|--------|----------------------|
| Free | $0 | $0 | No |
| Any bundle | $0.99/mo | $9.99/yr | If in bundle |
| ɳSelf+ | $3.99/mo | $39.99/yr | Yes |

**Minimum tier:** Basic (this is a `tier: pro` plugin per F07-PRICING-TIERS).

## Bundle membership

**nTV** ($0.99/mo) bundle, see `.github/docs/licensing/bundles.md`. Also via tier subscription.

Or get all bundles + all apps via **ɳSelf+** ($49.99/yr).

## Install

```bash
nself license set nself_pro_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
nself plugin install tmdb
nself build
```

The license is validated against `ping.nself.org/license/validate`. Tier is checked server-side; insufficient tier returns an error.

## Description

Media metadata enrichment from TMDB, IMDb, TVDB, and MusicBrainz with auto-matching, a manual review queue, and multi-provider support.

Auto-match movies, TV shows, seasons, and episodes against TMDB / OMDB / TVDB / MusicBrainz. A confidence threshold drives auto-accept versus a manual review queue. Multi-language metadata, genres, and artwork ship in one query.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Required env var |
| `TMDB_API_KEY` | Yes | — | Required env var |
| `TMDB_PLUGIN_PORT` | No | — | Optional override |
| `TMDB_APP_IDS` | No | — | Optional override |
| `TMDB_API_READ_ACCESS_TOKEN` | No | — | Optional override |
| `OMDB_API_KEY` | No | — | Optional override |
| `TVDB_API_KEY` | No | — | Optional override |
| `MUSICBRAINZ_USER_AGENT` | No | — | Optional override |
| `TMDB_AUTO_ACCEPT_THRESHOLD` | No | — | Optional override |
| `TMDB_DEFAULT_LANGUAGE` | No | — | Optional override |
| `TMDB_CACHE_TTL_DAYS` | No | — | Optional override |
| `TMDB_REFRESH_CRON` | No | — | Optional override |

Reference vault credentials. Never hardcode secrets.

## Ports

- Default port: `3122`
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_`):

- `np_tmdb_movies`
- `np_tmdb_tv_shows`
- `np_tmdb_tv_seasons`
- `np_tmdb_tv_episodes`
- `np_tmdb_genres`
- `np_tmdb_match_queue`
- `np_tmdb_webhook_events`

All tables use `source_account_id` for multi-app isolation where applicable.

## REST API

Public endpoints exposed by the plugin. Internal admin endpoints exist but are not part of the public surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/` | Plugin index / capability list |

Refer to the plugin's OpenAPI spec (under `paid/tmdb/`) for the full route list.

## Examples

Look up a movie by title:

```bash
curl -H 'Authorization: Bearer $TOKEN' 'https://api.example.com/tmdb/search?query=Dune+2021'
```

Approve a match:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/tmdb/match-queue/queue_xxx/approve
```

Get artwork URLs for a TV show:

```bash
curl -H 'Authorization: Bearer $TOKEN' https://api.example.com/tmdb/tv/12345/artwork
```


## Source

Source (MIT, no license required): [`plugins-pro/paid/tmdb/`](https://github.com/nself-org/plugins-pro/tree/main/paid/tmdb)

Note: `plugins-pro` is a private repository. Source access is granted to ɳSelf+ subscribers and Enterprise customers.

## See Also

- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the 7-tier pricing matrix
- `.github/docs/bundles.md` for the public-facing bundle guide
- `plugin.json` in this directory for the canonical manifest
