# link-preview

URL metadata extraction with Open Graph, Twitter Cards, and oEmbed support. Fetches and caches rich link previews from any URL, with per-channel settings, URL blocklisting, custom preview templates, and click analytics. Designed for use with nself-chat and any messaging application.

## Installation

```bash
nself plugin install link-preview
```

## Features

- Open Graph metadata extraction (title, description, image, site name, video, audio)
- Twitter Card tag extraction as a fallback to Open Graph
- oEmbed support with built-in providers plus custom database-configured providers
- oEmbed provider discovery for any URL
- PostgreSQL-backed preview cache with configurable TTL (default 7 days)
- Batch URL fetching — up to 50 URLs in a single request
- Per-channel and global settings (auto-expand, image/video visibility, max previews per message)
- URL blocklist supporting exact match, domain match, and regex patterns
- Custom preview templates with regex URL matching and priority ordering
- Click-through tracking and analytics with daily view/click counts
- Popular links reporting with click-through rate calculation
- Estimated reading time based on word count (200 words/minute)
- Language and favicon detection
- Admin endpoints for cache clearing, cleanup of expired entries, and stats
- Multi-app isolation via `source_account_id`
- API key authentication and rate limiting

## Configuration

| Name | Required | Default | Description |
| ---- | -------- | ------- | ----------- |
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `LP_PLUGIN_PORT` | No | `3718` | HTTP server port |
| `LINK_PREVIEW_ENABLED` | No | `true` | Enable or disable the plugin |
| `LINK_PREVIEW_CACHE_TTL_HOURS` | No | `168` | Preview cache lifetime in hours (7 days) |
| `LINK_PREVIEW_TIMEOUT_SECONDS` | No | `10` | Fetch timeout per URL (1–60 seconds) |
| `LINK_PREVIEW_USER_AGENT` | No | `nself-bot/1.0` | User-Agent header sent when fetching URLs |
| `LINK_PREVIEW_MAX_RESPONSE_SIZE_MB` | No | `10` | Maximum response body size to accept |
| `LINK_PREVIEW_FOLLOW_REDIRECTS` | No | `true` | Follow HTTP redirects |
| `LINK_PREVIEW_MAX_REDIRECTS` | No | `5` | Maximum redirect hops to follow |
| `LINK_PREVIEW_RESPECT_ROBOTS_TXT` | No | `true` | Respect robots.txt crawl directives |
| `OEMBED_ENABLED` | No | `true` | Enable oEmbed lookups |
| `OEMBED_DISCOVERY` | No | `true` | Enable oEmbed endpoint discovery |
| `OEMBED_MAX_WIDTH` | No | `1024` | Maximum embed width in pixels |
| `OEMBED_MAX_HEIGHT` | No | `768` | Maximum embed height in pixels |
| `LINK_PREVIEW_SAFETY_CHECK` | No | `true` | Enable URL safety check |
| `LINK_PREVIEW_PHISHING_DETECTION` | No | `true` | Enable phishing detection |
| `LINK_PREVIEW_RATE_LIMIT_PER_MINUTE` | No | `60` | Outbound fetch rate limit per minute |
| `LINK_PREVIEW_RATE_LIMIT_PER_DOMAIN` | No | `10` | Outbound fetch rate limit per domain |
| `LP_API_KEY` | No | — | API key required on all requests (if set) |
| `LP_RATE_LIMIT_MAX` | No | `200` | Inbound API rate limit — requests per window |
| `LP_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

## API Reference

### Health and Status

#### GET /health

Returns `{ status: "ok", plugin: "link-preview", timestamp }`. No authentication required.

#### GET /ready

Returns `{ ready: true }` when the database is reachable, or `503` when it is not.

#### GET /live

Returns uptime, memory usage, and cache statistics.

#### GET /v1/status

Returns plugin version, enabled state, and cache statistics.

### Preview Fetching

#### GET /api/link-preview?url={url}

Returns a cached preview if one exists. If the URL is not cached, stores a partial record and returns it. If the URL is blocked, returns `403`.

```bash
curl "http://localhost:3718/api/link-preview?url=https://example.com"
```

Response:

```json
{
  "id": "uuid",
  "url": "https://example.com",
  "title": "Example Domain",
  "description": "This domain is for use in illustrative examples.",
  "image_url": null,
  "site_name": null,
  "favicon_url": "https://example.com/favicon.ico",
  "language": "en",
  "status": "success",
  "created_at": "2026-02-21T00:00:00Z"
}
```

#### POST /api/link-preview/fetch

Fetches live metadata from the URL, stores the result, and returns it. Use `force: true` to bypass the cache.

Request body:

```json
{
  "url": "https://example.com",
  "force": false
}
```

#### GET /api/link-preview/:id

Returns a stored preview by ID. Returns `404` if not found.

#### DELETE /api/link-preview/:id

Deletes a cached preview by ID.

#### POST /api/link-preview/refresh/:id

Re-fetches metadata for an existing preview from its original URL and updates the stored record.

### Batch Operations

#### POST /api/link-preview/batch

Fetches up to 50 URLs in one request. Checks blocklist and cache for each.

Request body:

```json
{
  "urls": ["https://example.com", "https://github.com"]
}
```

Response:

```json
{
  "data": [
    { "url": "https://example.com", "blocked": false, "preview": {} },
    { "url": "https://github.com", "blocked": false, "preview": {} }
  ],
  "total": 2
}
```

#### GET /api/link-preview/message/:messageId

Returns all previews associated with a specific chat message ID, ordered by creation time.

### Templates

Custom templates let you override default preview rendering for specific URLs.

#### GET /api/link-preview/templates

Lists all templates, ordered by priority descending. Query parameters: `limit` (default 100), `offset` (default 0).

#### POST /api/link-preview/templates

Creates a new template. `name`, `url_pattern`, and `template_html` are required.

Request body:

```json
{
  "name": "GitHub PR Template",
  "description": "Enhanced preview for GitHub pull requests",
  "url_pattern": "https://github\\.com/.+/pull/\\d+",
  "priority": 10,
  "template_html": "<div class=\"pr-preview\">...</div>",
  "css_styles": ".pr-preview { ... }"
}
```

#### GET /api/link-preview/templates/:id

Returns a single template by ID.

#### PUT /api/link-preview/templates/:id

Updates a template. All fields are optional.

#### DELETE /api/link-preview/templates/:id

Deletes a template.

#### POST /api/link-preview/templates/:id/test

Tests whether a template's URL pattern matches a given URL.

Request body:

```json
{ "url": "https://github.com/org/repo/pull/123" }
```

Response:

```json
{ "matches": true, "template_id": "uuid", "url": "...", "pattern": "..." }
```

### oEmbed

#### GET /api/link-preview/oembed/providers

Lists all oEmbed providers — both built-in and custom database-configured providers.

#### POST /api/link-preview/oembed/providers

Registers a custom oEmbed provider. `provider_name`, `provider_url`, `endpoint_url`, and `url_schemes` are required.

Request body:

```json
{
  "provider_name": "My Video Service",
  "provider_url": "https://myvideo.example",
  "endpoint_url": "https://myvideo.example/oembed",
  "url_schemes": ["https://myvideo.example/watch/*"]
}
```

#### GET /api/link-preview/oembed/discover?url={url}

Checks whether a URL has a known oEmbed provider (built-in first, then database-configured).

#### GET /api/link-preview/oembed/fetch?url={url}&maxwidth={w}&maxheight={h}

Fetches the oEmbed response for a URL and returns embed data.

### Blocklist

#### GET /api/link-preview/blocklist

Lists all blocked URL patterns. Query parameters: `limit`, `offset`.

#### POST /api/link-preview/blocklist

Adds a URL pattern to the blocklist. `url_pattern`, `pattern_type`, and `reason` are required.

`pattern_type` values: `exact`, `domain`, `regex`

`reason` values: `spam`, `phishing`, `malware`, `offensive`, `other`

Request body:

```json
{
  "url_pattern": "malware-site.example",
  "pattern_type": "domain",
  "reason": "malware",
  "description": "Known malware distribution site",
  "expires_at": "2027-01-01T00:00:00Z"
}
```

#### DELETE /api/link-preview/blocklist/:id

Removes a blocklist entry by ID.

#### POST /api/link-preview/blocklist/check

Checks whether a URL matches any blocklist entry.

Request body: `{ "url": "https://example.com" }`

Response: `{ "url": "...", "blocked": false }`

### Settings

Preview behavior is configurable globally and per-channel.

#### GET /api/link-preview/settings

Returns settings for a scope. Query parameters: `scope` (`global` or `channel`), `scope_id`.

#### PUT /api/link-preview/settings

Updates settings for a scope. `scope` is required.

Request body:

```json
{
  "scope": "global",
  "enabled": true,
  "auto_expand": false,
  "show_images": true,
  "show_videos": true,
  "max_previews_per_message": 3,
  "preview_position": "bottom",
  "blocked_domains": [],
  "allowed_domains": []
}
```

#### GET /api/link-preview/settings/channel/:id

Returns settings for a specific channel.

#### PUT /api/link-preview/settings/channel/:id

Updates settings for a specific channel.

### Analytics

#### GET /api/link-preview/analytics

Returns analytics for a date range. Query parameters: `start_date`, `end_date`, `preview_id`. Defaults to the last 30 days.

#### GET /api/link-preview/popular

Returns the most-shared links ranked by usage count, with click-through rate. Query parameter: `limit` (default 20).

#### POST /api/link-preview/click/:usageId

Records a click event on a usage record.

#### POST /api/link-preview/usage

Tracks a preview being shown in a message. `preview_id` is required.

Request body:

```json
{
  "preview_id": "uuid",
  "message_id": "msg-123",
  "user_id": "user-456",
  "channel_id": "channel-789"
}
```

### Admin

#### POST /api/link-preview/admin/cache/clear

Clears all cached previews for the current source account.

#### GET /api/link-preview/admin/stats

Returns cache statistics: total previews, successful, failed, expired, average fetch duration, oEmbed count, unique sites.

#### POST /api/link-preview/admin/cleanup

Deletes all expired previews (where `cache_expires_at < NOW()`).

## CLI Commands

```bash
# Initialize database schema
nself-link-preview init

# Start the HTTP server
nself-link-preview server --port 3718 --host 0.0.0.0

# Show cache statistics
nself-link-preview status
nself-link-preview stats

# Fetch a preview for a URL
nself-link-preview fetch https://example.com
nself-link-preview fetch https://example.com --force

# Refresh an existing preview
nself-link-preview refresh <preview-id>

# Delete a cached preview
nself-link-preview delete <preview-id>

# Template management
nself-link-preview template list --limit 20
nself-link-preview template test <template-id> <url>
nself-link-preview template delete <template-id>

# oEmbed
nself-link-preview oembed providers
nself-link-preview oembed discover https://www.youtube.com/watch?v=abc

# Blocklist management
nself-link-preview block https://spam.example --type domain --reason spam
nself-link-preview block "malware.*\.example" --type regex --reason malware
nself-link-preview unblock <blocklist-entry-id>
nself-link-preview blocklist --limit 20
nself-link-preview check https://example.com

# Analytics
nself-link-preview popular --limit 10

# Cache maintenance
nself-link-preview cache clear
nself-link-preview cleanup
```

## Database Tables

| Table | Purpose |
| ----- | ------- |
| `lp_link_previews` | Cached preview metadata: title, description, image, video, audio, favicon, reading time, oEmbed data, fetch status |
| `lp_link_preview_usage` | Per-message usage tracking: which preview appeared in which message, click events |
| `lp_preview_templates` | Custom rendering templates matched by regex URL pattern |
| `lp_oembed_providers` | Custom oEmbed provider registry with URL schemes and endpoint URLs |
| `lp_url_blocklist` | Blocked URL patterns with type (exact, domain, regex) and optional expiry |
| `lp_preview_settings` | Per-scope settings (global or per-channel) for display preferences |
| `lp_preview_analytics` | Daily aggregated view and click counts per preview |

All tables include `source_account_id` for multi-app isolation.

## Usage Examples

### Fetch a preview when a user posts a link

```typescript
const response = await fetch('http://localhost:3718/api/link-preview/fetch', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ url: 'https://github.com/nself-org/cli' }),
});
const preview = await response.json();
// preview.title, preview.description, preview.image_url, etc.
```

### Track when a preview appears in a message

```typescript
await fetch('http://localhost:3718/api/link-preview/usage', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    preview_id: preview.id,
    message_id: 'msg-123',
    user_id: 'user-456',
    channel_id: 'general',
  }),
});
```

### Block an entire domain

```typescript
await fetch('http://localhost:3718/api/link-preview/blocklist', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    url_pattern: 'spam-domain.example',
    pattern_type: 'domain',
    reason: 'spam',
  }),
});
```

### Configure a channel to disable image previews

```typescript
await fetch('http://localhost:3718/api/link-preview/settings/channel/channel-id-here', {
  method: 'PUT',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ show_images: false }),
});
```

## Integration

This plugin is used by **nself-chat** to generate rich link previews for messages. It integrates with chat's message pipeline to detect URLs in outgoing messages, fetch metadata asynchronously, and surface click analytics in the admin dashboard.

The plugin works with any nself application that processes user-submitted URLs. It can also be used standalone — for example, as a server-side rendering helper for a content feed.

## Changelog

### v1.0.0

- Initial release
- Open Graph and Twitter Card extraction
- oEmbed support with built-in and custom providers
- PostgreSQL caching with configurable TTL
- Per-channel settings
- URL blocklist (exact, domain, regex)
- Custom preview templates
- Click tracking and analytics
- Batch URL fetching (up to 50 URLs)
- Multi-app isolation via `source_account_id`
- API key authentication and rate limiting
