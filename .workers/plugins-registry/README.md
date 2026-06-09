# nself Plugin Registry — Cloudflare Worker

Fast, globally distributed API for the nself plugin registry.

## Language

TypeScript (strict mode, `noImplicitAny`, `noImplicitReturns`). Source files live in `src/`.
Wrangler compiles them at deploy/dev time — no separate build step required.

Run the type-checker standalone:
```bash
pnpm typecheck
```

## Overview

This Cloudflare Worker serves the plugin registry at `plugins.nself.org`. It:

- Combines free (public) and pro (private) plugin registries from GitHub
- Generates Ed25519 tarball signatures so the CLI can verify downloads offline
- Maintains a revocation list that the CLI polls hourly
- Caches all registry data in Workers KV (5-minute TTL)
- Handles GitHub Actions webhook for instant cache invalidation

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

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check — `{"status":"ok","ts":"..."}` |
| `/plugins` | GET | All plugins (`PluginListResponse`) — query params: `tier`, `category` |
| `/plugins/:name` | GET | Single plugin detail |
| `/plugins/:name/tarball` | GET | 302 redirect to GitHub tarball; `X-Signature` header if key configured |
| `/plugins/:name/signature` | GET | Ed25519 signature metadata — `{name, version, signature, publicKey, ...}` |
| `/plugins/:name/:version` | GET | Plugin at a specific version |
| `/plugins/revocations` | GET | Revocation list — polled hourly by CLI |
| `/registry.json` | GET | Combined registry (legacy CLI compat) — query param: `tier` |
| `/categories` | GET | Category list |
| `/manifest.json` | GET | Flat array for `nself plugin outdated` |
| `/stats` | GET | KV cache statistics |
| `/api/sync` | POST | Force-refresh KV cache (requires `Authorization: Bearer <GITHUB_SYNC_TOKEN>`) |

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

### Environment Variables (wrangler.toml `[vars]`)

| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_TTL` | `300` | KV cache TTL in seconds |
| `PLUGIN_REGISTRY_VERSION` | `1.0.0` | Returned in `/health` response |

### Secrets

Set via `wrangler secret put <NAME>`:

| Secret | Description |
|--------|-------------|
| `SIGNING_PRIVATE_KEY` | Ed25519 seed — 32 bytes as 64 lowercase hex chars. Signs tarball URLs. |
| `PUBLIC_KEY_HEX` | Ed25519 public key — 32 bytes as 64 lowercase hex chars. Returned in `/signature` endpoint for CLI offline verification. |
| `GH_ACCESS_TOKEN` | Fine-grained GitHub PAT with `contents:read` on `plugins` and `plugins-pro` repos. |
| `GITHUB_SYNC_TOKEN` | Bearer token for `POST /api/sync` (triggered by GitHub Actions on release). |

Generate an Ed25519 keypair:
```bash
# Private key seed (32 bytes → 64 hex chars)
openssl genpkey -algorithm ed25519 | openssl pkey -outform DER | tail -c 32 | xxd -p -c 64

# Public key (32 bytes → 64 hex chars)
openssl pkey -pubout -outform DER | tail -c 32 | xxd -p -c 64
```

### KV Schema

The `REGISTRY` KV namespace stores:

| Key | Value | Description |
|-----|-------|-------------|
| `registry:free` | `{data: PluginEntry[], timestamp: ms}` | Cached free registry |
| `registry:pro` | `{data: PluginEntry[], timestamp: ms}` | Cached pro registry |
| `registry:combined` | `{data: CombinedRegistry, timestamp: ms}` | Cached merged output |
| `registry:manifest` | `{data: ManifestEntry[], timestamp: ms}` | Cached CLI manifest |
| `revocations:list` | `RevocationEntry[]` | Raw JSON array (no envelope) |
| `stats:global` | `StatsRecord` | Request counters |

All entries except `revocations:list` use a `{ data, timestamp }` envelope for TTL freshness checks.

## KV Namespace

The Worker uses Workers KV for caching:

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
│   ├── index.ts        # Worker entry point — router and all route handlers
│   ├── registry.ts     # PluginEntry types, KV helpers, GitHub registry loaders
│   ├── sign.ts         # Ed25519 sign/verify via Web Crypto API (crypto.subtle)
│   ├── revocations.ts  # Revocation list module (KV-backed)
│   ├── marketplace.js  # Marketplace endpoint (categorised plugin cards)
│   └── sign.js         # Legacy JS signing (kept for reference, not imported by TS)
├── deploy.sh           # Deployment and first-time setup script
├── package.json        # pnpm config — includes TypeScript and @cloudflare/workers-types
├── tsconfig.json       # TypeScript strict config (target ES2022, bundler resolution)
├── wrangler.toml       # Cloudflare Worker config (main = src/index.ts)
└── README.md           # This file
```

## Deployment

```bash
cd plugins/.workers/plugins-registry

# First-time: install deps
pnpm install

# Type-check
pnpm typecheck

# Local dev at http://localhost:8787
pnpm dev

# Deploy to production
pnpm deploy:production
# or:
./deploy.sh --production
```
