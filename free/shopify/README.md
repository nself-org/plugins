# Shopify Plugin for nself

Sync Shopify store data to PostgreSQL with real-time webhook support.

## Features

- **Full Data Sync** - Shop info, products, variants, collections, customers, orders, inventory
- **Real-time Webhooks** - 22+ webhook event handlers
- **REST API** - Query synced data via HTTP endpoints
- **CLI Tools** - Command-line interface for management
- **Analytics Views** - Daily sales, top products, customer segments, inventory status

## Installation

### TypeScript Implementation

```bash
# Install shared utilities first
cd shared
npm install
npm run build
cd ..

# Install the Shopify plugin
cd plugins/shopify/ts
npm install
npm run build
```

## Configuration

Create a `.env` file in `plugins/shopify/ts/`:

```bash
# Required
SHOPIFY_SHOP_DOMAIN=your-store.myshopify.com
SHOPIFY_ACCESS_TOKEN=shpat_xxxxx
DATABASE_URL=postgresql://user:pass@localhost:5432/nself

# Optional
SHOPIFY_API_VERSION=2024-01
SHOPIFY_WEBHOOK_SECRET=your_secret

# Server options
PORT=3003
HOST=0.0.0.0
```

### Getting Shopify Credentials

1. Go to Shopify Admin > Settings > Apps and sales channels
2. Click "Develop apps" > "Create an app"
3. Configure Admin API scopes:
   - `read_products`, `write_products`
   - `read_customers`
   - `read_orders`
   - `read_inventory`
4. Install app and copy the Admin API access token

## Usage

### CLI Commands

```bash
# Initialize database schema
npx nself-shopify init

# Sync all data
npx nself-shopify sync

# Sync specific resources
npx nself-shopify sync --resources products,orders

# Start webhook server
npx nself-shopify server --port 3003

# Show sync status
npx nself-shopify status

# List data
npx nself-shopify products --limit 20
npx nself-shopify customers --limit 20
npx nself-shopify orders --status paid
npx nself-shopify collections
npx nself-shopify inventory

# View webhook events
npx nself-shopify webhooks --topic orders/create

# Show analytics
npx nself-shopify analytics
```

### REST API

Start the server and access endpoints at `http://localhost:3003`:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/webhook` | Shopify webhook receiver |
| POST | `/api/sync` | Trigger data sync |
| GET | `/api/status` | Get sync status |
| GET | `/api/shop` | Get shop info |
| GET | `/api/products` | List products |
| GET | `/api/products/:id` | Get product with variants |
| GET | `/api/customers` | List customers |
| GET | `/api/customers/:id` | Get customer |
| GET | `/api/orders` | List orders |
| GET | `/api/orders/:id` | Get order with items |
| GET | `/api/collections` | List collections |
| GET | `/api/inventory` | List inventory levels |
| GET | `/api/webhook-events` | List webhook events |
| GET | `/api/analytics/daily-sales` | Daily sales data |
| GET | `/api/analytics/top-products` | Best selling products |
| GET | `/api/analytics/customer-value` | Top customers by value |

## Webhook Setup

1. Go to Shopify Admin > Settings > Notifications
2. Scroll to "Webhooks" section
3. Create webhook for each event type:
   - URL: `https://your-domain.com/webhook`
   - Format: JSON
4. Set `SHOPIFY_WEBHOOK_SECRET` in your `.env`

### Supported Webhook Topics

| Topic | Description |
|-------|-------------|
| `orders/create` | New order placed |
| `orders/updated` | Order modified |
| `orders/paid` | Payment received |
| `orders/fulfilled` | Order shipped |
| `orders/cancelled` | Order cancelled |
| `orders/delete` | Order deleted |
| `products/create` | New product |
| `products/update` | Product modified |
| `products/delete` | Product removed |
| `customers/create` | New customer |
| `customers/update` | Customer modified |
| `customers/delete` | Customer removed |
| `inventory_levels/update` | Stock changed |
| `inventory_levels/connect` | Inventory connected |
| `inventory_levels/disconnect` | Inventory disconnected |
| `fulfillments/create` | Fulfillment created |
| `fulfillments/update` | Fulfillment updated |
| `refunds/create` | Refund issued |
| `collections/create` | Collection created |
| `collections/update` | Collection updated |
| `collections/delete` | Collection deleted |
| `shop/update` | Shop settings changed |

## Database Schema

### Tables

| Table | Description |
|-------|-------------|
| `shopify_shops` | Store metadata |
| `shopify_products` | Product catalog |
| `shopify_variants` | Product variants with pricing/inventory |
| `shopify_collections` | Product collections |
| `shopify_customers` | Customer data |
| `shopify_orders` | Order history |
| `shopify_order_items` | Line items per order |
| `shopify_inventory` | Inventory levels by location |
| `shopify_webhook_events` | Webhook event log |

### Analytics Views

```sql
-- Sales overview
SELECT * FROM shopify_sales_overview;

-- Best-selling products
SELECT * FROM shopify_top_products;

-- Low inventory alerts
SELECT * FROM shopify_low_inventory;

-- Customer lifetime value
SELECT * FROM shopify_customer_value;
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SHOPIFY_SHOP_DOMAIN` | Yes | - | Shop domain (e.g., myshop.myshopify.com) |
| `SHOPIFY_ACCESS_TOKEN` | Yes | - | Admin API access token |
| `SHOPIFY_API_VERSION` | No | 2024-01 | Shopify API version |
| `SHOPIFY_WEBHOOK_SECRET` | No | - | Webhook signing secret |
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `PORT` | No | 3003 | Server port |
| `HOST` | No | 0.0.0.0 | Server host |

## Rate Limiting

The Shopify API has rate limits (2 requests/second for standard plans). The plugin automatically:

- Throttles requests to stay within limits
- Retries on rate limit errors
- Uses bulk operations where possible

## Architecture

```
plugins/shopify/ts/
├── src/
│   ├── types.ts        # Shopify-specific type definitions
│   ├── client.ts       # Shopify REST Admin API client
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
npm run watch

# Type checking
npm run typecheck

# Development server
npm run dev
```

## Support

- [GitHub Issues](https://github.com/acamarata/nself-plugins/issues)
- [Shopify API Documentation](https://shopify.dev/docs/api/admin-rest)
