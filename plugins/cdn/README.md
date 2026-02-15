# cdn

CDN management and integration plugin for Cloudflare and BunnyCDN.

## Current Features

- **Cache Management**: Purge CDN cache by URL, wildcard, or zone-wide
- **Signed URLs**: Generate time-limited signed URLs with custom expiration
- **Zone Management**: Configure and manage CDN zones
- **Multi-provider**: Supports Cloudflare and BunnyCDN

## Planned Features

- **Analytics Sync**: Automatic syncing of bandwidth, cache hit rates, and request metrics from CDN providers (currently stub only)

## Installation

```bash
nself plugin install cdn
```

## Configuration

See plugin.json for environment variables and configuration options.

### Required Environment Variables

- `DATABASE_URL` - PostgreSQL connection string

### Optional Environment Variables

- `CDN_PROVIDER` - CDN provider (cloudflare or bunnycdn)
- `CDN_CLOUDFLARE_API_TOKEN` - Cloudflare API token
- `CDN_CLOUDFLARE_ZONE_IDS` - Comma-separated Cloudflare zone IDs
- `CDN_BUNNYCDN_API_KEY` - BunnyCDN API key
- `CDN_BUNNYCDN_PULL_ZONE_IDS` - Comma-separated BunnyCDN pull zone IDs
- `CDN_SIGNING_KEY` - Secret key for signed URLs
- `CDN_SIGNED_URL_TTL` - Default TTL for signed URLs (seconds)
- `CDN_ANALYTICS_SYNC_INTERVAL` - Analytics sync interval (seconds, default 86400)
- `CDN_PURGE_BATCH_SIZE` - Batch size for cache purging
- `CDN_API_KEY` - API key for plugin endpoints
- `CDN_RATE_LIMIT_MAX` - Max requests per window
- `CDN_RATE_LIMIT_WINDOW_MS` - Rate limit window (milliseconds)

## Usage

### CLI Commands

```bash
# Initialize database schema
nself plugin cdn init

# Start API server
nself plugin cdn server

# View statistics
nself plugin cdn status

# Manage zones
nself plugin cdn zones list
nself plugin cdn zones add <name> <provider> <zone_id>

# Purge cache
nself plugin cdn purge <zone_id> --urls url1,url2,url3
nself plugin cdn purge <zone_id> --wildcard "*.css"
nself plugin cdn purge <zone_id> --all

# Generate signed URL
nself plugin cdn sign <url> --ttl 3600

# Analytics (currently displays stored data only - sync not implemented)
nself plugin cdn analytics --zone <zone_id>
nself plugin cdn analytics sync  # Stub - returns "provider integration pending"
```

### API Endpoints

See `ts/src/server.ts` for complete API documentation.

## License

See LICENSE file in repository root.
