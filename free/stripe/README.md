# Stripe Plugin for nself

Sync Stripe billing data to PostgreSQL with real-time webhook support.

## Features

- **Full Data Sync** - Customers, products, prices, subscriptions, invoices, payments
- **Real-time Webhooks** - 24+ webhook event handlers
- **REST API** - Query synced data via HTTP endpoints
- **CLI Tools** - Command-line interface for management
- **Analytics Views** - MRR, customer summary, recent activity
- **Unified Multi-Account Sync** - Optionally sync N Stripe accounts into one dataset
- **Account Provenance** - Synced rows include `source_account_id` for per-account traceability

## Installation

```bash
nself plugin install stripe
```

No license key required. MIT-licensed. The CLI fetches the current binary, verifies its checksum, and registers the plugin with your nself stack.

## Configuration

Create a `.env` file in `plugins/stripe/ts/`:

```bash
# Required
STRIPE_API_KEY=sk_live_your_api_key
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Optional (for webhooks)
STRIPE_WEBHOOK_SECRET=whsec_your_webhook_secret

# Optional (unified multi-account sync)
# STRIPE_API_KEYS=sk_live_legacy,sk_live_rebrand
# STRIPE_ACCOUNT_LABELS=legacy,rebrand
# STRIPE_WEBHOOK_SECRETS=whsec_legacy,whsec_rebrand

# Server options
PORT=3001
HOST=0.0.0.0
```

### Getting Stripe Credentials

1. Go to [Stripe Dashboard > API Keys](https://dashboard.stripe.com/apikeys)
2. Copy your Secret Key (starts with `sk_live_` or `sk_test_`)
3. For webhooks, create an endpoint and copy the signing secret

## Usage

### CLI Commands

Run these from `plugins/stripe/ts`:

```bash
# Initialize database schema
pnpm exec nself-stripe init

# Sync all data
pnpm exec nself-stripe sync

# Sync specific resources
pnpm exec nself-stripe sync --resources customers,subscriptions

# Start webhook server
pnpm exec nself-stripe server --port 3001

# Show sync status
pnpm exec nself-stripe status

# List data
pnpm exec nself-stripe customers --limit 50
pnpm exec nself-stripe subscriptions --status active
pnpm exec nself-stripe invoices --status paid
pnpm exec nself-stripe products
pnpm exec nself-stripe prices
```

### Unified Multi-Account Sync

To treat multiple Stripe accounts as one during sync, set `STRIPE_API_KEYS` and optionally labels/secrets:

```bash
STRIPE_API_KEYS=sk_live_legacy,sk_live_rebrand
STRIPE_ACCOUNT_LABELS=legacy,rebrand
STRIPE_WEBHOOK_SECRETS=whsec_legacy,whsec_rebrand
```

`pnpm exec nself-stripe sync` and `POST /sync` will then aggregate data across all configured accounts.
Every synced row stores its origin account in `source_account_id` so you can analyze combined data and still filter by account when needed.
If the same Stripe object ID ever appears in multiple accounts, the latest synced row wins for that ID.

### REST API

Start the server and access endpoints at `http://localhost:3001`:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/webhooks/stripe` | Stripe webhook receiver |
| POST | `/sync` | Trigger data sync |
| GET | `/status` | Get sync status |
| GET | `/api/customers` | List customers |
| GET | `/api/customers/:id` | Get customer |
| GET | `/api/subscriptions` | List subscriptions |
| GET | `/api/subscriptions/:id` | Get subscription |
| GET | `/api/invoices` | List invoices |
| GET | `/api/products` | List products |
| GET | `/api/prices` | List prices |
| GET | `/api/events` | List webhook events |
| GET | `/api/stats` | Get sync statistics |

## Webhook Setup

1. Go to [Stripe Dashboard > Webhooks](https://dashboard.stripe.com/webhooks)
2. Click "Add endpoint"
3. Enter your webhook URL: `https://your-domain.com/webhooks/stripe`
4. Select events to listen for (or use "All events")
5. Copy the signing secret to `STRIPE_WEBHOOK_SECRET` (or `STRIPE_WEBHOOK_SECRETS` for multi-account mode)

### Supported Webhook Events

| Event | Description |
|-------|-------------|
| `customer.created` | New customer created |
| `customer.updated` | Customer data changed |
| `customer.deleted` | Customer deleted |
| `product.created` | New product created |
| `product.updated` | Product changed |
| `product.deleted` | Product deleted |
| `price.created` | New price created |
| `price.updated` | Price changed |
| `price.deleted` | Price deleted |
| `subscription.created` | New subscription started |
| `subscription.updated` | Subscription changed |
| `subscription.deleted` | Subscription cancelled |
| `invoice.created` | New invoice generated |
| `invoice.updated` | Invoice updated |
| `invoice.paid` | Invoice paid successfully |
| `invoice.payment_failed` | Payment failed |
| `invoice.finalized` | Invoice finalized |
| `payment_intent.created` | Payment intent created |
| `payment_intent.succeeded` | Payment successful |
| `payment_intent.payment_failed` | Payment failed |
| `payment_intent.canceled` | Payment canceled |
| `payment_method.attached` | Payment method attached |
| `payment_method.detached` | Payment method detached |
| `payment_method.updated` | Payment method updated |

## Database Schema

### Tables

| Table | Description |
|-------|-------------|
| `stripe_customers` | Customer profiles with metadata |
| `stripe_products` | Product catalog |
| `stripe_prices` | Product pricing (one-time and recurring) |
| `stripe_subscriptions` | Subscription details and status |
| `stripe_invoices` | Invoice history with line items |
| `stripe_payment_intents` | Payment attempts and status |
| `stripe_payment_methods` | Saved payment methods |
| `stripe_webhook_events` | Webhook event log |

### Analytics Views

```sql
-- Active subscriptions with customer info
SELECT * FROM stripe_active_subscriptions;

-- Monthly recurring revenue
SELECT * FROM stripe_mrr;

-- Recent failed payments
SELECT * FROM stripe_failed_payments;
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `STRIPE_API_KEY` | Yes* | - | Stripe API secret key |
| `STRIPE_API_KEYS` | No | - | Comma-separated Stripe API keys for unified multi-account sync |
| `STRIPE_ACCOUNT_LABELS` | No | - | Comma-separated labels matching `STRIPE_API_KEYS` order |
| `STRIPE_WEBHOOK_SECRET` | No | - | Webhook signing secret |
| `STRIPE_WEBHOOK_SECRETS` | No | - | Comma-separated webhook secrets matching `STRIPE_API_KEYS` order |
| `STRIPE_ACCOUNT_ID` | No | `primary` | Label used for single-account sync status output |
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `PORT` | No | 3001 | Server port |
| `HOST` | No | 0.0.0.0 | Server host |

\* `STRIPE_API_KEY` is required when `STRIPE_API_KEYS` is not set.

## Architecture

```
plugins/stripe/ts/
├── src/
│   ├── types.ts        # Stripe-specific type definitions
│   ├── client.ts       # Stripe API client wrapper
│   ├── database.ts     # Database operations
│   ├── sync.ts         # Full sync service
│   ├── webhooks.ts     # Webhook event handlers
│   ├── config.ts       # Configuration loading
│   ├── server.ts       # Fastify HTTP server
│   ├── cli.ts          # Commander.js CLI
│   └── index.ts        # Module exports
├── package.json
└── tsconfig.json
```

## Development

```bash
# Watch mode
pnpm run watch

# Type checking
pnpm run typecheck

# Development server
pnpm run dev
```

## Support

- [GitHub Issues](https://github.com/acamarata/nself-plugins/issues)
- [Stripe API Documentation](https://stripe.com/docs/api)
