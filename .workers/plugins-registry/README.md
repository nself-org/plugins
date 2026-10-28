# nself Plugin Registry - Cloudflare Worker

Fast, globally distributed API for the nself plugin registry.

## Overview

This Cloudflare Worker serves the plugin registry at `plugins.nself.org`. It:

- Caches registry, catalog, bundles and marketplace responses in the Workers Cache API
- Provides individual plugin lookup endpoints
- Handles GitHub Actions webhook for cache invalidation
- Tracks usage statistics

## Architecture

```
┌─────────────────┐     on push/tag       ┌──────────────────────┐
│  nself-plugins  │ ────────────────────► │  GitHub Actions      │
│  (this repo)    │                       │  publish.yml         │
└─────────────────┘                       └──────────┬───────────┘
                                                     │ POST /api/sync
                                                     ▼
                                          ┌──────────────────────┐
                                          │  plugins.nself.org   │
                                          │  (Cloudflare Worker) │
                                          │                      │
                                          │  ┌────────────────┐  │
                                          │  │  KV Cache      │  │
                                          │  │  (5 min TTL)   │  │
                                          │  └────────────────┘  │
                                          └──────────┬───────────┘
                                                     │
              ┌──────────────────────────────────────┼──────────────────────────────────────┐
              ▼                                      ▼                                      ▼
      ┌───────────────┐                      ┌───────────────┐                      ┌───────────────┐
      │ nself CLI     │                      │ Browser/API   │                      │ Direct curl   │
      │ plugin list   │                      │ integrations  │                      │ access        │
      └───────────────┘                      └───────────────┘                      └───────────────┘
```

## Endpoints

Every GET endpoint also accepts HEAD, including `/v1/` aliases. HEAD returns
the GET status and headers without a response body. `/catalog.json` preserves
its ETag and returns 304 when `If-None-Match` matches for either method.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/registry.json` | GET | Full plugin registry (cached) |
| `/plugins/:name` | GET | Plugin info (latest version) |
| `/plugins/:name/:version` | GET | Plugin info (specific version) |
| `/categories` | GET | List available categories |
| `/stats` | GET | Registry statistics |
| `/health` | GET | Health check |
| `/api/sync` | POST | Webhook to sync from GitHub (requires auth) |

## Quick Start

### First-Time Setup

```bash
cd .workers/plugins-registry
npm install

# Authenticate with Cloudflare
npx wrangler login

# Or set API credentials manually
export CLOUDFLARE_API_TOKEN="your_api_token"
# Or use legacy API key authentication:
# export CLOUDFLARE_API_KEY="your_api_key"
# export CLOUDFLARE_EMAIL="your_email"

# Run setup (creates KV namespace, sets secrets)
./deploy.sh --setup

# Deploy to production
./deploy.sh --production
```

### Development

```bash
# Run locally at http://localhost:8787
npm run dev

# View real-time logs from production
npm run tail
```

### Deployment

```bash
# Deploy to production (plugins.nself.org)
npm run deploy:production

# Deploy to dev (workers.dev subdomain)
npm run deploy
```

## Configuration

### Environment Variables (wrangler.toml)

| Variable | Default | Description |
|----------|---------|-------------|
| `GITHUB_REPO` | `nself-org/plugins` | Source repository |
| `GITHUB_BRANCH` | `main` | Branch to fetch from |
| `CACHE_TTL` | `300` | Cache TTL in seconds |
| `REGISTRY_VERSION` | `1.0.0` | API version |

### Secrets

Set via `wrangler secret put`:

| Secret | Description |
|--------|-------------|
| `GITHUB_SYNC_TOKEN` | Token for webhook authentication from GitHub Actions |

## KV Namespace

The Worker keeps revocations, durable ratings/install data and sync statistics in
Workers KV. Public response envelopes and per-IP GET/POST rate counters use
`caches.default`; public reads do not put or delete KV, including statistics.
`/stats` retains historical counters and records the last authenticated sync.

Cache API entries are local to each data center and may be evicted. Rate limits
are best effort within each data center, with the existing GET and POST minute
thresholds. Envelope timestamps control freshness (default five minutes), while
catalog entries remain available for stale fallback for up to a day, subject to
eviction. Sync invalidates local edge keys; other data centers refresh on TTL.
Catalog sync preserves the old copy if upstream is unavailable.

The owner must deploy the fixed worker. Before and after deployment, compare
Cloudflare GraphQL `kvOperationsAdaptiveGroups` reads/writes/deletes for the
`PLUGINS_KV` namespace over equal traffic windows. Ordinary GET traffic should
produce no KV puts/deletes; ratings, installs and authenticated sync still can.

To manage the durable namespace:

```bash
# List existing namespaces
npm run kv:list

# Create new namespace (if needed)
npm run kv:create
# Copy the ID to wrangler.toml
```

## GitHub Actions Integration

The `publish.yml` workflow automatically syncs the registry when:
- A version tag (v*) is pushed
- The workflow is manually triggered

Add this secret to the GitHub repository:
- `CLOUDFLARE_WORKER_SYNC_TOKEN` - Same value as `GITHUB_SYNC_TOKEN`

## Testing

```bash
# Health check
curl https://plugins.nself.org/health

# Get full registry
curl https://plugins.nself.org/registry.json

# Get specific plugin
curl https://plugins.nself.org/plugins/stripe

# View stats
curl https://plugins.nself.org/stats

# Force sync (requires token)
curl -X POST https://plugins.nself.org/api/sync \
  -H "Authorization: Bearer YOUR_SYNC_TOKEN"
```

## Troubleshooting

### Worker not responding

1. Check deployment status:
   ```bash
   npx wrangler deployments list
   ```

2. View logs:
   ```bash
   npm run tail
   ```

3. Verify KV namespace is bound correctly in wrangler.toml

### Cache not updating

1. Force sync via webhook
2. Wait for TTL (default 5 minutes)
3. Check that GitHub raw file is accessible

### Authentication issues

1. Re-authenticate:
   ```bash
   npx wrangler login
   ```

2. Or use API credentials:
   ```bash
   export CLOUDFLARE_API_TOKEN="your_api_token"
   # Or use legacy API key authentication:
   # export CLOUDFLARE_API_KEY="your_api_key"
   # export CLOUDFLARE_EMAIL="your_email"
   ```

## DNS Configuration

The Worker is configured to route `plugins.nself.org/*` via Cloudflare.

DNS should have:
```
Type: CNAME
Name: plugins
Target: nself-plugin-registry.<account>.workers.dev
Proxy: On (orange cloud)
```

Or use the route configuration in wrangler.toml with the zone ID.

## Files

```
.workers/plugins-registry/
├── src/
│   ├── index.ts      # Worker entry (the only source; wrangler main)
│   ├── registry.ts   # Registry loaders; edge-cache.ts owns disposable envelopes
│   ├── marketplace.ts, revocations.ts, sign.ts, middleware/
│   └── *.test.mjs    # node --test suites (run against an esbuild bundle: pnpm test)
├── deploy.sh         # Deployment script
├── package.json      # npm configuration
├── wrangler.toml     # Cloudflare Worker config
└── README.md         # This file
```
