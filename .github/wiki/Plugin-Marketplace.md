# Plugin Marketplace

The nSelf plugin marketplace is the catalog of free and pro plugins that extend a nSelf backend. It is served by the Cloudflare Worker at `plugins.nself.org` and consumed by three surfaces:

- **Admin UI** — `ɳSelf Admin` runs locally at `http://localhost:3021/plugins/marketplace`
- **CLI** — `nself plugin marketplace {list,search,info}`
- **web/cloud** — `https://cloud.nself.org/marketplace` for users on the managed Cloud tier

This page documents the public API, the filters, and the install flow.

## Endpoints

Base URL: `https://plugins.nself.org`

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/marketplace` | Categorized plugin list with ratings, bundles, and filters |
| GET | `/marketplace?tier=free` | Only free plugins |
| GET | `/marketplace?tier=pro` | Only pro plugins |
| GET | `/marketplace?category=media` | Filter by category |
| GET | `/marketplace?bundle=nclaw` | Filter by bundle slug |
| GET | `/marketplace?q=stripe` | Full-text search over name, description, display name, tags |
| GET | `/marketplace?sort=score\|alpha\|newest\|popular` | Sort order |
| GET | `/marketplace/ratings/:name` | Rating aggregate + review list for one plugin |
| POST | `/marketplace/ratings/:name` | Submit a rating (see Ratings below) |
| POST | `/plugins/:name/install-event` | Record an install count (called by CLI after install) |

Filters combine with AND.

## Rate limits

All marketplace GET endpoints are rate-limited at **60 requests per minute per IP**. The 61st request in a 60-second window returns `429 Too Many Requests` with a `Retry-After: N` header (seconds until the next minute window). Clients should honour this header and not retry before `Retry-After` elapses.

Rating POST endpoints are limited at **5 requests per minute per IP** (anti-spam) and **1 submission per (user, plugin) per 7 days** (dedup). Exceeding either limit returns `429` with `Retry-After`.

## Response schema (`GET /marketplace`)

```json
{
  "version": "1.0.0",
  "fetchedAt": "2026-04-17T12:00:00.000Z",
  "total": 87,
  "categories": [
    { "name": "AI", "slug": "ai", "count": 5, "plugins": [ /* cards */ ] },
    { "name": "Media", "slug": "media", "count": 7, "plugins": [ /* cards */ ] }
  ],
  "plugins": [ /* flat card list for client-side filter */ ],
  "bundles": [
    { "slug": "nclaw", "name": "ɳClaw Bundle", "price": "$0.99/mo", "plugins": ["ai","claw","mux"] }
  ],
  "filters": {
    "tiers": ["free","pro"],
    "categories": ["ai","media","commerce"],
    "bundles": ["nclaw","nchat","ntv"]
  }
}
```

Each plugin card includes:

```json
{
  "name": "ai",
  "displayName": "Ai",
  "version": "1.1.1",
  "description": "Unified AI adapter",
  "tier": "pro",
  "category": "ai",
  "author": "nself",
  "icon": "ai.svg",
  "tags": ["openai","anthropic"],
  "rating": 4.7,
  "ratingCount": 42,
  "downloads": 1234,
  "bundle": "nclaw",
  "bundleName": "ɳClaw Bundle",
  "price": "$0.99/mo",
  "related": ["claw","claw-web","mux","voice"],
  "licenseRequired": true
}
```

## Filters

- `tier` — `free` or `pro`
- `category` — one of the 13 canonical categories in the free registry + `ai`
- `bundle` — one of `nclaw`, `clawde`, `ntv`, `nfamily`, `nchat`
- `q` — free-text search over name, description, display name, and tags

Client-side code (Admin, web/cloud) may combine these with additional client-side filtering. The server always returns a consistent subset even when several filters are set.

## CLI

```bash
# List everything (categorized)
nself plugin marketplace list

# Filter
nself plugin marketplace list --category=media --tier=pro
nself plugin marketplace list --bundle=nclaw

# Search
nself plugin marketplace search stripe
nself plugin marketplace search "video" --tier=pro

# Details
nself plugin marketplace info ai

# Install (requires license for pro)
nself plugin install ai
```

All three subcommands accept `--json` for scripting.

## Admin UI

The Admin marketplace page (`/plugins/marketplace`) uses the enriched payload to:

- Group cards by category with counts
- Show bundle membership + monthly price on each card
- Surface related plugins ("Pairs with ai, claw, mux")
- Show 5-star rating + review count
- One-click install via the local daemon API (which shells out to `nself plugin install`)
- Toast a notification when a newer version is available for an installed plugin

## Ratings

Ratings are stored per-plugin in KV. The GET endpoint returns:

```json
{
  "name": "ai",
  "rating": 4.5,
  "reviewCount": 12,
  "reviews": [
    { "user": "<sha256-hash>", "rating": 5, "comment": "Rock solid.", "createdAt": "2026-04-17T12:00:00.000Z" }
  ]
}
```

The POST endpoint accepts `{ rating: 1-5, comment?: string (max 500 chars), userHash: string }`. `userHash` must be a 64-character lowercase hex string (SHA-256 of the submitter's license key). Submissions are deduplicated: the same `(userHash, plugin)` pair can only submit once per 7 days. The server computes a running average; up to 100 reviews are kept (newest first). The histogram in the web UI is shown only when `reviewCount >= 10`.

The `user` field in stored reviews is the opaque `userHash`. No email, username, or account information is stored.

## Bundles

Five bundles group plugins for a flat monthly price. Installing any plugin in a bundle requires an active subscription to that bundle OR the all-in ɳSelf+ subscription.

| Bundle | Price | Plugins (canonical) |
|--------|-------|---------------------|
| ɳClaw | $0.99/mo | ai, claw, claw-web, mux, voice, browser, google, notify, cron |
| ClawDE | $0.99/mo | claw, ai, realtime, auth, notify, cms, sync |
| nTV | $0.99/mo | media-processing, streaming, epg, tmdb, torrent-manager, content-acquisition |
| nFamily | $0.99/mo | social, photos, activity-feed, moderation, realtime, cms, chat |
| nChat | $0.99/mo | chat, livekit, recording, moderation, bots, realtime, auth |

Bundle membership is canonical in `.claude/docs/sport/F06-BUNDLE-INVENTORY.md` and mirrored here. Update SPORT first; this page follows.

## Environment overrides

For testing against a staging or local Worker:

```bash
# CLI
export NSELF_MARKETPLACE_URL=http://localhost:8787/marketplace
nself plugin marketplace list

# Admin UI
# Adjust the admin API route `MARKETPLACE_URL` or proxy to the staging Worker.
```

## Related

- [[Plugin-Registry]] — the underlying registry format
- [[Plugin-Development]] — building a plugin
- [[Home]]
