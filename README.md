# nself Plugins

Official plugins for [nself](https://github.com/acamarata/nself) - the production-ready self-hosted backend infrastructure manager.

## Overview

nself plugins extend the core functionality of nself by integrating with third-party services. Each plugin provides:

- **Historical Data Sync** - Full data download from the service API
- **Real-time Webhooks** - Live updates as data changes
- **Database Schema** - PostgreSQL tables with indexes and analytics views
- **REST API** - HTTP endpoints for querying synced data
- **CLI Tools** - Command-line interface for management

## Available Plugins

| Plugin | Category | Port | Description |
|--------|----------|------|-------------|
| [Stripe](plugins/stripe/) | Billing | 3001 | Customers, subscriptions, invoices, payments |
| [GitHub](plugins/github/) | DevOps | 3002 | Repositories, issues, PRs, workflows, deployments |
| [Shopify](plugins/shopify/) | E-Commerce | 3003 | Products, orders, customers, inventory |

## Repository Structure Policy

Root should stay minimal and intentional:

- `.claude/` (private control plane, gitignored)
- `.codex/` (private control plane, gitignored)
- `.github/`
- `.wiki/` (public wiki source)
- `plugins/`
- `shared/`
- `registry.json`
- `registry-schema.json`
- `README.md`
- `LICENSE`
- required meta files (for example `.gitignore`)

Allowed exception:
- `.workers/` is retained because it is required registry publishing infrastructure.

All planning/temp/task artifacts belong in `.claude/` or `.codex/`.
All public docs belong in `.wiki/` (legacy `docs/` is retired).

## Authorship Attribution Policy

Tracked project artifacts must remain attribution-free regarding assistant/tool authorship.

- No assistant/tool authorship claims in code comments, docs, release notes, or commits.
- No `Co-authored-by` trailers in commit messages.
- Product capability language is allowed (for example, feature descriptions such as `AI-powered`).

Local enforcement:

```bash
bash .github/scripts/install-hooks.sh
```

## Quick Start

### Prerequisites

- Node.js 18+
- PostgreSQL 14+
- Service API credentials (Stripe, GitHub, or Shopify)

### Installation

```bash
# Clone the repository
git clone https://github.com/acamarata/nself-plugins.git
cd nself-plugins

# Install shared utilities
cd shared
npm install
npm run build
cd ..

# Install a plugin (e.g., Stripe)
cd plugins/stripe/ts
npm install
npm run build
```

### Configuration

Create a `.env` file in the plugin's `ts/` directory:

```bash
# Database (required for all plugins)
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Stripe
STRIPE_API_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...

# GitHub
GITHUB_TOKEN=ghp_...
GITHUB_WEBHOOK_SECRET=...
GITHUB_ORG=your-org

# Shopify
SHOPIFY_SHOP_DOMAIN=myshop.myshopify.com
SHOPIFY_ACCESS_TOKEN=shpat_...
SHOPIFY_WEBHOOK_SECRET=...
```

### Running

```bash
# Initialize database schema
npx nself-stripe init

# Run full data sync
npx nself-stripe sync

# Start webhook server
npx nself-stripe server --port 3001
```

## Plugin Architecture

Each TypeScript plugin follows a consistent architecture:

```
plugins/<name>/ts/
├── src/
│   ├── types.ts        # Type definitions
│   ├── client.ts       # Service API client
│   ├── database.ts     # Database operations
│   ├── sync.ts         # Sync service
│   ├── webhooks.ts     # Webhook handlers
│   ├── config.ts       # Configuration
│   ├── server.ts       # Fastify HTTP server
│   ├── cli.ts          # CLI commands
│   └── index.ts        # Module exports
├── package.json
└── tsconfig.json
```

### Shared Utilities

The `shared/` directory contains common TypeScript utilities:

- `types.ts` - Common types (PluginConfig, SyncResult, etc.)
- `logger.ts` - Colored logging with levels
- `database.ts` - PostgreSQL connection pool and helpers
- `webhook.ts` - Webhook signature verification
- `http.ts` - HTTP client with rate limiting

## CLI Commands

All plugins share common commands:

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema |
| `sync` | Sync all data from service |
| `server` | Start webhook server |
| `status` | Show sync statistics |

Plugin-specific commands are documented in each plugin's README.

## REST API

Each plugin exposes HTTP endpoints on its configured port:

### Common Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/webhook` | Webhook receiver |
| POST | `/api/sync` | Trigger data sync |
| GET | `/api/status` | Get sync status |

### Resource Endpoints

- **Stripe**: `/api/customers`, `/api/subscriptions`, `/api/invoices`, `/api/mrr`
- **GitHub**: `/api/repositories`, `/api/issues`, `/api/pull-requests`, `/api/commits`
- **Shopify**: `/api/shop`, `/api/products`, `/api/orders`, `/api/analytics/*`

## Database Schema

Each plugin creates its own set of tables:

### Stripe Tables
- `stripe_customers`, `stripe_products`, `stripe_prices`
- `stripe_subscriptions`, `stripe_invoices`
- `stripe_payment_intents`, `stripe_payment_methods`
- `stripe_webhook_events`

### GitHub Tables
- `github_repositories`, `github_issues`, `github_pull_requests`
- `github_commits`, `github_releases`
- `github_workflow_runs`, `github_deployments`
- `github_webhook_events`

### Shopify Tables
- `shopify_shops`, `shopify_products`, `shopify_variants`
- `shopify_collections`, `shopify_customers`
- `shopify_orders`, `shopify_order_items`
- `shopify_inventory`, `shopify_webhook_events`

## Webhook Setup

### nself Subdomain Routing

Use nself's built-in subdomain routing for production:

```
https://stripe.your-domain.com/webhook
https://github.your-domain.com/webhook
https://shopify.your-domain.com/webhook
```

### Local Development

Use ngrok or similar for local testing:

```bash
ngrok http 3001  # Stripe
ngrok http 3002  # GitHub
ngrok http 3003  # Shopify
```

### Webhook Security

All plugins verify webhook signatures using HMAC-SHA256:
- **Stripe**: `Stripe-Signature` header
- **GitHub**: `X-Hub-Signature-256` header
- **Shopify**: `X-Shopify-Hmac-Sha256` header

## Registry

The `registry.json` file contains metadata for all plugins:

- Plugin info (name, version, description)
- Implementation details (language, runtime, framework)
- Database tables and views
- Supported webhooks
- CLI commands
- API endpoints
- Environment variables

## Development

### Building

```bash
# Build shared utilities
cd shared && npm run build

# Build a plugin
cd plugins/stripe/ts && npm run build

# Watch mode
npm run watch
```

### Type Checking

```bash
npm run typecheck
```

### Development Server

```bash
npm run dev
```

## Requirements

- nself v0.4.8 or later
- Node.js 18+
- PostgreSQL 14+

## License

Source-Available License - See [LICENSE](LICENSE)

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](.wiki/CONTRIBUTING.md) for guidelines.

## Support

- [Documentation](https://github.com/acamarata/nself-plugins/wiki)
- [Repository Structure Policy](.wiki/REPOSITORY-STRUCTURE.md)
- [Changelog](.wiki/CHANGELOG.md)
- [License Page](.wiki/License.md)
- [Issues](https://github.com/acamarata/nself-plugins/issues)
- [nself Main Repo](https://github.com/acamarata/nself)
