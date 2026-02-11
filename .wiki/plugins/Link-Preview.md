# Link Preview Plugin

**Version:** 1.0.0
**Category:** Content Enhancement
**Port:** 3718
**Multi-App Support:** Yes (isolated by `source_account_id`)

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Configuration](#configuration)
4. [CLI Commands](#cli-commands)
5. [REST API](#rest-api)
6. [Webhook Events](#webhook-events)
7. [Database Schema](#database-schema)
8. [Examples](#examples)
9. [Troubleshooting](#troubleshooting)

---

## Overview

The **Link Preview Plugin** provides URL metadata extraction with Open Graph, Twitter Cards, and oEmbed support. It enables automatic link preview generation, custom preview templates, URL blocklist management, and comprehensive analytics tracking.

### Key Features

- **Metadata Extraction**: Automatic parsing of Open Graph, Twitter Cards, and HTML meta tags
- **oEmbed Support**: Native support for oEmbed providers (YouTube, Vimeo, Twitter, etc.)
- **Custom Templates**: Create custom preview templates with regex URL patterns
- **Intelligent Caching**: Configurable TTL-based caching with automatic expiration
- **URL Blocklist**: Block URLs by exact match, domain, or regex pattern
- **Safety Checks**: Built-in phishing detection and safety validation
- **Usage Analytics**: Track views, clicks, and click-through rates
- **Per-Channel Settings**: Customize preview behavior per channel or user
- **Rate Limiting**: Domain-level and global rate limiting
- **Multi-App Isolation**: Full data isolation per `source_account_id`

### Use Cases

1. **Chat Applications**: Automatic link previews in messages
2. **Social Platforms**: Rich media embeds for shared URLs
3. **Content Management**: Preview generation for external links
4. **Security Monitoring**: Block malicious or inappropriate URLs
5. **Analytics**: Track popular links and engagement metrics

---

## Quick Start

### Installation

```bash
# Install the link-preview plugin
nself plugin install link-preview

# Initialize the database
nself-link-preview init

# Start the server
nself-link-preview server
```

### Basic Usage

```bash
# Fetch a link preview
nself-link-preview fetch https://example.com

# Check cache statistics
nself-link-preview status

# Block a malicious URL
nself-link-preview block https://spam.com --type domain --reason spam

# View popular links
nself-link-preview popular --limit 10
```

### Test the Server

```bash
# Health check
curl http://localhost:3718/health

# Fetch a preview via API
curl "http://localhost:3718/api/link-preview?url=https://example.com"

# Get cache stats
curl http://localhost:3718/api/link-preview/admin/stats
```

---

## Configuration

### Environment Variables

All configuration is via environment variables. Below is the **complete** reference with **accurate defaults** from `config.ts`:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| **Server** | | | |
| `LP_PLUGIN_PORT` | No | `3718` | HTTP server port |
| `LP_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server host |
| **Database** | | | |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | Database name |
| `POSTGRES_USER` | No | `postgres` | Database user |
| `POSTGRES_PASSWORD` | **Yes*** | `''` | Database password (*required in production) |
| `POSTGRES_SSL` | No | `false` | Enable SSL for database connection |
| **Preview Settings** | | | |
| `LINK_PREVIEW_ENABLED` | No | `true` | Enable/disable link preview fetching |
| `LINK_PREVIEW_CACHE_TTL_HOURS` | No | `168` | Cache TTL in hours (7 days) |
| `LINK_PREVIEW_TIMEOUT_SECONDS` | No | `10` | HTTP request timeout in seconds |
| `LINK_PREVIEW_USER_AGENT` | No | `nself-bot/1.0` | User agent for HTTP requests |
| `LINK_PREVIEW_MAX_PER_MESSAGE` | No | `3` | Maximum previews per message |
| **Fetching** | | | |
| `LINK_PREVIEW_MAX_RESPONSE_SIZE_MB` | No | `10` | Maximum response size in MB |
| `LINK_PREVIEW_FOLLOW_REDIRECTS` | No | `true` | Follow HTTP redirects |
| `LINK_PREVIEW_MAX_REDIRECTS` | No | `5` | Maximum redirects to follow |
| `LINK_PREVIEW_RESPECT_ROBOTS_TXT` | No | `true` | Respect robots.txt directives |
| **oEmbed** | | | |
| `OEMBED_ENABLED` | No | `true` | Enable oEmbed support |
| `OEMBED_DISCOVERY` | No | `true` | Enable oEmbed auto-discovery |
| `OEMBED_MAX_WIDTH` | No | `1024` | Maximum embed width in pixels |
| `OEMBED_MAX_HEIGHT` | No | `768` | Maximum embed height in pixels |
| **Safety** | | | |
| `LINK_PREVIEW_SAFETY_CHECK` | No | `true` | Enable safety checks |
| `LINK_PREVIEW_PHISHING_DETECTION` | No | `true` | Enable phishing detection |
| **Rate Limiting (Domain-Level)** | | | |
| `LINK_PREVIEW_RATE_LIMIT_PER_MINUTE` | No | `60` | Requests per minute (global) |
| `LINK_PREVIEW_RATE_LIMIT_PER_DOMAIN` | No | `10` | Requests per domain per minute |
| **Security (API-Level)** | | | |
| `LP_API_KEY` | No | `undefined` | API key for authentication (optional) |
| `LP_RATE_LIMIT_MAX` | No | `200` | Max API requests per window |
| `LP_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds (1 minute) |
| **Logging** | | | |
| `LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |

### Configuration Validation

The following validations are enforced:

- `POSTGRES_PASSWORD` must be set in production
- `LINK_PREVIEW_CACHE_TTL_HOURS` must be at least 1
- `LINK_PREVIEW_TIMEOUT_SECONDS` must be between 1 and 60

### Example `.env` File

```bash
# Server
LP_PLUGIN_PORT=3718
LP_PLUGIN_HOST=0.0.0.0

# Database
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your_secure_password

# Preview Settings
LINK_PREVIEW_ENABLED=true
LINK_PREVIEW_CACHE_TTL_HOURS=168  # 7 days
LINK_PREVIEW_TIMEOUT_SECONDS=10
LINK_PREVIEW_MAX_PER_MESSAGE=3

# Fetching
LINK_PREVIEW_MAX_RESPONSE_SIZE_MB=10
LINK_PREVIEW_FOLLOW_REDIRECTS=true
LINK_PREVIEW_MAX_REDIRECTS=5

# oEmbed
OEMBED_ENABLED=true
OEMBED_DISCOVERY=true
OEMBED_MAX_WIDTH=1024
OEMBED_MAX_HEIGHT=768

# Safety
LINK_PREVIEW_SAFETY_CHECK=true
LINK_PREVIEW_PHISHING_DETECTION=true

# Rate Limiting
LINK_PREVIEW_RATE_LIMIT_PER_MINUTE=60
LINK_PREVIEW_RATE_LIMIT_PER_DOMAIN=10

# API Security (optional)
LP_API_KEY=your_api_key_here
LP_RATE_LIMIT_MAX=200
LP_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

---

## CLI Commands

The `nself-link-preview` CLI provides comprehensive link preview management.

### Core Commands

#### `init`
Initialize the database schema.

```bash
nself-link-preview init
```

#### `server`
Start the HTTP server.

```bash
nself-link-preview server [options]

Options:
  -p, --port <port>    Server port (default: 3718)
  -h, --host <host>    Server host (default: 0.0.0.0)
```

**Example:**
```bash
nself-link-preview server --port 3718 --host 0.0.0.0
```

#### `status`
Show link preview cache statistics.

```bash
nself-link-preview status
```

**Output:**
```
Link Preview Cache Statistics
=============================
Total Previews:    1,234
  Successful:      1,100
  Failed:          120
  Expired:         14
Avg Fetch Time:    245.67ms
oEmbed Count:      340
Unique Sites:      89
```

---

### Preview Commands

#### `fetch <url>`
Fetch preview for a URL.

```bash
nself-link-preview fetch <url> [options]

Options:
  -f, --force    Force refresh even if cached
```

**Examples:**
```bash
# Fetch from cache or create new preview
nself-link-preview fetch https://example.com

# Force refresh
nself-link-preview fetch https://example.com --force
```

#### `refresh <id>`
Refresh a cached preview.

```bash
nself-link-preview refresh <preview-id>
```

**Example:**
```bash
nself-link-preview refresh 550e8400-e29b-41d4-a716-446655440000
```

#### `delete <id>`
Delete a cached preview.

```bash
nself-link-preview delete <preview-id>
```

**Example:**
```bash
nself-link-preview delete 550e8400-e29b-41d4-a716-446655440000
```

---

### Template Commands

#### `template list`
List custom preview templates.

```bash
nself-link-preview template list [options]

Options:
  -l, --limit <limit>    Number of records (default: 20)
```

**Example:**
```bash
nself-link-preview template list --limit 50
```

**Output:**
```
Preview Templates:
--------------------------------------------------------------------------------
[ACTIVE] YouTube Video (priority: 100)
  ID: 123e4567-e89b-12d3-a456-426614174000
  Pattern: ^https?://(www\.)?youtube\.com/watch\?v=.*

[INACTIVE] GitHub Repository (priority: 50)
  ID: 123e4567-e89b-12d3-a456-426614174001
  Pattern: ^https?://github\.com/[^/]+/[^/]+$
```

#### `template test <id> <url>`
Test a template against a URL.

```bash
nself-link-preview template test <template-id> <url>
```

**Example:**
```bash
nself-link-preview template test 123e4567-e89b-12d3-a456-426614174000 \
  "https://youtube.com/watch?v=dQw4w9WgXcQ"
```

**Output:**
```
Template: YouTube Video
Pattern:  ^https?://(www\.)?youtube\.com/watch\?v=.*
URL:      https://youtube.com/watch?v=dQw4w9WgXcQ
Matches:  YES
```

#### `template delete <id>`
Delete a template.

```bash
nself-link-preview template delete <template-id>
```

---

### oEmbed Commands

#### `oembed providers`
List oEmbed providers.

```bash
nself-link-preview oembed providers
```

**Output:**
```
oEmbed Providers:
--------------------------------------------------------------------------------
[ACTIVE] YouTube
  ID: 550e8400-e29b-41d4-a716-446655440000
  URL: https://youtube.com
  Endpoint: https://www.youtube.com/oembed
  Schemes: https://youtube.com/watch*, https://youtu.be/*
```

#### `oembed discover <url>`
Discover oEmbed provider for a URL.

```bash
nself-link-preview oembed discover <url>
```

**Example:**
```bash
nself-link-preview oembed discover "https://youtube.com/watch?v=dQw4w9WgXcQ"
```

---

### Blocklist Commands

#### `block <url>`
Add a URL to the blocklist.

```bash
nself-link-preview block <url> [options]

Options:
  -t, --type <type>              Pattern type: exact, domain, regex (default: exact)
  -r, --reason <reason>          Reason: spam, phishing, malware, offensive, other (default: other)
  -d, --description <description> Description
```

**Examples:**
```bash
# Block exact URL
nself-link-preview block https://spam.com/malware --type exact --reason malware

# Block entire domain
nself-link-preview block spam.com --type domain --reason spam

# Block by regex pattern
nself-link-preview block "https://.*\.malicious\.com/.*" --type regex --reason phishing
```

#### `unblock <id>`
Remove a URL from the blocklist.

```bash
nself-link-preview unblock <blocklist-id>
```

#### `blocklist`
List blocked URLs.

```bash
nself-link-preview blocklist [options]

Options:
  -l, --limit <limit>    Number of records (default: 20)
```

**Output:**
```
Blocked URLs:
--------------------------------------------------------------------------------
[PHISHING] https://malicious.com (exact)
  ID: 123e4567-e89b-12d3-a456-426614174000
  Known phishing site targeting login credentials

[SPAM] spam-domain.com (domain)
  ID: 123e4567-e89b-12d3-a456-426614174001
  Expires: 2026-03-15T12:00:00Z
```

#### `check <url>`
Check if a URL is blocked.

```bash
nself-link-preview check <url>
```

**Example:**
```bash
nself-link-preview check https://spam.com
```

**Output:**
```
URL: https://spam.com
Blocked: YES
```

---

### Analytics Commands

#### `popular`
Show most popular link previews.

```bash
nself-link-preview popular [options]

Options:
  -l, --limit <limit>    Number of records (default: 10)
```

**Example:**
```bash
nself-link-preview popular --limit 20
```

**Output:**
```
Popular Link Previews:
--------------------------------------------------------------------------------
1. Rick Astley - Never Gonna Give You Up
   URL: https://youtube.com/watch?v=dQw4w9WgXcQ
   Usage: 1,234 | Clicks: 567 | CTR: 45.9%

2. Example Domain
   URL: https://example.com
   Usage: 890 | Clicks: 123 | CTR: 13.8%
```

#### `stats`
Show detailed cache statistics.

```bash
nself-link-preview stats
```

**Output:**
```
Cache Statistics:
=================
Total Previews:    1,234
  Successful:      1,100
  Failed:          120
  Expired:         14
Avg Fetch Time:    245.67ms
oEmbed Count:      340
Unique Sites:      89

Cache Hit Rate:    89.1%
```

---

### Cache Maintenance Commands

#### `cache clear`
Clear all cached previews.

```bash
nself-link-preview cache clear
```

#### `cleanup`
Remove expired previews.

```bash
nself-link-preview cleanup
```

**Output:**
```
Cleaned up 14 expired previews
```

---

## REST API

The Link Preview Plugin exposes a comprehensive REST API on port `3718`.

### Base URL

```
http://localhost:3718
```

### Authentication

If `LP_API_KEY` is set, include it in requests:

```bash
curl -H "X-API-Key: your_api_key" http://localhost:3718/api/link-preview
```

---

### Health & Status Endpoints

#### `GET /health`
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "link-preview",
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### `GET /ready`
Readiness check (validates database connection).

**Response:**
```json
{
  "ready": true,
  "plugin": "link-preview",
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### `GET /live`
Liveness check with stats.

**Response:**
```json
{
  "alive": true,
  "plugin": "link-preview",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 52428800,
    "heapTotal": 20971520,
    "heapUsed": 15728640,
    "external": 1048576
  },
  "stats": {
    "total_previews": 1234,
    "successful": 1100,
    "failed": 120,
    "expired": 14,
    "avg_fetch_duration_ms": 245.67,
    "oembed_count": 340,
    "unique_sites": 89
  },
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

#### `GET /v1/status`
Plugin status.

**Response:**
```json
{
  "plugin": "link-preview",
  "version": "1.0.0",
  "status": "running",
  "enabled": true,
  "stats": { ... },
  "timestamp": "2026-02-11T12:00:00.000Z"
}
```

---

### Preview Fetching Endpoints

#### `GET /api/link-preview`
Fetch preview for a URL (cached or new).

**Query Parameters:**
- `url` (required): URL to preview

**Example:**
```bash
curl "http://localhost:3718/api/link-preview?url=https://example.com"
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "url": "https://example.com",
  "url_hash": "a1b2c3...",
  "title": "Example Domain",
  "description": "This domain is for use in illustrative examples in documents.",
  "image_url": "https://example.com/image.png",
  "site_name": "Example",
  "favicon_url": "https://example.com/favicon.ico",
  "status": "success",
  "cache_expires_at": "2026-02-18T12:00:00Z",
  "fetch_duration_ms": 245,
  "http_status_code": 200,
  "is_safe": true,
  "created_at": "2026-02-11T12:00:00Z",
  "updated_at": "2026-02-11T12:00:00Z"
}
```

**Error Responses:**
- `400`: Missing `url` parameter
- `403`: URL is blocked

#### `POST /api/link-preview/fetch`
Fetch preview with force option.

**Request Body:**
```json
{
  "url": "https://example.com",
  "force": false
}
```

**Response:** Same as `GET /api/link-preview`

#### `GET /api/link-preview/:id`
Get preview by ID.

**Example:**
```bash
curl http://localhost:3718/api/link-preview/550e8400-e29b-41d4-a716-446655440000
```

**Response:** Preview object or `404` if not found.

#### `DELETE /api/link-preview/:id`
Delete a preview.

**Response:**
```json
{
  "success": true
}
```

#### `POST /api/link-preview/refresh/:id`
Refresh a cached preview.

**Response:** Updated preview object or `404` if not found.

---

### Batch Operations

#### `POST /api/link-preview/batch`
Fetch multiple previews at once (max 50 URLs).

**Request Body:**
```json
{
  "urls": [
    "https://example.com",
    "https://another-example.com"
  ]
}
```

**Response:**
```json
{
  "data": [
    {
      "url": "https://example.com",
      "blocked": false,
      "preview": { ... }
    },
    {
      "url": "https://another-example.com",
      "blocked": false,
      "preview": { ... }
    }
  ],
  "total": 2
}
```

**Error Responses:**
- `400`: Invalid request or >50 URLs

#### `GET /api/link-preview/message/:messageId`
Get all previews for a message.

**Response:**
```json
{
  "data": [
    { /* preview 1 */ },
    { /* preview 2 */ }
  ]
}
```

---

### Template Endpoints

#### `GET /api/link-preview/templates`
List custom preview templates.

**Query Parameters:**
- `limit` (optional): Default 100
- `offset` (optional): Default 0

**Response:**
```json
{
  "data": [
    {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "source_account_id": "primary",
      "name": "YouTube Video",
      "description": "Custom template for YouTube videos",
      "url_pattern": "^https?://(www\\.)?youtube\\.com/watch\\?v=.*",
      "priority": 100,
      "template_html": "<div>...</div>",
      "css_styles": ".youtube { ... }",
      "metadata_extractors": [],
      "is_active": true,
      "created_at": "2026-02-11T12:00:00Z"
    }
  ],
  "limit": 100,
  "offset": 0
}
```

#### `POST /api/link-preview/templates`
Create a new template.

**Request Body:**
```json
{
  "name": "YouTube Video",
  "description": "Custom template for YouTube videos",
  "url_pattern": "^https?://(www\\.)?youtube\\.com/watch\\?v=.*",
  "priority": 100,
  "template_html": "<div class='youtube'>...</div>",
  "css_styles": ".youtube { border: 1px solid #ccc; }",
  "metadata_extractors": []
}
```

**Response:** Created template object.

**Error Responses:**
- `400`: Missing required fields
- `500`: Creation failed

#### `GET /api/link-preview/templates/:id`
Get template by ID.

**Response:** Template object or `404`.

#### `PUT /api/link-preview/templates/:id`
Update a template.

**Request Body:** Partial template object with fields to update.

**Response:** Updated template object or `404`.

#### `DELETE /api/link-preview/templates/:id`
Delete a template.

**Response:**
```json
{
  "success": true
}
```

#### `POST /api/link-preview/templates/:id/test`
Test a template against a URL.

**Request Body:**
```json
{
  "url": "https://youtube.com/watch?v=dQw4w9WgXcQ"
}
```

**Response:**
```json
{
  "matches": true,
  "template_id": "123e4567-e89b-12d3-a456-426614174000",
  "url": "https://youtube.com/watch?v=dQw4w9WgXcQ",
  "pattern": "^https?://(www\\.)?youtube\\.com/watch\\?v=.*"
}
```

---

### oEmbed Endpoints

#### `GET /api/link-preview/oembed/providers`
List oEmbed providers.

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "source_account_id": "primary",
      "provider_name": "YouTube",
      "provider_url": "https://youtube.com",
      "endpoint_url": "https://www.youtube.com/oembed",
      "url_schemes": [
        "https://youtube.com/watch*",
        "https://youtu.be/*"
      ],
      "formats": ["json"],
      "discovery": true,
      "max_width": 1024,
      "max_height": 768,
      "is_active": true,
      "created_at": "2026-02-11T12:00:00Z"
    }
  ]
}
```

#### `POST /api/link-preview/oembed/providers`
Add a new oEmbed provider.

**Request Body:**
```json
{
  "provider_name": "YouTube",
  "provider_url": "https://youtube.com",
  "endpoint_url": "https://www.youtube.com/oembed",
  "url_schemes": ["https://youtube.com/watch*", "https://youtu.be/*"],
  "formats": ["json"],
  "max_width": 1024,
  "max_height": 768
}
```

**Response:** Created provider object.

#### `GET /api/link-preview/oembed/discover`
Discover oEmbed provider for a URL.

**Query Parameters:**
- `url` (required): URL to check

**Response:** Provider object or `404` if not found.

#### `GET /api/link-preview/oembed/fetch`
Fetch oEmbed data for a URL.

**Query Parameters:**
- `url` (required): URL to fetch

**Response:**
```json
{
  "provider": "YouTube",
  "endpoint": "https://www.youtube.com/oembed",
  "url": "https://youtube.com/watch?v=dQw4w9WgXcQ",
  "type": "link",
  "version": "1.0"
}
```

---

### Blocklist Endpoints

#### `GET /api/link-preview/blocklist`
List blocked URLs.

**Query Parameters:**
- `limit` (optional): Default 100
- `offset` (optional): Default 0

**Response:**
```json
{
  "data": [
    {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "source_account_id": "primary",
      "url_pattern": "https://malicious.com",
      "pattern_type": "exact",
      "reason": "phishing",
      "description": "Known phishing site",
      "added_by": "admin",
      "expires_at": null,
      "created_at": "2026-02-11T12:00:00Z"
    }
  ],
  "limit": 100,
  "offset": 0
}
```

#### `POST /api/link-preview/blocklist`
Add URL to blocklist.

**Request Body:**
```json
{
  "url_pattern": "spam.com",
  "pattern_type": "domain",
  "reason": "spam",
  "description": "Spam domain",
  "added_by": "admin",
  "expires_at": "2026-12-31T23:59:59Z"
}
```

**Response:** Created blocklist entry.

**Error Responses:**
- `400`: Missing required fields
- `500`: Creation failed

#### `DELETE /api/link-preview/blocklist/:id`
Remove URL from blocklist.

**Response:**
```json
{
  "success": true
}
```

#### `POST /api/link-preview/blocklist/check`
Check if a URL is blocked.

**Request Body:**
```json
{
  "url": "https://spam.com"
}
```

**Response:**
```json
{
  "url": "https://spam.com",
  "blocked": true
}
```

---

### Settings Endpoints

#### `GET /api/link-preview/settings`
Get preview settings.

**Query Parameters:**
- `scope` (optional): `global`, `channel`, `user` (default: `global`)
- `scope_id` (optional): Channel or user ID

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "scope": "channel",
  "scope_id": "general",
  "enabled": true,
  "auto_expand": false,
  "show_images": true,
  "show_videos": true,
  "max_previews_per_message": 3,
  "preview_position": "bottom",
  "custom_css": null,
  "blocked_domains": [],
  "allowed_domains": [],
  "created_at": "2026-02-11T12:00:00Z"
}
```

#### `PUT /api/link-preview/settings`
Update preview settings.

**Request Body:**
```json
{
  "scope": "channel",
  "scope_id": "general",
  "enabled": true,
  "auto_expand": false,
  "show_images": true,
  "show_videos": true,
  "max_previews_per_message": 3,
  "preview_position": "bottom",
  "blocked_domains": ["spam.com"],
  "allowed_domains": []
}
```

**Response:** Updated settings object.

#### `GET /api/link-preview/settings/channel/:id`
Get settings for a specific channel.

**Response:** Channel settings object.

#### `PUT /api/link-preview/settings/channel/:id`
Update settings for a specific channel.

**Request Body:** Partial settings object.

**Response:** Updated channel settings.

---

### Analytics Endpoints

#### `GET /api/link-preview/analytics`
Get analytics data.

**Query Parameters:**
- `start_date` (optional): Default 30 days ago (YYYY-MM-DD)
- `end_date` (optional): Default today (YYYY-MM-DD)
- `preview_id` (optional): Filter by preview ID

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "source_account_id": "primary",
      "date": "2026-02-11",
      "preview_id": "123e4567-e89b-12d3-a456-426614174000",
      "views_count": 150,
      "clicks_count": 45,
      "unique_users_count": 89,
      "avg_click_rate": 0.3000,
      "created_at": "2026-02-11T12:00:00Z"
    }
  ],
  "start_date": "2026-01-12",
  "end_date": "2026-02-11"
}
```

#### `GET /api/link-preview/popular`
Get most popular previews.

**Query Parameters:**
- `limit` (optional): Default 20

**Response:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "url": "https://youtube.com/watch?v=dQw4w9WgXcQ",
      "title": "Rick Astley - Never Gonna Give You Up",
      "usage_count": "1234",
      "unique_users": "567",
      "click_count": "890",
      "click_through_rate": "0.7217"
    }
  ]
}
```

#### `POST /api/link-preview/click/:usageId`
Record a click on a preview.

**Response:**
```json
{
  "success": true
}
```

**Error Responses:**
- `404`: Usage record not found or already clicked

---

### Admin Endpoints

#### `POST /api/link-preview/admin/cache/clear`
Clear all cached previews.

**Response:**
```json
{
  "success": true,
  "cleared": 1234
}
```

#### `GET /api/link-preview/admin/stats`
Get cache statistics.

**Response:**
```json
{
  "total_previews": 1234,
  "successful": 1100,
  "failed": 120,
  "expired": 14,
  "avg_fetch_duration_ms": 245.67,
  "oembed_count": 340,
  "unique_sites": 89
}
```

#### `POST /api/link-preview/admin/cleanup`
Remove expired previews.

**Response:**
```json
{
  "success": true,
  "cleaned": 14
}
```

---

### Usage Tracking Endpoint

#### `POST /api/link-preview/usage`
Track usage of a preview.

**Request Body:**
```json
{
  "preview_id": "550e8400-e29b-41d4-a716-446655440000",
  "message_id": "msg_123",
  "user_id": "user_456",
  "channel_id": "channel_789"
}
```

**Response:**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "source_account_id": "primary",
  "preview_id": "550e8400-e29b-41d4-a716-446655440000",
  "message_id": "msg_123",
  "user_id": "user_456",
  "channel_id": "channel_789",
  "clicked": false,
  "clicked_at": null,
  "created_at": "2026-02-11T12:00:00Z"
}
```

---

## Webhook Events

The Link Preview Plugin does **not** expose webhook endpoints. All operations are synchronous REST API calls or CLI commands.

---

## Database Schema

The plugin creates **7 tables** in PostgreSQL for link preview data, templates, oEmbed providers, blocklist, settings, usage tracking, and analytics.

### Table: `np_linkprev_link_previews`

Link preview cache with metadata.

```sql
CREATE TABLE IF NOT EXISTS np_linkprev_link_previews (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  url TEXT NOT NULL,
  url_hash VARCHAR(64) NOT NULL,
  title TEXT,
  description TEXT,
  image_url TEXT,
  video_url TEXT,
  audio_url TEXT,
  site_name TEXT,
  favicon_url TEXT,
  embed_html TEXT,
  embed_type VARCHAR(50),
  provider_name VARCHAR(255),
  provider_url TEXT,
  author_name VARCHAR(255),
  author_url TEXT,
  published_date TIMESTAMP WITH TIME ZONE,
  word_count INTEGER,
  reading_time_minutes INTEGER,
  tags TEXT[] DEFAULT '{}',
  language VARCHAR(10),
  metadata JSONB DEFAULT '{}',
  status VARCHAR(50) DEFAULT 'success',
  error_message TEXT,
  cache_expires_at TIMESTAMP WITH TIME ZONE,
  fetch_duration_ms INTEGER,
  http_status_code INTEGER,
  content_type VARCHAR(100),
  content_length BIGINT,
  is_safe BOOLEAN DEFAULT true,
  safety_check_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, url_hash)
);

CREATE INDEX IF NOT EXISTS idx_lp_previews_source_account ON np_linkprev_link_previews(source_account_id);
CREATE INDEX IF NOT EXISTS idx_lp_previews_hash ON np_linkprev_link_previews(url_hash);
CREATE INDEX IF NOT EXISTS idx_lp_previews_url ON np_linkprev_link_previews(url);
CREATE INDEX IF NOT EXISTS idx_lp_previews_expires ON np_linkprev_link_previews(cache_expires_at);
CREATE INDEX IF NOT EXISTS idx_lp_previews_site ON np_linkprev_link_previews(site_name);
CREATE INDEX IF NOT EXISTS idx_lp_previews_created ON np_linkprev_link_previews(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_lp_previews_status ON np_linkprev_link_previews(status);
```

**Key Columns:**
- `url_hash`: SHA-256 hash of normalized URL for fast lookups
- `status`: `success`, `failed`, `partial`, `pending`
- `embed_html`: oEmbed HTML embed code
- `metadata`: Additional custom metadata (JSONB)
- `is_safe`: Safety check result
- `cache_expires_at`: Cache expiration timestamp

---

### Table: `np_linkprev_link_preview_usage`

Usage tracking for previews.

```sql
CREATE TABLE IF NOT EXISTS np_linkprev_link_preview_usage (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  preview_id UUID NOT NULL REFERENCES np_linkprev_link_previews(id) ON DELETE CASCADE,
  message_id VARCHAR(255),
  user_id VARCHAR(255),
  channel_id VARCHAR(255),
  clicked BOOLEAN DEFAULT false,
  clicked_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lp_usage_source_account ON np_linkprev_link_preview_usage(source_account_id);
CREATE INDEX IF NOT EXISTS idx_lp_usage_preview ON np_linkprev_link_preview_usage(preview_id);
CREATE INDEX IF NOT EXISTS idx_lp_usage_message ON np_linkprev_link_preview_usage(message_id);
CREATE INDEX IF NOT EXISTS idx_lp_usage_user ON np_linkprev_link_preview_usage(user_id);
CREATE INDEX IF NOT EXISTS idx_lp_usage_created ON np_linkprev_link_preview_usage(created_at DESC);
```

**Purpose:** Tracks when and where previews are displayed, and records clicks for CTR analytics.

---

### Table: `np_linkprev_preview_templates`

Custom preview templates for specific URL patterns.

```sql
CREATE TABLE IF NOT EXISTS np_linkprev_preview_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  url_pattern TEXT NOT NULL,
  priority INTEGER DEFAULT 0,
  template_html TEXT NOT NULL,
  css_styles TEXT,
  metadata_extractors JSONB DEFAULT '[]',
  is_active BOOLEAN DEFAULT true,
  created_by VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lp_templates_source_account ON np_linkprev_preview_templates(source_account_id);
CREATE INDEX IF NOT EXISTS idx_lp_templates_active ON np_linkprev_preview_templates(is_active, priority DESC);
```

**Purpose:** Define custom HTML/CSS templates for specific URL patterns (e.g., YouTube, GitHub repos).

---

### Table: `np_linkprev_oembed_providers`

oEmbed provider registry.

```sql
CREATE TABLE IF NOT EXISTS np_linkprev_oembed_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  provider_name VARCHAR(255) NOT NULL,
  provider_url TEXT NOT NULL,
  endpoint_url TEXT NOT NULL,
  url_schemes TEXT[] NOT NULL DEFAULT '{}',
  formats VARCHAR(20)[] DEFAULT ARRAY['json'],
  discovery BOOLEAN DEFAULT true,
  max_width INTEGER,
  max_height INTEGER,
  is_active BOOLEAN DEFAULT true,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lp_oembed_source_account ON np_linkprev_oembed_providers(source_account_id);
CREATE INDEX IF NOT EXISTS idx_lp_oembed_active ON np_linkprev_oembed_providers(is_active);
```

**Purpose:** Register oEmbed providers (YouTube, Vimeo, Twitter, etc.) with URL schemes and endpoint URLs.

---

### Table: `np_linkprev_url_blocklist`

URL blocklist for spam, phishing, malware, and offensive content.

```sql
CREATE TABLE IF NOT EXISTS np_linkprev_url_blocklist (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  url_pattern TEXT NOT NULL,
  pattern_type VARCHAR(20) NOT NULL,
  reason VARCHAR(50) NOT NULL,
  description TEXT,
  added_by VARCHAR(255),
  expires_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lp_blocklist_source_account ON np_linkprev_url_blocklist(source_account_id);
CREATE INDEX IF NOT EXISTS idx_lp_blocklist_pattern ON np_linkprev_url_blocklist(url_pattern);
CREATE INDEX IF NOT EXISTS idx_lp_blocklist_expires ON np_linkprev_url_blocklist(expires_at) WHERE expires_at IS NOT NULL;
```

**Key Columns:**
- `pattern_type`: `exact`, `domain`, `regex`
- `reason`: `spam`, `phishing`, `malware`, `offensive`, `other`
- `expires_at`: Optional expiration for temporary blocks

---

### Table: `np_linkprev_preview_settings`

Per-channel/user preview settings.

```sql
CREATE TABLE IF NOT EXISTS np_linkprev_preview_settings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  scope VARCHAR(20) NOT NULL,
  scope_id VARCHAR(255),
  enabled BOOLEAN DEFAULT true,
  auto_expand BOOLEAN DEFAULT false,
  show_images BOOLEAN DEFAULT true,
  show_videos BOOLEAN DEFAULT true,
  max_previews_per_message INTEGER DEFAULT 3,
  preview_position VARCHAR(20) DEFAULT 'bottom',
  custom_css TEXT,
  blocked_domains TEXT[] DEFAULT '{}',
  allowed_domains TEXT[] DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, scope, scope_id)
);

CREATE INDEX IF NOT EXISTS idx_lp_settings_source_account ON np_linkprev_preview_settings(source_account_id);
CREATE INDEX IF NOT EXISTS idx_lp_settings_scope ON np_linkprev_preview_settings(scope, scope_id);
```

**Key Columns:**
- `scope`: `global`, `channel`, `user`
- `scope_id`: Channel or user ID (NULL for global)
- `preview_position`: `top`, `bottom`, `inline`

---

### Table: `np_linkprev_preview_analytics`

Daily analytics for preview performance.

```sql
CREATE TABLE IF NOT EXISTS np_linkprev_preview_analytics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  date DATE NOT NULL,
  preview_id UUID REFERENCES np_linkprev_link_previews(id) ON DELETE CASCADE,
  views_count INTEGER DEFAULT 0,
  clicks_count INTEGER DEFAULT 0,
  unique_users_count INTEGER DEFAULT 0,
  avg_click_rate DECIMAL(5,4),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, date, preview_id)
);

CREATE INDEX IF NOT EXISTS idx_lp_analytics_source_account ON np_linkprev_preview_analytics(source_account_id);
CREATE INDEX IF NOT EXISTS idx_lp_analytics_date ON np_linkprev_preview_analytics(date DESC);
CREATE INDEX IF NOT EXISTS idx_lp_analytics_preview ON np_linkprev_preview_analytics(preview_id);
```

**Purpose:** Aggregate daily views, clicks, and CTR for each preview.

---

## Examples

### Example 1: Basic Link Preview Workflow

```bash
# 1. Initialize the plugin
nself-link-preview init

# 2. Start the server
nself-link-preview server

# 3. Fetch a link preview
curl "http://localhost:3718/api/link-preview?url=https://example.com"

# 4. View cache statistics
nself-link-preview status
```

**Output:**
```
Link Preview Cache Statistics
=============================
Total Previews:    1
  Successful:      1
  Failed:          0
  Expired:         0
Avg Fetch Time:    245.00ms
oEmbed Count:      0
Unique Sites:      1
```

---

### Example 2: Custom Template for YouTube

```bash
# Create a custom template for YouTube videos
curl -X POST http://localhost:3718/api/link-preview/templates \
  -H "Content-Type: application/json" \
  -d '{
    "name": "YouTube Video",
    "description": "Custom embed for YouTube videos",
    "url_pattern": "^https?://(www\\.)?youtube\\.com/watch\\?v=.*",
    "priority": 100,
    "template_html": "<div class=\"youtube-embed\"><iframe src=\"{embed_url}\" frameborder=\"0\" allowfullscreen></iframe></div>",
    "css_styles": ".youtube-embed { width: 100%; max-width: 640px; aspect-ratio: 16/9; }"
  }'

# Test the template
nself-link-preview template test <template-id> "https://youtube.com/watch?v=dQw4w9WgXcQ"
```

**Output:**
```
Template: YouTube Video
Pattern:  ^https?://(www\.)?youtube\.com/watch\?v=.*
URL:      https://youtube.com/watch?v=dQw4w9WgXcQ
Matches:  YES
```

---

### Example 3: URL Blocklist Management

```bash
# Block a phishing domain
nself-link-preview block "malicious.com" \
  --type domain \
  --reason phishing \
  --description "Known phishing site targeting login credentials"

# Block specific malware URL
nself-link-preview block "https://spam.com/malware.exe" \
  --type exact \
  --reason malware

# Block by regex pattern
nself-link-preview block "https://.*\\.spam-network\\.com/.*" \
  --type regex \
  --reason spam

# Check if URL is blocked
nself-link-preview check "https://malicious.com/login"
```

**Output:**
```
URL: https://malicious.com/login
Blocked: YES
```

---

### Example 4: oEmbed Provider Configuration

```bash
# Add YouTube as an oEmbed provider
curl -X POST http://localhost:3718/api/link-preview/oembed/providers \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "YouTube",
    "provider_url": "https://youtube.com",
    "endpoint_url": "https://www.youtube.com/oembed",
    "url_schemes": [
      "https://youtube.com/watch*",
      "https://youtu.be/*"
    ],
    "formats": ["json"],
    "max_width": 1024,
    "max_height": 768
  }'

# Discover provider for a URL
nself-link-preview oembed discover "https://youtube.com/watch?v=dQw4w9WgXcQ"
```

---

### Example 5: Channel-Specific Settings

```bash
# Disable previews in a specific channel
curl -X PUT http://localhost:3718/api/link-preview/settings/channel/support \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": false,
    "blocked_domains": ["spam.com", "malicious.com"]
  }'

# Get channel settings
curl http://localhost:3718/api/link-preview/settings/channel/support
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "scope": "channel",
  "scope_id": "support",
  "enabled": false,
  "auto_expand": false,
  "show_images": true,
  "show_videos": true,
  "max_previews_per_message": 3,
  "preview_position": "bottom",
  "blocked_domains": ["spam.com", "malicious.com"],
  "allowed_domains": []
}
```

---

### Example 6: Analytics and Popular Links

```bash
# View popular links
nself-link-preview popular --limit 10

# Get analytics for date range
curl "http://localhost:3718/api/link-preview/analytics?start_date=2026-02-01&end_date=2026-02-11"

# Get analytics for specific preview
curl "http://localhost:3718/api/link-preview/analytics?preview_id=550e8400-e29b-41d4-a716-446655440000"
```

**Output:**
```
Popular Link Previews:
--------------------------------------------------------------------------------
1. Rick Astley - Never Gonna Give You Up
   URL: https://youtube.com/watch?v=dQw4w9WgXcQ
   Usage: 1,234 | Clicks: 567 | CTR: 45.9%

2. Example Domain
   URL: https://example.com
   Usage: 890 | Clicks: 123 | CTR: 13.8%
```

---

## Troubleshooting

### Issue: Preview Fetch Timeout

**Symptom:**
```
Error: Preview fetch timed out after 10 seconds
```

**Solution:**
Increase timeout in environment variables:
```bash
LINK_PREVIEW_TIMEOUT_SECONDS=30
```

---

### Issue: URL Blocked Unexpectedly

**Symptom:**
```json
{
  "error": "URL is blocked"
}
```

**Solution:**
Check blocklist and remove entry if needed:
```bash
# List blocklist
nself-link-preview blocklist

# Remove entry
nself-link-preview unblock <blocklist-id>
```

---

### Issue: High Cache Miss Rate

**Symptom:**
Cache hit rate below 50% in stats.

**Solution:**
Increase cache TTL:
```bash
LINK_PREVIEW_CACHE_TTL_HOURS=336  # 14 days
```

---

### Issue: Too Many Expired Previews

**Symptom:**
High expired count in stats.

**Solution:**
Run cleanup regularly:
```bash
# Manual cleanup
nself-link-preview cleanup

# Or set up a cron job
0 2 * * * /usr/local/bin/nself-link-preview cleanup
```

---

### Issue: Database Connection Failed

**Symptom:**
```
Error: Connection refused
```

**Solution:**
Verify PostgreSQL is running and credentials are correct:
```bash
# Check PostgreSQL status
systemctl status postgresql

# Test connection
psql -h localhost -U postgres -d nself -c "SELECT 1"
```

---

### Issue: Rate Limit Exceeded

**Symptom:**
```json
{
  "error": "Rate limit exceeded"
}
```

**Solution:**
Adjust API-level or domain-level rate limits:
```bash
# API-level
LP_RATE_LIMIT_MAX=500
LP_RATE_LIMIT_WINDOW_MS=60000

# Domain-level
LINK_PREVIEW_RATE_LIMIT_PER_MINUTE=120
LINK_PREVIEW_RATE_LIMIT_PER_DOMAIN=20
```

---

### Issue: oEmbed Provider Not Found

**Symptom:**
```json
{
  "error": "No oEmbed provider found for this URL"
}
```

**Solution:**
Add the provider manually:
```bash
curl -X POST http://localhost:3718/api/link-preview/oembed/providers \
  -H "Content-Type: application/json" \
  -d '{
    "provider_name": "CustomProvider",
    "provider_url": "https://example.com",
    "endpoint_url": "https://example.com/oembed",
    "url_schemes": ["https://example.com/*"]
  }'
```

---

### Issue: Custom Template Not Matching

**Symptom:**
Template test returns `Matches: NO`.

**Solution:**
1. Verify regex pattern syntax
2. Test with regex tool (e.g., regex101.com)
3. Ensure URL pattern is properly escaped

Example fix:
```bash
# BAD: Unescaped dots
"url_pattern": "^https://youtube.com/watch?v=.*"

# GOOD: Escaped dots
"url_pattern": "^https://youtube\\.com/watch\\?v=.*"
```

---

### Issue: Safety Check False Positives

**Symptom:**
Legitimate URLs marked as unsafe.

**Solution:**
Disable safety checks for specific domains via settings:
```bash
curl -X PUT http://localhost:3718/api/link-preview/settings \
  -H "Content-Type: application/json" \
  -d '{
    "scope": "global",
    "allowed_domains": ["example.com", "trusted-site.com"]
  }'
```

Or disable globally:
```bash
LINK_PREVIEW_SAFETY_CHECK=false
LINK_PREVIEW_PHISHING_DETECTION=false
```

---

### Issue: High Memory Usage

**Symptom:**
Server consuming excessive memory.

**Solution:**
1. Reduce cache size by lowering TTL
2. Run cleanup more frequently
3. Limit response size:
```bash
LINK_PREVIEW_MAX_RESPONSE_SIZE_MB=5
LINK_PREVIEW_CACHE_TTL_HOURS=72
```

---

### Debugging Tips

Enable debug logging:
```bash
LOG_LEVEL=debug nself-link-preview server
```

Check server logs for detailed error messages:
```bash
# Follow logs
tail -f /var/log/nself/link-preview.log

# Search for errors
grep ERROR /var/log/nself/link-preview.log
```

Test database connectivity:
```bash
# Via CLI
nself-link-preview status

# Via SQL
psql -h localhost -U postgres -d nself -c "SELECT COUNT(*) FROM np_linkprev_link_previews"
```

---

## Additional Resources

- **Plugin Repository**: https://github.com/acamarata/nself-plugins
- **Issues & Support**: https://github.com/acamarata/nself-plugins/issues
- **nself CLI**: https://github.com/acamarata/nself
- **Open Graph Protocol**: https://ogp.me/
- **oEmbed Specification**: https://oembed.com/

---

**End of Link Preview Plugin Documentation**
