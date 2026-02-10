# Shopify Plugin for nself

Complete Shopify e-commerce integration that syncs your store's products, orders, customers, inventory, and more to your local PostgreSQL database with real-time webhook support for instant updates.

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhooks](#webhooks)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Use Cases](#use-cases)
- [TypeScript Implementation](#typescript-implementation)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Shopify plugin provides complete synchronization between your Shopify store and a local PostgreSQL database. It captures all aspects of your e-commerce operations including products, variants, collections, customers, orders, inventory, fulfillments, refunds, and more.

### Why Sync Shopify Data Locally?

1. **Faster Analytics** - Run complex SQL queries on your entire store history without API latency
2. **No API Rate Limits** - Query millions of orders without hitting Shopify's rate limits
3. **Cross-Platform Integration** - Join Shopify data with data from other services
4. **Custom Reporting** - Build custom dashboards and reports with SQL
5. **Headless Commerce** - Power headless frontends with your synced product catalog
6. **Real-Time Updates** - Webhooks keep your local data current as orders flow in
7. **Historical Analysis** - Track trends and patterns over your complete order history
8. **Backup & Recovery** - Your data is always accessible, even if Shopify is down

---

## Features

### Data Synchronization

| Resource | Synced Data | Incremental Sync |
|----------|-------------|------------------|
| Shop | Store metadata and settings | Yes |
| Products | Full catalog with images, metafields | Yes |
| Variants | SKU, price, inventory, weight | Yes |
| Collections | Smart and custom collections | Yes |
| Customers | Profiles, addresses, tags | Yes |
| Orders | Complete order history with line items | Yes |
| Order Items | Individual line items per order | Yes |
| Fulfillments | Shipment tracking data | Yes |
| Refunds | Refund transactions and adjustments | Yes |
| Transactions | Payment transactions | Yes |
| Inventory | Stock levels per location | Yes |
| Locations | Store locations/warehouses | Yes |
| Draft Orders | Unpaid draft orders | Yes |
| Abandoned Checkouts | Incomplete checkouts | Yes |
| Price Rules | Discount rules | Yes |
| Discount Codes | Generated discount codes | Yes |
| Gift Cards | Gift card balances | Yes |
| Metafields | Shop and resource metafields | Yes |

### Real-Time Webhooks

Supported webhook events for instant updates:

- `orders/create` - New order placed
- `orders/updated` - Order modified
- `orders/paid` - Payment received
- `orders/fulfilled` - Order shipped
- `orders/cancelled` - Order cancelled
- `orders/delete` - Order deleted
- `products/create` - New product created
- `products/update` - Product modified
- `products/delete` - Product deleted
- `customers/create` - New customer registered
- `customers/update` - Customer info changed
- `customers/delete` - Customer deleted
- `inventory_levels/update` - Stock level changed
- `inventory_levels/connect` - Inventory connected to location
- `inventory_levels/disconnect` - Inventory disconnected
- `fulfillments/create` - Shipment created
- `fulfillments/update` - Shipment updated
- `refunds/create` - Refund issued
- `collections/create` - Collection created
- `collections/update` - Collection modified
- `collections/delete` - Collection deleted
- `shop/update` - Store settings changed
- `draft_orders/create` - Draft order created
- `draft_orders/update` - Draft order modified
- `draft_orders/delete` - Draft order deleted
- `order_transactions/create` - Transaction recorded
- `checkouts/create` - Checkout started
- `checkouts/update` - Checkout modified
- `checkouts/delete` - Checkout completed/abandoned
- `themes/create`, `themes/update`, `themes/delete`, `themes/publish`
- `app/uninstalled` - App removed from store

---

## Installation

### Via nself CLI

```bash
# Install the plugin
nself plugin install shopify

# Verify installation
nself plugin status shopify
```

### Manual Installation

```bash
# Clone the repository
git clone https://github.com/acamarata/nself-plugins.git
cd nself-plugins/plugins/shopify/ts

# Install dependencies
npm install

# Build
npm run build

# Link for CLI access
npm link
```

---

## Configuration

### Environment Variables

Create a `.env` file in the plugin directory or add to your project's `.env`:

```bash
# Required - Your Shopify store domain
# Format: your-store.myshopify.com (NOT the custom domain)
SHOPIFY_SHOP_DOMAIN=your-store.myshopify.com

# Required - Admin API access token
# Generate via: Settings > Apps > Develop apps > Create an app
SHOPIFY_ACCESS_TOKEN=shpat_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Required - PostgreSQL connection string
DATABASE_URL=postgresql://user:password@localhost:5432/nself

# Optional - API version (default: 2024-01)
SHOPIFY_API_VERSION=2024-01

# Optional - Webhook signing secret
# Get from: Settings > Notifications > Webhooks
SHOPIFY_WEBHOOK_SECRET=your_webhook_secret

# Optional - Server configuration
PORT=3003
HOST=0.0.0.0

# Optional - Sync interval in seconds (default: 3600)
SHOPIFY_SYNC_INTERVAL=3600
```

### Creating a Shopify App for API Access

1. Go to your Shopify Admin
2. Navigate to **Settings** > **Apps and sales channels**
3. Click **Develop apps** > **Create an app**
4. Name your app (e.g., "nself Sync")
5. Configure **Admin API access scopes**:

| Scope | Purpose |
|-------|---------|
| `read_products` | Product and variant data |
| `read_product_listings` | Published products |
| `read_inventory` | Inventory levels |
| `read_locations` | Store locations |
| `read_customers` | Customer data |
| `read_orders` | Order history |
| `read_all_orders` | Older orders beyond 60 days |
| `read_draft_orders` | Draft orders |
| `read_checkouts` | Abandoned checkouts |
| `read_fulfillments` | Shipment data |
| `read_price_rules` | Discount rules |
| `read_discounts` | Discount codes |
| `read_gift_cards` | Gift card data |
| `read_metafields` | Metafield data |

6. Install the app
7. Generate and copy the **Admin API access token**

---

## Usage

### Initialize Database Schema

```bash
# Create all required tables
nself-shopify init

# Or via nself CLI
nself plugin shopify init
```

### Sync Data

```bash
# Sync all data from Shopify
nself-shopify sync

# Sync specific resources
nself-shopify sync --resources products,orders,customers

# Incremental sync (only changes since last sync)
nself-shopify sync --incremental

# Sync orders from a specific date
nself-shopify sync --resources orders --since 2024-01-01
```

### Start Webhook Server

```bash
# Start the server
nself-shopify server

# Custom port
nself-shopify server --port 3003

# The server exposes:
# - POST /webhook - Shopify webhook endpoint
# - GET /health - Health check
# - GET /api/* - REST API endpoints
```

---

## CLI Commands

### Product Commands

```bash
# List all products
nself-shopify products list

# List with variants
nself-shopify products list --variants

# Search products
nself-shopify products search "t-shirt"

# Get product details
nself-shopify products get <product_id>

# List product variants
nself-shopify products variants <product_id>

# Low stock products
nself-shopify products low-stock --threshold 10
```

### Order Commands

```bash
# List recent orders
nself-shopify orders list

# Filter by status
nself-shopify orders list --status paid
nself-shopify orders list --status fulfilled
nself-shopify orders list --status unfulfilled

# Filter by date range
nself-shopify orders list --since 2024-01-01 --until 2024-12-31

# Get order details
nself-shopify orders get <order_id>

# View order line items
nself-shopify orders items <order_id>

# Daily sales summary
nself-shopify orders daily

# Export orders
nself-shopify orders export --format csv --output orders.csv
```

### Customer Commands

```bash
# List customers
nself-shopify customers list

# Search customers
nself-shopify customers search "john@example.com"

# Get customer details
nself-shopify customers get <customer_id>

# Customer order history
nself-shopify customers orders <customer_id>

# Top customers by spend
nself-shopify customers top --limit 20
```

### Collection Commands

```bash
# List all collections
nself-shopify collections list

# List products in a collection
nself-shopify collections products <collection_id>
```

### Inventory Commands

```bash
# List inventory levels
nself-shopify inventory list

# Filter by location
nself-shopify inventory list --location <location_id>

# Low stock alerts
nself-shopify inventory low-stock --threshold 5

# Inventory by product
nself-shopify inventory product <product_id>
```

### Analytics Commands

```bash
# Daily sales summary
nself-shopify analytics daily-sales

# Top products by revenue
nself-shopify analytics top-products --limit 10

# Customer lifetime value
nself-shopify analytics customer-value --limit 20

# Monthly revenue
nself-shopify analytics monthly

# Fulfillment metrics
nself-shopify analytics fulfillment
```

### Webhook Commands

```bash
# List recent webhook events
nself-shopify webhooks list

# Filter by topic
nself-shopify webhooks list --topic orders/create

# Retry failed events
nself-shopify webhooks retry <event_id>
```

### Status Command

```bash
# Show sync status and statistics
nself-shopify status

# Output:
# Shop: your-store.myshopify.com
# Products: 1,234 (5,678 variants)
# Customers: 45,678
# Orders: 123,456
# Total Revenue: $2,345,678.90
# Inventory Items: 7,890
# Last Sync: 2026-01-24 12:00:00
```

---

## REST API

The plugin exposes a REST API when running in server mode.

### Endpoints

#### Health Check

```http
GET /health
```

Response:
```json
{
  "status": "ok",
  "version": "1.0.0",
  "shop": "your-store.myshopify.com"
}
```

#### Sync Trigger

```http
POST /api/sync
Content-Type: application/json

{
  "resources": ["products", "orders", "customers"],
  "incremental": true
}
```

Response:
```json
{
  "results": [
    { "resource": "products", "synced": 1234, "duration": 5678 },
    { "resource": "orders", "synced": 456, "duration": 12345 },
    { "resource": "customers", "synced": 789, "duration": 3456 }
  ]
}
```

#### Sync Status

```http
GET /api/status
```

Response:
```json
{
  "shop": "your-store.myshopify.com",
  "stats": {
    "products": 1234,
    "variants": 5678,
    "customers": 45678,
    "orders": 123456,
    "collections": 45,
    "inventory_items": 7890
  },
  "last_sync": "2026-01-24T12:00:00Z"
}
```

#### Shop Information

```http
GET /api/shop
```

Response:
```json
{
  "id": 12345678,
  "name": "My Store",
  "domain": "my-store.myshopify.com",
  "email": "store@example.com",
  "currency": "USD",
  "timezone": "America/New_York",
  "plan_name": "Shopify Plus"
}
```

#### Products

```http
GET /api/products
GET /api/products?limit=50&offset=0
GET /api/products?collection_id=123
GET /api/products?status=active
GET /api/products/:id
GET /api/products/:id/variants
GET /api/products/:id/inventory
```

#### Customers

```http
GET /api/customers
GET /api/customers?limit=50&offset=0
GET /api/customers?email=john@example.com
GET /api/customers/:id
GET /api/customers/:id/orders
```

#### Orders

```http
GET /api/orders
GET /api/orders?limit=50&offset=0
GET /api/orders?status=paid
GET /api/orders?since=2024-01-01
GET /api/orders?customer_id=123
GET /api/orders/:id
GET /api/orders/:id/items
GET /api/orders/:id/fulfillments
GET /api/orders/:id/refunds
```

#### Collections

```http
GET /api/collections
GET /api/collections/:id
GET /api/collections/:id/products
```

#### Inventory

```http
GET /api/inventory
GET /api/inventory?location_id=123
GET /api/inventory?product_id=456
GET /api/inventory/low-stock?threshold=10
```

#### Locations

```http
GET /api/locations
GET /api/locations/:id
GET /api/locations/:id/inventory
```

#### Analytics

```http
GET /api/analytics/daily-sales
GET /api/analytics/daily-sales?start=2024-01-01&end=2024-12-31
GET /api/analytics/top-products?limit=10
GET /api/analytics/customer-value?limit=20
GET /api/analytics/monthly-revenue
```

---

## Webhooks

### Webhook Setup

1. Go to your Shopify Admin
2. Navigate to **Settings** > **Notifications** > **Webhooks**
3. Click **Create webhook**
4. Configure:
   - **Event**: Select the event type
   - **Format**: JSON
   - **URL**: `https://your-domain.com/webhook`
5. Copy the webhook signing secret to `SHOPIFY_WEBHOOK_SECRET`

Alternatively, register webhooks programmatically through the Shopify API.

### Webhook Endpoint

```http
POST /webhook
X-Shopify-Topic: orders/create
X-Shopify-Hmac-Sha256: base64_signature
X-Shopify-Shop-Domain: your-store.myshopify.com
X-Shopify-Webhook-Id: uuid

{
  "id": 12345,
  ...
}
```

### Signature Verification

The plugin verifies all incoming webhooks using HMAC-SHA256 with Base64 encoding:

```typescript
import crypto from 'crypto';

function verifyShopifySignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  const expected = crypto
    .createHmac('sha256', secret)
    .update(payload, 'utf8')
    .digest('base64');

  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(expected)
  );
}
```

### Event Handling

Each webhook event is:
1. Verified for signature
2. Stored in `shopify_webhook_events` table
3. Processed by appropriate handler
4. Used to update synced data in real-time

---

## Database Schema

### Tables

#### shopify_shops

```sql
CREATE TABLE shopify_shops (
    id BIGINT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    domain VARCHAR(255) NOT NULL,
    myshopify_domain VARCHAR(255) NOT NULL UNIQUE,
    phone VARCHAR(50),
    address1 VARCHAR(255),
    address2 VARCHAR(255),
    city VARCHAR(255),
    province VARCHAR(255),
    province_code VARCHAR(10),
    country VARCHAR(100),
    country_code VARCHAR(10),
    zip VARCHAR(20),
    currency VARCHAR(10) DEFAULT 'USD',
    money_format VARCHAR(100),
    timezone VARCHAR(100),
    plan_name VARCHAR(100),
    plan_display_name VARCHAR(100),
    shop_owner VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### shopify_products

```sql
CREATE TABLE shopify_products (
    id BIGINT PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    body_html TEXT,
    vendor VARCHAR(255),
    product_type VARCHAR(255),
    handle VARCHAR(255) UNIQUE,
    status VARCHAR(50) DEFAULT 'active',
    template_suffix VARCHAR(255),
    published_scope VARCHAR(100),
    tags TEXT,
    image JSONB,
    images JSONB DEFAULT '[]',
    options JSONB DEFAULT '[]',
    metafields JSONB DEFAULT '[]',
    published_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### shopify_variants

```sql
CREATE TABLE shopify_variants (
    id BIGINT PRIMARY KEY,
    product_id BIGINT REFERENCES shopify_products(id) ON DELETE CASCADE,
    title VARCHAR(255),
    sku VARCHAR(255),
    barcode VARCHAR(255),
    price DECIMAL(10, 2),
    compare_at_price DECIMAL(10, 2),
    position INTEGER DEFAULT 1,
    option1 VARCHAR(255),
    option2 VARCHAR(255),
    option3 VARCHAR(255),
    taxable BOOLEAN DEFAULT TRUE,
    tax_code VARCHAR(100),
    weight DECIMAL(10, 4),
    weight_unit VARCHAR(10) DEFAULT 'kg',
    inventory_item_id BIGINT,
    inventory_quantity INTEGER DEFAULT 0,
    inventory_policy VARCHAR(50) DEFAULT 'deny',
    inventory_management VARCHAR(100),
    fulfillment_service VARCHAR(100) DEFAULT 'manual',
    requires_shipping BOOLEAN DEFAULT TRUE,
    image_id BIGINT,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### shopify_customers

```sql
CREATE TABLE shopify_customers (
    id BIGINT PRIMARY KEY,
    email VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    phone VARCHAR(50),
    accepts_marketing BOOLEAN DEFAULT FALSE,
    accepts_marketing_updated_at TIMESTAMP WITH TIME ZONE,
    marketing_opt_in_level VARCHAR(100),
    orders_count INTEGER DEFAULT 0,
    total_spent DECIMAL(12, 2) DEFAULT 0,
    tax_exempt BOOLEAN DEFAULT FALSE,
    tax_exemptions JSONB DEFAULT '[]',
    tags TEXT,
    note TEXT,
    state VARCHAR(100) DEFAULT 'enabled',
    verified_email BOOLEAN DEFAULT FALSE,
    currency VARCHAR(10),
    default_address JSONB,
    addresses JSONB DEFAULT '[]',
    metafields JSONB DEFAULT '[]',
    last_order_id BIGINT,
    last_order_name VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### shopify_orders

```sql
CREATE TABLE shopify_orders (
    id BIGINT PRIMARY KEY,
    order_number INTEGER,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(50),
    customer_id BIGINT REFERENCES shopify_customers(id) ON DELETE SET NULL,
    financial_status VARCHAR(100),
    fulfillment_status VARCHAR(100),
    cancel_reason VARCHAR(100),
    cancelled_at TIMESTAMP WITH TIME ZONE,
    closed_at TIMESTAMP WITH TIME ZONE,
    confirmed BOOLEAN DEFAULT TRUE,
    test BOOLEAN DEFAULT FALSE,
    currency VARCHAR(10) DEFAULT 'USD',
    subtotal_price DECIMAL(12, 2),
    total_price DECIMAL(12, 2),
    total_tax DECIMAL(12, 2),
    total_discounts DECIMAL(12, 2),
    total_shipping DECIMAL(12, 2),
    total_weight INTEGER,
    taxes_included BOOLEAN DEFAULT FALSE,
    tax_lines JSONB DEFAULT '[]',
    discount_codes JSONB DEFAULT '[]',
    discount_applications JSONB DEFAULT '[]',
    note TEXT,
    note_attributes JSONB DEFAULT '[]',
    tags TEXT,
    gateway VARCHAR(100),
    payment_gateway_names JSONB DEFAULT '[]',
    processing_method VARCHAR(100),
    source_name VARCHAR(100),
    source_identifier VARCHAR(255),
    source_url VARCHAR(2048),
    landing_site VARCHAR(2048),
    referring_site VARCHAR(2048),
    browser_ip VARCHAR(50),
    buyer_accepts_marketing BOOLEAN DEFAULT FALSE,
    billing_address JSONB,
    shipping_address JSONB,
    shipping_lines JSONB DEFAULT '[]',
    fulfillments JSONB DEFAULT '[]',
    refunds JSONB DEFAULT '[]',
    checkout_token VARCHAR(255),
    cart_token VARCHAR(255),
    token VARCHAR(255),
    processed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### shopify_order_items

```sql
CREATE TABLE shopify_order_items (
    id BIGINT PRIMARY KEY,
    order_id BIGINT REFERENCES shopify_orders(id) ON DELETE CASCADE,
    product_id BIGINT,
    variant_id BIGINT,
    title VARCHAR(255),
    variant_title VARCHAR(255),
    sku VARCHAR(255),
    vendor VARCHAR(255),
    name VARCHAR(511),
    quantity INTEGER NOT NULL,
    price DECIMAL(10, 2),
    total_discount DECIMAL(10, 2) DEFAULT 0,
    fulfillment_status VARCHAR(100),
    fulfillable_quantity INTEGER DEFAULT 0,
    fulfillment_service VARCHAR(100),
    grams INTEGER DEFAULT 0,
    requires_shipping BOOLEAN DEFAULT TRUE,
    taxable BOOLEAN DEFAULT TRUE,
    gift_card BOOLEAN DEFAULT FALSE,
    properties JSONB DEFAULT '[]',
    tax_lines JSONB DEFAULT '[]',
    discount_allocations JSONB DEFAULT '[]',
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

#### shopify_inventory

```sql
CREATE TABLE shopify_inventory (
    inventory_item_id BIGINT NOT NULL,
    location_id BIGINT NOT NULL,
    available INTEGER DEFAULT 0,
    on_hand INTEGER DEFAULT 0,
    incoming INTEGER DEFAULT 0,
    reserved INTEGER DEFAULT 0,
    committed INTEGER DEFAULT 0,
    damaged INTEGER DEFAULT 0,
    quality_control INTEGER DEFAULT 0,
    safety_stock INTEGER DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (inventory_item_id, location_id)
);
```

#### shopify_webhook_events

```sql
CREATE TABLE shopify_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    topic VARCHAR(100) NOT NULL,
    shop_id BIGINT,
    shop_domain VARCHAR(255),
    data JSONB NOT NULL,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### Additional Tables

- `shopify_collections` - Product collections
- `shopify_locations` - Store locations/warehouses
- `shopify_fulfillments` - Fulfillment/shipment records
- `shopify_refunds` - Refund transactions
- `shopify_transactions` - Payment transactions
- `shopify_draft_orders` - Draft orders
- `shopify_checkouts` - Abandoned checkouts
- `shopify_price_rules` - Discount rules
- `shopify_discount_codes` - Discount codes
- `shopify_gift_cards` - Gift card records
- `shopify_metafields` - Resource metafields

---

## Analytics Views

Pre-built SQL views for common e-commerce analytics:

### shopify_sales_overview

```sql
CREATE VIEW shopify_sales_overview AS
SELECT
    DATE(created_at) AS date,
    COUNT(*) AS order_count,
    SUM(total_price) AS revenue,
    AVG(total_price) AS avg_order_value,
    SUM(total_discounts) AS total_discounts,
    COUNT(DISTINCT customer_id) AS unique_customers
FROM shopify_orders
WHERE financial_status = 'paid'
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

### shopify_top_products

```sql
CREATE VIEW shopify_top_products AS
SELECT
    p.id,
    p.title,
    p.vendor,
    p.product_type,
    COUNT(DISTINCT oi.order_id) AS order_count,
    SUM(oi.quantity) AS units_sold,
    SUM(oi.quantity * oi.price) AS revenue
FROM shopify_products p
JOIN shopify_order_items oi ON oi.product_id = p.id
JOIN shopify_orders o ON oi.order_id = o.id
WHERE o.financial_status = 'paid'
GROUP BY p.id, p.title, p.vendor, p.product_type
ORDER BY revenue DESC;
```

### shopify_low_inventory

```sql
CREATE VIEW shopify_low_inventory AS
SELECT
    p.id AS product_id,
    p.title AS product_title,
    v.id AS variant_id,
    v.title AS variant_title,
    v.sku,
    i.available,
    i.on_hand,
    l.name AS location
FROM shopify_inventory i
JOIN shopify_variants v ON v.inventory_item_id = i.inventory_item_id
JOIN shopify_products p ON v.product_id = p.id
JOIN shopify_locations l ON i.location_id = l.id
WHERE i.available < 10
ORDER BY i.available ASC;
```

### shopify_customer_value

```sql
CREATE VIEW shopify_customer_value AS
SELECT
    c.id,
    c.email,
    c.first_name,
    c.last_name,
    c.orders_count,
    c.total_spent,
    CASE
        WHEN c.total_spent >= 1000 THEN 'VIP'
        WHEN c.total_spent >= 500 THEN 'Premium'
        WHEN c.total_spent >= 100 THEN 'Regular'
        ELSE 'New'
    END AS customer_tier,
    c.created_at AS customer_since
FROM shopify_customers c
ORDER BY c.total_spent DESC;
```

### shopify_monthly_revenue

```sql
CREATE VIEW shopify_monthly_revenue AS
SELECT
    DATE_TRUNC('month', created_at) AS month,
    COUNT(*) AS order_count,
    SUM(total_price) AS revenue,
    AVG(total_price) AS avg_order_value,
    COUNT(DISTINCT customer_id) AS unique_customers
FROM shopify_orders
WHERE financial_status = 'paid'
GROUP BY DATE_TRUNC('month', created_at)
ORDER BY month DESC;
```

---

## Performance Considerations

### Rate Limiting Strategy

Shopify enforces strict rate limits to protect their infrastructure:

| API Type | Rate Limit | Bucket Size | Notes |
|----------|------------|-------------|-------|
| Admin REST API | 2 requests/second | 40 requests | Refills at 2/sec |
| GraphQL API | 1000 points/second | 50 points/sec burst | Cost varies by query |
| Webhook Delivery | No limit | N/A | Must respond in < 5 seconds |

#### Rate Limiter Implementation

```typescript
import { RateLimiter } from '@nself/plugin-utils';

export class ShopifyClient {
  private rateLimiter: RateLimiter;

  constructor() {
    // Conservative limit: 2 requests/second
    this.rateLimiter = new RateLimiter(2);
  }

  async makeRequest<T>(endpoint: string): Promise<T> {
    // Wait for rate limit bucket
    await this.rateLimiter.acquire();

    const response = await this.http.get<T>(endpoint);

    // Parse rate limit headers
    const remaining = response.headers.get('X-Shopify-Shop-Api-Call-Limit');
    if (remaining) {
      const [used, total] = remaining.split('/').map(Number);

      // If approaching limit, add delay
      if (used > total * 0.8) {
        await new Promise(resolve => setTimeout(resolve, 1000));
      }
    }

    return response.data;
  }
}
```

#### Best Practices for Large Stores

```bash
# Use incremental sync for stores with 10k+ products
nself-shopify sync --incremental --since 2024-01-01

# Sync specific resources to reduce API calls
nself-shopify sync --resources products,inventory

# Schedule syncs during off-peak hours
0 3 * * * /usr/local/bin/nself-shopify sync --incremental

# Use webhooks for real-time updates instead of polling
nself-shopify server --port 3003
```

### Database Optimization

#### Index Strategy

```sql
-- Orders performance indexes
CREATE INDEX CONCURRENTLY idx_shopify_orders_customer_id
    ON shopify_orders(customer_id) WHERE customer_id IS NOT NULL;
CREATE INDEX CONCURRENTLY idx_shopify_orders_created_at
    ON shopify_orders(created_at DESC);
CREATE INDEX CONCURRENTLY idx_shopify_orders_financial_status
    ON shopify_orders(financial_status);
CREATE INDEX CONCURRENTLY idx_shopify_orders_fulfillment_status
    ON shopify_orders(fulfillment_status);

-- Products performance indexes
CREATE INDEX CONCURRENTLY idx_shopify_products_vendor
    ON shopify_products(vendor);
CREATE INDEX CONCURRENTLY idx_shopify_products_product_type
    ON shopify_products(product_type);
CREATE INDEX CONCURRENTLY idx_shopify_products_status
    ON shopify_products(status);
CREATE INDEX CONCURRENTLY idx_shopify_products_created_at
    ON shopify_products(created_at DESC);

-- Full-text search on products
CREATE INDEX CONCURRENTLY idx_shopify_products_search
    ON shopify_products USING gin(to_tsvector('english',
        COALESCE(title, '') || ' ' ||
        COALESCE(body_html, '') || ' ' ||
        COALESCE(tags, '')
    ));

-- Customers performance indexes
CREATE INDEX CONCURRENTLY idx_shopify_customers_email
    ON shopify_customers(email);
CREATE INDEX CONCURRENTLY idx_shopify_customers_orders_count
    ON shopify_customers(orders_count DESC);
CREATE INDEX CONCURRENTLY idx_shopify_customers_total_spent
    ON shopify_customers(total_spent DESC);

-- Order items performance
CREATE INDEX CONCURRENTLY idx_shopify_order_items_order_id
    ON shopify_order_items(order_id);
CREATE INDEX CONCURRENTLY idx_shopify_order_items_product_id
    ON shopify_order_items(product_id);
CREATE INDEX CONCURRENTLY idx_shopify_order_items_variant_id
    ON shopify_order_items(variant_id);

-- Inventory tracking
CREATE INDEX CONCURRENTLY idx_shopify_inventory_location_id
    ON shopify_inventory(location_id);
CREATE INDEX CONCURRENTLY idx_shopify_inventory_available
    ON shopify_inventory(available) WHERE available < 10;
```

#### Partitioning for Large Datasets

For stores with millions of orders:

```sql
-- Partition orders by year
CREATE TABLE shopify_orders_2024 PARTITION OF shopify_orders
    FOR VALUES FROM ('2024-01-01') TO ('2025-01-01');

CREATE TABLE shopify_orders_2025 PARTITION OF shopify_orders
    FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');

CREATE TABLE shopify_orders_2026 PARTITION OF shopify_orders
    FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');

-- Automatically create partitions
CREATE OR REPLACE FUNCTION create_order_partition()
RETURNS void AS $$
DECLARE
    partition_year INTEGER;
    partition_name TEXT;
    start_date TEXT;
    end_date TEXT;
BEGIN
    partition_year := EXTRACT(YEAR FROM NOW() + INTERVAL '1 year');
    partition_name := 'shopify_orders_' || partition_year;
    start_date := partition_year || '-01-01';
    end_date := (partition_year + 1) || '-01-01';

    EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF shopify_orders
                   FOR VALUES FROM (%L) TO (%L)',
                   partition_name, start_date, end_date);
END;
$$ LANGUAGE plpgsql;
```

### Bulk Operations

#### Batch Upserts for Faster Sync

```typescript
async upsertProductsBatch(products: ShopifyProduct[]): Promise<void> {
  const batchSize = 500;

  for (let i = 0; i < products.length; i += batchSize) {
    const batch = products.slice(i, i + batchSize);

    // Build multi-value INSERT with ON CONFLICT
    const values = batch.map((p, idx) => {
      const offset = idx * 10;
      return `($${offset+1}, $${offset+2}, $${offset+3}, $${offset+4},
               $${offset+5}, $${offset+6}, $${offset+7}, $${offset+8},
               $${offset+9}, NOW())`;
    }).join(',');

    const params = batch.flatMap(p => [
      p.id, p.title, p.body_html, p.vendor,
      p.product_type, p.handle, p.status, p.tags,
      JSON.stringify(p.images)
    ]);

    await this.db.execute(`
      INSERT INTO shopify_products
        (id, title, body_html, vendor, product_type, handle,
         status, tags, images, synced_at)
      VALUES ${values}
      ON CONFLICT (id) DO UPDATE SET
        title = EXCLUDED.title,
        body_html = EXCLUDED.body_html,
        vendor = EXCLUDED.vendor,
        product_type = EXCLUDED.product_type,
        handle = EXCLUDED.handle,
        status = EXCLUDED.status,
        tags = EXCLUDED.tags,
        images = EXCLUDED.images,
        synced_at = NOW()
    `, params);

    this.logger.debug(`Upserted batch ${i/batchSize + 1}/${Math.ceil(products.length/batchSize)}`);
  }
}
```

#### Connection Pooling

```typescript
import { Pool } from 'pg';

const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
  max: 20, // Maximum pool size
  idleTimeoutMillis: 30000,
  connectionTimeoutMillis: 2000,
});

// Use pool for all queries
export class ShopifyDatabase {
  async query(sql: string, params?: any[]) {
    const client = await pool.connect();
    try {
      return await client.query(sql, params);
    } finally {
      client.release();
    }
  }
}
```

---

## Security Notes

### Access Token Security

Shopify access tokens are permanent and provide full access to your store data:

#### Best Practices

1. **Environment Variables Only** - Never hardcode tokens
2. **Restricted Scopes** - Request only the scopes you need
3. **Token Rotation** - Periodically regenerate access tokens
4. **Audit Logging** - Log all API access with tokens
5. **Secure Storage** - Use secret management (AWS Secrets Manager, HashiCorp Vault)

```bash
# Bad - Token in code
SHOPIFY_ACCESS_TOKEN=shpat_xxxxx node sync.js

# Good - Token in environment
export SHOPIFY_ACCESS_TOKEN=$(aws secretsmanager get-secret-value \
  --secret-id shopify/access-token --query SecretString --output text)
nself-shopify sync
```

#### Token Detection Prevention

Add to `.gitignore`:

```gitignore
.env
.env.*
*.secret
*_secret
config/secrets.json
credentials.env
```

Pre-commit hook to detect tokens:

```bash
#!/bin/bash
# .git/hooks/pre-commit

if git diff --cached | grep -E 'shpat_[a-zA-Z0-9]{32}'; then
  echo "ERROR: Shopify access token detected in commit!"
  exit 1
fi
```

### Webhook HMAC Verification

Always verify webhook signatures to prevent spoofing:

```typescript
import crypto from 'crypto';

function verifyShopifyWebhook(
  rawBody: string,
  hmacHeader: string,
  secret: string
): boolean {
  const computed = crypto
    .createHmac('sha256', secret)
    .update(rawBody, 'utf8')
    .digest('base64');

  // Timing-safe comparison to prevent timing attacks
  return crypto.timingSafeEqual(
    Buffer.from(hmacHeader),
    Buffer.from(computed)
  );
}

// Fastify example
fastify.post('/webhook', async (request, reply) => {
  const hmac = request.headers['x-shopify-hmac-sha256'];
  const rawBody = request.rawBody; // Must be raw, not parsed

  if (!verifyShopifyWebhook(rawBody, hmac, process.env.SHOPIFY_WEBHOOK_SECRET)) {
    return reply.code(401).send({ error: 'Invalid signature' });
  }

  // Process webhook...
});
```

#### Webhook Security Checklist

- [ ] Always verify HMAC signature
- [ ] Use timing-safe comparison
- [ ] Validate webhook is from correct shop domain
- [ ] Check webhook timestamp to prevent replay attacks
- [ ] Store raw payload for debugging
- [ ] Implement idempotency (duplicate event handling)
- [ ] Rate limit webhook endpoint
- [ ] Return 200 OK within 5 seconds (async processing)

### PCI Compliance Considerations

The Shopify plugin does **not** store payment card data. However:

#### What is Stored

- Order totals and payment status
- Payment gateway names (e.g., "Shopify Payments")
- Transaction IDs
- Customer email and shipping addresses

#### What is NOT Stored

- Full credit card numbers
- CVV codes
- Card expiration dates
- Bank account information

#### Data Retention Policy

```sql
-- Anonymize customer data after 2 years (GDPR compliance)
UPDATE shopify_customers
SET
    email = 'deleted_' || id || '@example.com',
    first_name = 'Deleted',
    last_name = 'User',
    phone = NULL,
    note = NULL,
    default_address = NULL,
    addresses = '[]'
WHERE updated_at < NOW() - INTERVAL '2 years'
  AND state != 'disabled';

-- Archive old orders
CREATE TABLE shopify_orders_archive AS
SELECT * FROM shopify_orders
WHERE created_at < NOW() - INTERVAL '7 years';

DELETE FROM shopify_orders
WHERE created_at < NOW() - INTERVAL '7 years';
```

### Database Security

```sql
-- Create read-only user for analytics
CREATE USER shopify_readonly WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE nself TO shopify_readonly;
GRANT USAGE ON SCHEMA public TO shopify_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO shopify_readonly;

-- Prevent data modification
REVOKE INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public FROM shopify_readonly;

-- Row-level security (if multi-tenant)
ALTER TABLE shopify_orders ENABLE ROW LEVEL SECURITY;

CREATE POLICY shop_isolation ON shopify_orders
    FOR ALL
    USING (shop_id = current_setting('app.current_shop_id')::BIGINT);
```

---

## Advanced Code Examples

### 1. Intelligent Inventory Management

Predict stockouts and automate reordering:

```typescript
interface InventoryAlert {
  product_id: number;
  variant_id: number;
  sku: string;
  current_stock: number;
  avg_daily_sales: number;
  days_until_stockout: number;
  recommended_reorder_quantity: number;
}

async function generateInventoryAlerts(): Promise<InventoryAlert[]> {
  const sql = `
    WITH daily_sales AS (
      -- Calculate average daily sales over last 30 days
      SELECT
        oi.product_id,
        oi.variant_id,
        oi.sku,
        AVG(daily_quantity) AS avg_daily_sales
      FROM (
        SELECT
          product_id,
          variant_id,
          sku,
          DATE(o.created_at) AS sale_date,
          SUM(quantity) AS daily_quantity
        FROM shopify_order_items oi
        JOIN shopify_orders o ON oi.order_id = o.id
        WHERE o.financial_status = 'paid'
          AND o.created_at > NOW() - INTERVAL '30 days'
        GROUP BY product_id, variant_id, sku, DATE(o.created_at)
      ) daily_totals
      GROUP BY product_id, variant_id, sku
    ),
    current_inventory AS (
      SELECT
        v.product_id,
        v.id AS variant_id,
        v.sku,
        SUM(i.available) AS total_available
      FROM shopify_variants v
      JOIN shopify_inventory i ON v.inventory_item_id = i.inventory_item_id
      GROUP BY v.product_id, v.id, v.sku
    )
    SELECT
      ci.product_id,
      ci.variant_id,
      ci.sku,
      ci.total_available AS current_stock,
      COALESCE(ds.avg_daily_sales, 0) AS avg_daily_sales,
      CASE
        WHEN COALESCE(ds.avg_daily_sales, 0) > 0
        THEN ci.total_available / ds.avg_daily_sales
        ELSE 999
      END AS days_until_stockout,
      -- Recommend 30 days of stock
      GREATEST(0, CEIL(ds.avg_daily_sales * 30) - ci.total_available)
        AS recommended_reorder_quantity
    FROM current_inventory ci
    LEFT JOIN daily_sales ds USING (product_id, variant_id, sku)
    WHERE ci.total_available < ds.avg_daily_sales * 14 -- Alert at 2 weeks
    ORDER BY days_until_stockout ASC;
  `;

  const results = await db.query(sql);
  return results.rows;
}

// Send alerts via email/Slack
async function sendLowStockAlerts() {
  const alerts = await generateInventoryAlerts();

  if (alerts.length === 0) {
    logger.info('No inventory alerts');
    return;
  }

  const message = alerts
    .map(a =>
      `⚠️ SKU ${a.sku}: ${a.current_stock} units (${a.days_until_stockout.toFixed(1)} days left)\n` +
      `   Recommendation: Order ${a.recommended_reorder_quantity} units`
    )
    .join('\n');

  await sendSlackMessage('#inventory-alerts', message);
}
```

### 2. Advanced Customer Segmentation

RFM (Recency, Frequency, Monetary) analysis:

```sql
-- RFM Customer Segmentation
WITH rfm_metrics AS (
  SELECT
    c.id AS customer_id,
    c.email,
    c.first_name,
    c.last_name,
    -- Recency: days since last order
    DATE_PART('day', NOW() - MAX(o.created_at)) AS recency_days,
    -- Frequency: number of orders
    COUNT(DISTINCT o.id) AS frequency,
    -- Monetary: total spend
    SUM(o.total_price) AS monetary
  FROM shopify_customers c
  LEFT JOIN shopify_orders o ON c.id = o.customer_id
  WHERE o.financial_status = 'paid'
  GROUP BY c.id, c.email, c.first_name, c.last_name
),
rfm_scores AS (
  SELECT
    *,
    -- Score 1-5 for each dimension
    NTILE(5) OVER (ORDER BY recency_days DESC) AS r_score,
    NTILE(5) OVER (ORDER BY frequency ASC) AS f_score,
    NTILE(5) OVER (ORDER BY monetary ASC) AS m_score
  FROM rfm_metrics
)
SELECT
  customer_id,
  email,
  first_name,
  last_name,
  recency_days,
  frequency,
  monetary,
  r_score,
  f_score,
  m_score,
  -- Combined RFM score
  (r_score + f_score + m_score) AS rfm_total,
  -- Segment classification
  CASE
    WHEN r_score >= 4 AND f_score >= 4 AND m_score >= 4 THEN 'Champions'
    WHEN r_score >= 3 AND f_score >= 3 AND m_score >= 3 THEN 'Loyal Customers'
    WHEN r_score >= 4 AND f_score <= 2 THEN 'New Customers'
    WHEN r_score <= 2 AND f_score >= 3 THEN 'At Risk'
    WHEN r_score <= 2 AND f_score <= 2 THEN 'Lost'
    WHEN m_score >= 4 THEN 'Big Spenders'
    ELSE 'Regular'
  END AS segment
FROM rfm_scores
ORDER BY rfm_total DESC;
```

### 3. Revenue Attribution Analytics

Track which marketing channels drive sales:

```typescript
async function analyzeRevenueAttribution(startDate: Date, endDate: Date) {
  const sql = `
    SELECT
      -- Extract UTM source from landing_site
      CASE
        WHEN landing_site LIKE '%utm_source=facebook%' THEN 'Facebook'
        WHEN landing_site LIKE '%utm_source=google%' THEN 'Google'
        WHEN landing_site LIKE '%utm_source=instagram%' THEN 'Instagram'
        WHEN landing_site LIKE '%utm_source=email%' THEN 'Email'
        WHEN referring_site LIKE '%google.com%' THEN 'Google Organic'
        WHEN referring_site LIKE '%facebook.com%' THEN 'Facebook Organic'
        WHEN referring_site IS NULL OR referring_site = '' THEN 'Direct'
        ELSE 'Other'
      END AS channel,
      COUNT(DISTINCT id) AS order_count,
      COUNT(DISTINCT customer_id) AS unique_customers,
      SUM(total_price) AS revenue,
      AVG(total_price) AS avg_order_value,
      SUM(total_price) / COUNT(DISTINCT customer_id) AS customer_lifetime_value
    FROM shopify_orders
    WHERE financial_status = 'paid'
      AND created_at BETWEEN $1 AND $2
    GROUP BY channel
    ORDER BY revenue DESC;
  `;

  return await db.query(sql, [startDate, endDate]);
}

// Multi-touch attribution (first and last touch)
async function multiTouchAttribution() {
  const sql = `
    WITH customer_touchpoints AS (
      SELECT
        customer_id,
        id AS order_id,
        created_at,
        total_price,
        CASE
          WHEN landing_site LIKE '%utm_source=%'
          THEN regexp_replace(landing_site, '.*utm_source=([^&]+).*', '\\1')
          ELSE 'direct'
        END AS source,
        ROW_NUMBER() OVER (PARTITION BY customer_id ORDER BY created_at ASC) AS touch_rank_first,
        ROW_NUMBER() OVER (PARTITION BY customer_id ORDER BY created_at DESC) AS touch_rank_last
      FROM shopify_orders
      WHERE financial_status = 'paid'
    )
    SELECT
      source,
      -- First-touch attribution
      COUNT(*) FILTER (WHERE touch_rank_first = 1) AS first_touch_orders,
      SUM(total_price) FILTER (WHERE touch_rank_first = 1) AS first_touch_revenue,
      -- Last-touch attribution
      COUNT(*) FILTER (WHERE touch_rank_last = 1) AS last_touch_orders,
      SUM(total_price) FILTER (WHERE touch_rank_last = 1) AS last_touch_revenue
    FROM customer_touchpoints
    GROUP BY source
    ORDER BY first_touch_revenue DESC;
  `;

  return await db.query(sql);
}
```

### 4. Abandoned Cart Recovery System

Automate recovery emails with personalization:

```typescript
interface AbandonedCart {
  checkout_id: string;
  email: string;
  customer_name: string;
  cart_value: number;
  items: Array<{
    product_title: string;
    variant_title: string;
    price: number;
    quantity: number;
  }>;
  abandoned_hours_ago: number;
  recovery_url: string;
}

async function findAbandonedCarts(hoursAgo: number = 2): Promise<AbandonedCart[]> {
  const sql = `
    SELECT
      c.id AS checkout_id,
      c.email,
      COALESCE(c.customer_first_name || ' ' || c.customer_last_name, c.email) AS customer_name,
      c.total_price AS cart_value,
      c.line_items AS items,
      EXTRACT(EPOCH FROM (NOW() - c.created_at)) / 3600 AS abandoned_hours_ago,
      c.abandoned_checkout_url AS recovery_url
    FROM shopify_checkouts c
    WHERE c.completed_at IS NULL
      AND c.email IS NOT NULL
      AND c.created_at > NOW() - INTERVAL '24 hours'
      AND c.created_at < NOW() - INTERVAL '${hoursAgo} hours'
      -- Exclude if customer already ordered
      AND NOT EXISTS (
        SELECT 1 FROM shopify_orders o
        WHERE o.email = c.email
          AND o.created_at > c.created_at
      )
    ORDER BY c.total_price DESC;
  `;

  const result = await db.query(sql);
  return result.rows;
}

// Send recovery emails
async function sendAbandonedCartEmails() {
  const carts = await findAbandonedCarts(2);

  for (const cart of carts) {
    const emailTemplate = `
      Hi ${cart.customer_name},

      You left ${cart.items.length} item(s) in your cart worth $${cart.cart_value}:

      ${cart.items.map(item =>
        `- ${item.product_title} (${item.variant_title}) - $${item.price} x ${item.quantity}`
      ).join('\n')}

      Complete your purchase: ${cart.recovery_url}

      Use code COMEBACK10 for 10% off!
    `;

    await sendEmail({
      to: cart.email,
      subject: `You left ${cart.items.length} items in your cart`,
      body: emailTemplate
    });

    logger.info(`Sent recovery email to ${cart.email} (${cart.cart_value})`);
  }
}

// Schedule recovery campaign
// 2 hours: First reminder
// 24 hours: Second reminder with discount
// 3 days: Final reminder with urgency
```

### 5. Cohort Analysis

Track customer retention by signup cohort:

```sql
-- Monthly cohort retention analysis
WITH customer_cohorts AS (
  SELECT
    id AS customer_id,
    DATE_TRUNC('month', created_at) AS cohort_month
  FROM shopify_customers
),
customer_orders AS (
  SELECT
    o.customer_id,
    DATE_TRUNC('month', o.created_at) AS order_month,
    SUM(o.total_price) AS month_revenue
  FROM shopify_orders o
  WHERE o.financial_status = 'paid'
  GROUP BY o.customer_id, DATE_TRUNC('month', o.created_at)
)
SELECT
  cc.cohort_month,
  DATE_PART('month', AGE(co.order_month, cc.cohort_month)) AS months_since_signup,
  COUNT(DISTINCT co.customer_id) AS active_customers,
  SUM(co.month_revenue) AS cohort_revenue,
  -- Retention rate
  COUNT(DISTINCT co.customer_id)::FLOAT /
    NULLIF(COUNT(DISTINCT cc.customer_id), 0) * 100 AS retention_rate
FROM customer_cohorts cc
LEFT JOIN customer_orders co ON cc.customer_id = co.customer_id
GROUP BY cc.cohort_month, months_since_signup
ORDER BY cc.cohort_month DESC, months_since_signup ASC;
```

---

## Monitoring & Alerting

### Store Health Dashboard

Real-time metrics for store performance:

```typescript
interface StoreHealthMetrics {
  realtime: {
    orders_last_hour: number;
    revenue_last_hour: number;
    avg_order_value_last_hour: number;
  };
  today: {
    orders: number;
    revenue: number;
    unique_customers: number;
    conversion_rate: number;
  };
  week: {
    orders: number;
    revenue: number;
    growth_vs_last_week: number;
  };
  inventory: {
    products_low_stock: number;
    products_out_of_stock: number;
    inventory_value: number;
  };
  webhooks: {
    events_last_hour: number;
    failed_events_last_hour: number;
    avg_processing_time_ms: number;
  };
}

async function getStoreHealthMetrics(): Promise<StoreHealthMetrics> {
  const sql = `
    SELECT
      -- Real-time (last hour)
      COUNT(*) FILTER (WHERE o.created_at > NOW() - INTERVAL '1 hour') AS orders_last_hour,
      COALESCE(SUM(o.total_price) FILTER (WHERE o.created_at > NOW() - INTERVAL '1 hour'), 0) AS revenue_last_hour,
      COALESCE(AVG(o.total_price) FILTER (WHERE o.created_at > NOW() - INTERVAL '1 hour'), 0) AS avg_order_value_last_hour,

      -- Today
      COUNT(*) FILTER (WHERE DATE(o.created_at) = CURRENT_DATE) AS orders_today,
      COALESCE(SUM(o.total_price) FILTER (WHERE DATE(o.created_at) = CURRENT_DATE), 0) AS revenue_today,
      COUNT(DISTINCT o.customer_id) FILTER (WHERE DATE(o.created_at) = CURRENT_DATE) AS customers_today,

      -- This week
      COUNT(*) FILTER (WHERE o.created_at > DATE_TRUNC('week', NOW())) AS orders_week,
      COALESCE(SUM(o.total_price) FILTER (WHERE o.created_at > DATE_TRUNC('week', NOW())), 0) AS revenue_week,

      -- Last week (for comparison)
      COUNT(*) FILTER (
        WHERE o.created_at > DATE_TRUNC('week', NOW()) - INTERVAL '1 week'
          AND o.created_at < DATE_TRUNC('week', NOW())
      ) AS orders_last_week,
      COALESCE(SUM(o.total_price) FILTER (
        WHERE o.created_at > DATE_TRUNC('week', NOW()) - INTERVAL '1 week'
          AND o.created_at < DATE_TRUNC('week', NOW())
      ), 0) AS revenue_last_week
    FROM shopify_orders o
    WHERE o.financial_status = 'paid';
  `;

  const orderMetrics = await db.query(sql);

  // Inventory metrics
  const inventoryMetrics = await db.query(`
    SELECT
      COUNT(*) FILTER (WHERE i.available < 10 AND i.available > 0) AS low_stock_count,
      COUNT(*) FILTER (WHERE i.available = 0) AS out_of_stock_count,
      SUM(i.available * v.price) AS inventory_value
    FROM shopify_inventory i
    JOIN shopify_variants v ON v.inventory_item_id = i.inventory_item_id;
  `);

  // Webhook metrics
  const webhookMetrics = await db.query(`
    SELECT
      COUNT(*) FILTER (WHERE received_at > NOW() - INTERVAL '1 hour') AS events_last_hour,
      COUNT(*) FILTER (
        WHERE received_at > NOW() - INTERVAL '1 hour'
          AND processed = false
      ) AS failed_last_hour,
      AVG(EXTRACT(EPOCH FROM (processed_at - received_at)) * 1000) AS avg_processing_ms
    FROM shopify_webhook_events
    WHERE received_at > NOW() - INTERVAL '24 hours';
  `);

  const o = orderMetrics.rows[0];
  const i = inventoryMetrics.rows[0];
  const w = webhookMetrics.rows[0];

  return {
    realtime: {
      orders_last_hour: o.orders_last_hour,
      revenue_last_hour: parseFloat(o.revenue_last_hour),
      avg_order_value_last_hour: parseFloat(o.avg_order_value_last_hour),
    },
    today: {
      orders: o.orders_today,
      revenue: parseFloat(o.revenue_today),
      unique_customers: o.customers_today,
      conversion_rate: 0, // Calculate from checkouts vs orders
    },
    week: {
      orders: o.orders_week,
      revenue: parseFloat(o.revenue_week),
      growth_vs_last_week:
        ((o.revenue_week - o.revenue_last_week) / o.revenue_last_week * 100) || 0,
    },
    inventory: {
      products_low_stock: i.low_stock_count,
      products_out_of_stock: i.out_of_stock_count,
      inventory_value: parseFloat(i.inventory_value),
    },
    webhooks: {
      events_last_hour: w.events_last_hour,
      failed_events_last_hour: w.failed_last_hour,
      avg_processing_time_ms: parseFloat(w.avg_processing_ms) || 0,
    },
  };
}
```

### Sales Metrics Tracking

Set up automated alerts for sales anomalies:

```typescript
interface SalesAlert {
  type: 'spike' | 'drop' | 'threshold';
  severity: 'info' | 'warning' | 'critical';
  metric: string;
  current_value: number;
  expected_value: number;
  deviation_pct: number;
  message: string;
}

async function detectSalesAnomalies(): Promise<SalesAlert[]> {
  const alerts: SalesAlert[] = [];

  // Compare current hour to same hour last week
  const hourlyComparison = await db.query(`
    WITH current_hour AS (
      SELECT
        COUNT(*) AS orders,
        COALESCE(SUM(total_price), 0) AS revenue
      FROM shopify_orders
      WHERE created_at > NOW() - INTERVAL '1 hour'
        AND financial_status = 'paid'
    ),
    last_week_hour AS (
      SELECT
        COUNT(*) AS orders,
        COALESCE(SUM(total_price), 0) AS revenue
      FROM shopify_orders
      WHERE created_at > NOW() - INTERVAL '1 week' - INTERVAL '1 hour'
        AND created_at < NOW() - INTERVAL '1 week'
        AND financial_status = 'paid'
    )
    SELECT
      c.orders AS current_orders,
      c.revenue AS current_revenue,
      l.orders AS last_week_orders,
      l.revenue AS last_week_revenue,
      ((c.revenue - l.revenue) / NULLIF(l.revenue, 0) * 100) AS revenue_change_pct
    FROM current_hour c, last_week_hour l;
  `);

  const data = hourlyComparison.rows[0];

  // Alert on significant revenue drop
  if (data.revenue_change_pct < -50) {
    alerts.push({
      type: 'drop',
      severity: 'critical',
      metric: 'hourly_revenue',
      current_value: data.current_revenue,
      expected_value: data.last_week_revenue,
      deviation_pct: data.revenue_change_pct,
      message: `Revenue dropped ${Math.abs(data.revenue_change_pct).toFixed(1)}% vs last week`,
    });
  }

  // Alert on significant revenue spike (could indicate fraud)
  if (data.revenue_change_pct > 200) {
    alerts.push({
      type: 'spike',
      severity: 'warning',
      metric: 'hourly_revenue',
      current_value: data.current_revenue,
      expected_value: data.last_week_revenue,
      deviation_pct: data.revenue_change_pct,
      message: `Revenue spiked ${data.revenue_change_pct.toFixed(1)}% vs last week - possible fraud`,
    });
  }

  return alerts;
}

// Send alerts to Slack/PagerDuty
async function sendAlerts(alerts: SalesAlert[]) {
  for (const alert of alerts) {
    const emoji = alert.severity === 'critical' ? '🚨' : '⚠️';
    const message = `${emoji} ${alert.message}\nCurrent: $${alert.current_value} | Expected: $${alert.expected_value}`;

    if (alert.severity === 'critical') {
      await sendPagerDutyAlert(alert);
    }

    await sendSlackMessage('#sales-alerts', message);
  }
}
```

### Inventory Alerts

Proactive notifications for inventory issues:

```sql
-- Critical inventory alerts view
CREATE VIEW shopify_inventory_alerts AS
WITH sales_velocity AS (
  SELECT
    oi.variant_id,
    AVG(daily_qty) AS avg_daily_sales
  FROM (
    SELECT
      oi.variant_id,
      DATE(o.created_at) AS sale_date,
      SUM(oi.quantity) AS daily_qty
    FROM shopify_order_items oi
    JOIN shopify_orders o ON oi.order_id = o.id
    WHERE o.financial_status = 'paid'
      AND o.created_at > NOW() - INTERVAL '14 days'
    GROUP BY oi.variant_id, DATE(o.created_at)
  ) daily
  GROUP BY variant_id
)
SELECT
  p.id AS product_id,
  p.title AS product_title,
  v.id AS variant_id,
  v.title AS variant_title,
  v.sku,
  i.available,
  sv.avg_daily_sales,
  CASE
    WHEN i.available = 0 THEN 'OUT_OF_STOCK'
    WHEN i.available < sv.avg_daily_sales * 3 THEN 'CRITICAL_LOW'
    WHEN i.available < sv.avg_daily_sales * 7 THEN 'LOW'
    ELSE 'OK'
  END AS alert_level,
  CASE
    WHEN sv.avg_daily_sales > 0
    THEN i.available / sv.avg_daily_sales
    ELSE 999
  END AS days_of_stock,
  CEIL(sv.avg_daily_sales * 30 - i.available) AS recommended_reorder_qty
FROM shopify_inventory i
JOIN shopify_variants v ON v.inventory_item_id = i.inventory_item_id
JOIN shopify_products p ON v.product_id = p.id
LEFT JOIN sales_velocity sv ON sv.variant_id = v.id
WHERE i.available < 50 OR sv.avg_daily_sales > i.available / 7
ORDER BY
  CASE alert_level
    WHEN 'OUT_OF_STOCK' THEN 1
    WHEN 'CRITICAL_LOW' THEN 2
    WHEN 'LOW' THEN 3
    ELSE 4
  END,
  days_of_stock ASC;
```

---

## Use Cases

### 1. Real-Time Inventory Management

Keep inventory in sync across systems:

```sql
-- Current inventory levels by product
SELECT
    p.title,
    v.sku,
    SUM(i.available) AS total_available,
    SUM(i.on_hand) AS total_on_hand
FROM shopify_inventory i
JOIN shopify_variants v ON v.inventory_item_id = i.inventory_item_id
JOIN shopify_products p ON v.product_id = p.id
GROUP BY p.title, v.sku
ORDER BY total_available ASC;
```

### 2. Customer Segmentation

Identify high-value customers:

```sql
-- Top 100 customers by lifetime value
SELECT
    email,
    first_name,
    last_name,
    orders_count,
    total_spent,
    created_at AS customer_since
FROM shopify_customers
WHERE orders_count > 0
ORDER BY total_spent DESC
LIMIT 100;
```

### 3. Sales Analytics

Track performance over time:

```sql
-- Daily sales for the last 30 days
SELECT
    DATE(created_at) AS date,
    COUNT(*) AS orders,
    SUM(total_price) AS revenue
FROM shopify_orders
WHERE financial_status = 'paid'
  AND created_at > NOW() - INTERVAL '30 days'
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

### 4. Product Performance

Analyze what's selling:

```sql
-- Best sellers this month
SELECT
    p.title,
    p.vendor,
    SUM(oi.quantity) AS units_sold,
    SUM(oi.quantity * oi.price) AS revenue
FROM shopify_order_items oi
JOIN shopify_products p ON oi.product_id = p.id
JOIN shopify_orders o ON oi.order_id = o.id
WHERE o.financial_status = 'paid'
  AND o.created_at > DATE_TRUNC('month', NOW())
GROUP BY p.id, p.title, p.vendor
ORDER BY revenue DESC
LIMIT 20;
```

### 5. Abandoned Checkout Recovery

Track abandoned carts:

```sql
-- Recent abandoned checkouts
SELECT
    email,
    total_price,
    created_at,
    abandoned_checkout_url
FROM shopify_checkouts
WHERE completed_at IS NULL
  AND created_at > NOW() - INTERVAL '7 days'
ORDER BY total_price DESC;
```

### 6. Cross-Sell Opportunities

Products frequently bought together:

```sql
-- Product affinity analysis
WITH order_pairs AS (
  SELECT
    a.product_id AS product_a,
    b.product_id AS product_b,
    COUNT(DISTINCT a.order_id) AS times_bought_together
  FROM shopify_order_items a
  JOIN shopify_order_items b ON a.order_id = b.order_id
  WHERE a.product_id < b.product_id -- Avoid duplicates
  GROUP BY a.product_id, b.product_id
  HAVING COUNT(DISTINCT a.order_id) >= 5
)
SELECT
  pa.title AS product_a_title,
  pb.title AS product_b_title,
  op.times_bought_together,
  op.times_bought_together::FLOAT /
    NULLIF((SELECT COUNT(*) FROM shopify_orders WHERE financial_status = 'paid'), 0) * 100
    AS affinity_pct
FROM order_pairs op
JOIN shopify_products pa ON op.product_a = pa.id
JOIN shopify_products pb ON op.product_b = pb.id
ORDER BY times_bought_together DESC
LIMIT 50;
```

### 7. Shipping Performance Analysis

Track fulfillment speed:

```sql
-- Average fulfillment time by location
SELECT
  l.name AS location,
  COUNT(DISTINCT f.id) AS fulfillment_count,
  AVG(EXTRACT(EPOCH FROM (f.created_at - o.created_at)) / 3600) AS avg_hours_to_fulfill,
  PERCENTILE_CONT(0.5) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (f.created_at - o.created_at)) / 3600
  ) AS median_hours_to_fulfill,
  COUNT(*) FILTER (
    WHERE f.created_at - o.created_at < INTERVAL '24 hours'
  )::FLOAT / NULLIF(COUNT(*), 0) * 100 AS same_day_fulfillment_pct
FROM shopify_fulfillments f
JOIN shopify_orders o ON f.order_id = o.id
JOIN shopify_locations l ON f.location_id = l.id
WHERE f.status = 'success'
  AND f.created_at > NOW() - INTERVAL '30 days'
GROUP BY l.id, l.name
ORDER BY avg_hours_to_fulfill ASC;
```

### 8. Discount Code Effectiveness

Measure ROI of discount campaigns:

```sql
-- Discount code performance
WITH discount_usage AS (
  SELECT
    dc->>'code' AS discount_code,
    dc->>'type' AS discount_type,
    (dc->>'amount')::NUMERIC AS discount_amount,
    o.id AS order_id,
    o.total_price,
    o.created_at
  FROM shopify_orders o,
  jsonb_array_elements(o.discount_codes) AS dc
  WHERE o.financial_status = 'paid'
    AND o.created_at > NOW() - INTERVAL '90 days'
)
SELECT
  discount_code,
  discount_type,
  COUNT(DISTINCT order_id) AS times_used,
  SUM(discount_amount) AS total_discount_given,
  SUM(total_price) AS total_revenue,
  SUM(total_price) / NULLIF(SUM(discount_amount), 0) AS roi_multiple,
  AVG(total_price) AS avg_order_value
FROM discount_usage
GROUP BY discount_code, discount_type
ORDER BY total_revenue DESC;
```

### 9. Customer Churn Prediction

Identify customers at risk of churning:

```sql
-- Customers who haven't ordered recently (potential churn)
WITH customer_last_order AS (
  SELECT
    c.id AS customer_id,
    c.email,
    c.first_name,
    c.last_name,
    c.total_spent,
    c.orders_count,
    MAX(o.created_at) AS last_order_date,
    DATE_PART('day', NOW() - MAX(o.created_at)) AS days_since_last_order,
    -- Calculate average days between orders
    CASE
      WHEN c.orders_count > 1 THEN
        (DATE_PART('day', MAX(o.created_at) - MIN(o.created_at))::FLOAT /
         NULLIF(c.orders_count - 1, 0))
      ELSE NULL
    END AS avg_days_between_orders
  FROM shopify_customers c
  JOIN shopify_orders o ON c.id = o.customer_id
  WHERE o.financial_status = 'paid'
  GROUP BY c.id, c.email, c.first_name, c.last_name, c.total_spent, c.orders_count
)
SELECT
  customer_id,
  email,
  first_name,
  last_name,
  total_spent,
  orders_count,
  last_order_date,
  days_since_last_order,
  avg_days_between_orders,
  CASE
    WHEN days_since_last_order > avg_days_between_orders * 2 THEN 'High Risk'
    WHEN days_since_last_order > avg_days_between_orders * 1.5 THEN 'Medium Risk'
    WHEN days_since_last_order > avg_days_between_orders THEN 'Low Risk'
    ELSE 'Active'
  END AS churn_risk
FROM customer_last_order
WHERE avg_days_between_orders IS NOT NULL
  AND days_since_last_order > avg_days_between_orders
ORDER BY
  CASE churn_risk
    WHEN 'High Risk' THEN 1
    WHEN 'Medium Risk' THEN 2
    WHEN 'Low Risk' THEN 3
    ELSE 4
  END,
  total_spent DESC;
```

### 10. Product Return Analysis

Identify products with high return rates:

```sql
-- Product return rates
WITH product_sales AS (
  SELECT
    oi.product_id,
    SUM(oi.quantity) AS total_units_sold
  FROM shopify_order_items oi
  JOIN shopify_orders o ON oi.order_id = o.id
  WHERE o.financial_status = 'paid'
    AND o.created_at > NOW() - INTERVAL '6 months'
  GROUP BY oi.product_id
),
product_returns AS (
  SELECT
    oi.product_id,
    COUNT(DISTINCT r.id) AS return_count,
    SUM((ri->>'quantity')::INTEGER) AS total_units_returned
  FROM shopify_refunds r
  JOIN shopify_orders o ON r.order_id = o.id
  JOIN shopify_order_items oi ON oi.order_id = o.id,
  jsonb_array_elements(r.refund_line_items) AS ri
  WHERE o.created_at > NOW() - INTERVAL '6 months'
    AND (ri->>'line_item_id')::BIGINT = oi.id
  GROUP BY oi.product_id
)
SELECT
  p.id,
  p.title,
  p.vendor,
  p.product_type,
  ps.total_units_sold,
  COALESCE(pr.total_units_returned, 0) AS units_returned,
  COALESCE(pr.total_units_returned::FLOAT / NULLIF(ps.total_units_sold, 0) * 100, 0) AS return_rate_pct,
  pr.return_count
FROM shopify_products p
LEFT JOIN product_sales ps ON p.id = ps.product_id
LEFT JOIN product_returns pr ON p.id = pr.product_id
WHERE ps.total_units_sold > 10 -- Minimum sales threshold
ORDER BY return_rate_pct DESC
LIMIT 50;
```

### 11. Customer Acquisition Cost (CAC) Analysis

Track marketing efficiency:

```sql
-- CAC by channel (requires marketing spend data)
WITH customer_acquisition AS (
  SELECT
    c.id AS customer_id,
    c.created_at,
    CASE
      WHEN o.landing_site LIKE '%utm_source=facebook%' THEN 'Facebook'
      WHEN o.landing_site LIKE '%utm_source=google%' THEN 'Google'
      WHEN o.landing_site LIKE '%utm_source=instagram%' THEN 'Instagram'
      WHEN o.referring_site LIKE '%google.com%' THEN 'Organic Search'
      ELSE 'Direct/Other'
    END AS acquisition_channel,
    c.total_spent AS lifetime_value
  FROM shopify_customers c
  LEFT JOIN LATERAL (
    SELECT landing_site, referring_site
    FROM shopify_orders
    WHERE customer_id = c.id
    ORDER BY created_at ASC
    LIMIT 1
  ) o ON true
  WHERE c.created_at > NOW() - INTERVAL '90 days'
)
SELECT
  acquisition_channel,
  COUNT(*) AS customers_acquired,
  AVG(lifetime_value) AS avg_ltv,
  SUM(lifetime_value) AS total_ltv
  -- Add: marketing_spend / customers_acquired AS cac
  -- Add: avg_ltv / cac AS ltv_to_cac_ratio
FROM customer_acquisition
GROUP BY acquisition_channel
ORDER BY customers_acquired DESC;
```

### 12. Seasonal Trends Analysis

Identify seasonal patterns:

```sql
-- Sales by day of week and hour
SELECT
  TO_CHAR(created_at, 'Day') AS day_of_week,
  EXTRACT(HOUR FROM created_at) AS hour_of_day,
  COUNT(*) AS order_count,
  SUM(total_price) AS revenue,
  AVG(total_price) AS avg_order_value
FROM shopify_orders
WHERE financial_status = 'paid'
  AND created_at > NOW() - INTERVAL '90 days'
GROUP BY day_of_week, hour_of_day
ORDER BY day_of_week, hour_of_day;

-- Year-over-year comparison
SELECT
  DATE_TRUNC('month', created_at) AS month,
  EXTRACT(YEAR FROM created_at) AS year,
  COUNT(*) AS orders,
  SUM(total_price) AS revenue,
  AVG(total_price) AS avg_order_value
FROM shopify_orders
WHERE financial_status = 'paid'
GROUP BY DATE_TRUNC('month', created_at), EXTRACT(YEAR FROM created_at)
ORDER BY month DESC, year DESC;
```

---

## TypeScript Implementation

The plugin is built with TypeScript for type safety and maintainability.

### Key Files

| File | Purpose |
|------|---------|
| `types.ts` | All type definitions for Shopify resources |
| `client.ts` | Shopify API client with pagination and rate limiting |
| `database.ts` | PostgreSQL operations with upsert support |
| `sync.ts` | Orchestrates full and incremental syncs |
| `webhooks.ts` | Webhook event handlers |
| `server.ts` | Fastify HTTP server |
| `cli.ts` | Commander.js CLI |

### API Client Example

```typescript
export class ShopifyClient {
  private http: HttpClient;
  private rateLimiter: RateLimiter;

  constructor(shopDomain: string, accessToken: string, apiVersion: string) {
    this.http = new HttpClient({
      baseUrl: `https://${shopDomain}/admin/api/${apiVersion}`,
      headers: {
        'X-Shopify-Access-Token': accessToken,
        'Content-Type': 'application/json',
      },
    });
    // Shopify allows 2 requests/second per app
    this.rateLimiter = new RateLimiter(2);
  }

  async listProducts(): Promise<ShopifyProduct[]> {
    const products: ShopifyProduct[] = [];
    let pageInfo: string | undefined;

    do {
      await this.rateLimiter.acquire();

      const params = pageInfo
        ? { page_info: pageInfo, limit: '250' }
        : { limit: '250' };

      const response = await this.http.get<{ products: any[] }>(
        '/products.json',
        params
      );

      products.push(...response.products.map(this.mapProduct));

      // Handle cursor-based pagination
      const linkHeader = response.headers?.get('link');
      pageInfo = this.extractNextPageInfo(linkHeader);
    } while (pageInfo);

    return products;
  }
}
```

### Rate Limiting

Shopify has strict rate limits (2 requests/second for Admin API):

```typescript
const rateLimiter = new RateLimiter(2);

// Before each API call
await rateLimiter.acquire();
const response = await shopifyApi.call();
```

---

## Troubleshooting

### Common Issues

#### Rate Limiting

```
Error: 429 Too Many Requests
```

**Solution**: The plugin includes built-in rate limiting. If you still hit limits:
- Use incremental sync instead of full sync
- Increase the sync interval
- Check for other apps using your API quota
- Monitor rate limit headers in responses

```typescript
// Check API call limit status
const response = await shopifyClient.get('/shop.json');
const limitHeader = response.headers['x-shopify-shop-api-call-limit'];
console.log(`API calls used: ${limitHeader}`); // e.g., "32/40"
```

#### Access Token Invalid

```
Error: 401 [API] Invalid API key or access token
```

**Solution**:
1. Verify your access token is correct
2. Check that the app is still installed
3. Regenerate the access token if needed
4. Ensure access token has required scopes

```bash
# Test token validity
curl -X GET "https://your-store.myshopify.com/admin/api/2024-01/shop.json" \
  -H "X-Shopify-Access-Token: shpat_xxxxx"
```

#### Webhook Signature Invalid

```
Error: Webhook signature verification failed
```

**Solution**:
1. Verify `SHOPIFY_WEBHOOK_SECRET` matches the secret from Shopify
2. Ensure the raw request body is used for verification (not parsed JSON)
3. Check that no proxy is modifying the request
4. Verify webhook secret hasn't been regenerated

```typescript
// Debug webhook signature verification
app.post('/webhook', { rawBody: true }, async (req, reply) => {
  const hmac = req.headers['x-shopify-hmac-sha256'];
  const rawBody = req.rawBody; // Must be Buffer, not parsed JSON

  console.log('HMAC header:', hmac);
  console.log('Raw body length:', rawBody.length);
  console.log('Secret:', process.env.SHOPIFY_WEBHOOK_SECRET.substring(0, 10) + '...');

  // Verify...
});
```

#### Missing Orders

```
Only seeing orders from the last 60 days
```

**Solution**: Request the `read_all_orders` scope to access older orders. This requires Shopify approval for production apps.

```bash
# Check if you have read_all_orders scope
curl -X GET "https://your-store.myshopify.com/admin/api/2024-01/orders.json?limit=1&created_at_min=2020-01-01" \
  -H "X-Shopify-Access-Token: shpat_xxxxx"
```

#### API Version Mismatch

```
Error: API version not supported
```

**Solution**: Update `SHOPIFY_API_VERSION` to a supported version. Check [Shopify's API versioning docs](https://shopify.dev/docs/api/usage/versioning).

Supported versions (as of 2026-01):
- `2024-01` (Stable)
- `2024-04` (Stable)
- `2024-07` (Stable)
- `2024-10` (Release candidate)

#### Pagination Issues

```
Error: Missing page_info parameter
```

**Solution**: Shopify uses cursor-based pagination. Extract the `link` header:

```typescript
function extractNextPageInfo(linkHeader: string): string | undefined {
  if (!linkHeader) return undefined;

  const match = linkHeader.match(/<([^>]+)>; rel="next"/);
  if (!match) return undefined;

  const url = new URL(match[1]);
  return url.searchParams.get('page_info') || undefined;
}
```

#### Database Connection Pool Exhausted

```
Error: sorry, too many clients already
```

**Solution**: Increase pool size or reduce concurrent operations:

```typescript
const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
  max: 20, // Increase from default 10
  idleTimeoutMillis: 30000,
  connectionTimeoutMillis: 5000,
});
```

#### Webhook Event Duplication

```
Receiving duplicate webhook events
```

**Solution**: Implement idempotency using webhook ID:

```typescript
async function processWebhook(event: WebhookEvent) {
  const webhookId = event.headers['x-shopify-webhook-id'];

  // Check if already processed
  const existing = await db.query(
    'SELECT id FROM shopify_webhook_events WHERE id = $1',
    [webhookId]
  );

  if (existing.rows.length > 0) {
    logger.warn(`Duplicate webhook ${webhookId} - skipping`);
    return;
  }

  // Store and process...
}
```

#### Large Product Catalog Sync Timeout

```
Sync takes hours or times out
```

**Solution**: Use pagination and batching:

```bash
# Sync products in batches
nself-shopify sync --resources products --batch-size 250

# Resume from last sync point
nself-shopify sync --incremental --since "2024-01-24T12:00:00Z"

# Sync specific product IDs
nself-shopify products sync --ids 12345,67890,11121
```

#### JSON/JSONB Column Errors

```
Error: cannot extract field from a non-object
```

**Solution**: Always check JSONB structure before querying:

```sql
-- Safe JSONB access
SELECT
  id,
  CASE
    WHEN jsonb_typeof(metadata) = 'object'
    THEN metadata->>'key'
    ELSE NULL
  END AS metadata_value
FROM shopify_products;

-- Use COALESCE for default values
SELECT
  COALESCE(discount_codes->0->>'code', 'NO_DISCOUNT') AS first_discount
FROM shopify_orders;
```

#### Webhook Endpoint Not Receiving Events

```
Webhooks configured but no events received
```

**Checklist**:
1. Verify webhook URL is publicly accessible (not localhost)
2. Check webhook URL returns 200 OK on POST
3. Ensure SSL certificate is valid
4. Verify webhook secret matches
5. Check Shopify webhook delivery status in admin

```bash
# Test webhook endpoint
curl -X POST https://your-domain.com/webhook \
  -H "Content-Type: application/json" \
  -H "X-Shopify-Topic: orders/create" \
  -H "X-Shopify-Hmac-Sha256: test" \
  -d '{"id":12345}'

# Should return 200 OK
```

### Performance Troubleshooting

#### Slow Query Performance

Enable query logging to identify bottlenecks:

```sql
-- Enable slow query logging (PostgreSQL)
ALTER DATABASE nself SET log_min_duration_statement = 1000; -- Log queries > 1s

-- Check slow queries
SELECT
  query,
  calls,
  total_time,
  mean_time,
  max_time
FROM pg_stat_statements
WHERE mean_time > 1000
ORDER BY total_time DESC
LIMIT 10;
```

#### Memory Issues During Sync

```
Error: JavaScript heap out of memory
```

**Solution**: Process in smaller batches:

```typescript
async function syncProductsInBatches() {
  const pageSize = 250;
  let pageInfo: string | undefined;

  do {
    const products = await client.listProducts({ pageInfo, limit: pageSize });

    // Process batch immediately, don't accumulate
    await db.upsertProductsBatch(products.data);

    pageInfo = products.nextPageInfo;

    // Allow garbage collection
    if (global.gc) {
      global.gc();
    }
  } while (pageInfo);
}

// Run with: node --expose-gc sync.js
```

### Debug Mode

Enable comprehensive debug logging:

```bash
# All Shopify plugin logs
DEBUG=shopify:* nself-shopify sync

# Specific components
DEBUG=shopify:client nself-shopify sync
DEBUG=shopify:database nself-shopify sync
DEBUG=shopify:webhooks nself-shopify server

# Include HTTP requests
DEBUG=shopify:*,http nself-shopify sync

# Log to file
DEBUG=shopify:* nself-shopify sync 2>&1 | tee shopify-sync.log
```

### Health Checks

Verify plugin health:

```bash
# Database connectivity
psql $DATABASE_URL -c "SELECT COUNT(*) FROM shopify_products;"

# API connectivity
curl -X GET "https://${SHOPIFY_SHOP_DOMAIN}/admin/api/2024-01/shop.json" \
  -H "X-Shopify-Access-Token: ${SHOPIFY_ACCESS_TOKEN}"

# Webhook endpoint
curl -X GET https://your-domain.com/health

# Plugin server
nself-shopify status
```

### Monitoring Queries

Track plugin performance:

```sql
-- Sync freshness (how old is synced data?)
SELECT
  'products' AS table_name,
  COUNT(*) AS total_records,
  MAX(synced_at) AS last_sync,
  NOW() - MAX(synced_at) AS sync_age
FROM shopify_products
UNION ALL
SELECT 'orders', COUNT(*), MAX(synced_at), NOW() - MAX(synced_at)
FROM shopify_orders
UNION ALL
SELECT 'customers', COUNT(*), MAX(synced_at), NOW() - MAX(synced_at)
FROM shopify_customers;

-- Webhook processing health
SELECT
  DATE_TRUNC('hour', received_at) AS hour,
  COUNT(*) AS total_events,
  COUNT(*) FILTER (WHERE processed = true) AS processed,
  COUNT(*) FILTER (WHERE processed = false) AS failed,
  AVG(EXTRACT(EPOCH FROM (processed_at - received_at))) AS avg_processing_seconds
FROM shopify_webhook_events
WHERE received_at > NOW() - INTERVAL '24 hours'
GROUP BY hour
ORDER BY hour DESC;
```

### Getting Help

When reporting issues, include:

1. Plugin version (`nself-shopify --version`)
2. Shopify API version (`SHOPIFY_API_VERSION`)
3. PostgreSQL version (`SELECT version();`)
4. Error message with stack trace
5. Debug logs (`DEBUG=shopify:*`)
6. Webhook event payload (if applicable)
7. Database table sizes (`\dt+ shopify_*` in psql)

**Support Resources**:
- [GitHub Issues](https://github.com/acamarata/nself-plugins/issues)
- [Shopify API Documentation](https://shopify.dev/docs/api)
- [Shopify Partners Community](https://community.shopify.com/c/shopify-apis-and-sdks/bd-p/shopify-apis-and-technology)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)

---

## Additional Resources

### Example Dashboards

Pre-built dashboard queries for common metrics:

```sql
-- Executive Dashboard
CREATE VIEW shopify_executive_dashboard AS
SELECT
  -- Today's metrics
  (SELECT COUNT(*) FROM shopify_orders
   WHERE DATE(created_at) = CURRENT_DATE
     AND financial_status = 'paid') AS orders_today,

  (SELECT COALESCE(SUM(total_price), 0) FROM shopify_orders
   WHERE DATE(created_at) = CURRENT_DATE
     AND financial_status = 'paid') AS revenue_today,

  -- This week
  (SELECT COUNT(*) FROM shopify_orders
   WHERE created_at > DATE_TRUNC('week', NOW())
     AND financial_status = 'paid') AS orders_this_week,

  (SELECT COALESCE(SUM(total_price), 0) FROM shopify_orders
   WHERE created_at > DATE_TRUNC('week', NOW())
     AND financial_status = 'paid') AS revenue_this_week,

  -- This month
  (SELECT COUNT(*) FROM shopify_orders
   WHERE created_at > DATE_TRUNC('month', NOW())
     AND financial_status = 'paid') AS orders_this_month,

  (SELECT COALESCE(SUM(total_price), 0) FROM shopify_orders
   WHERE created_at > DATE_TRUNC('month', NOW())
     AND financial_status = 'paid') AS revenue_this_month,

  -- Inventory
  (SELECT COUNT(*) FROM shopify_inventory WHERE available = 0) AS out_of_stock_count,
  (SELECT COUNT(*) FROM shopify_inventory WHERE available < 10 AND available > 0) AS low_stock_count,

  -- Customers
  (SELECT COUNT(*) FROM shopify_customers WHERE created_at > NOW() - INTERVAL '7 days') AS new_customers_week,
  (SELECT COUNT(*) FROM shopify_customers) AS total_customers;
```

### Automation Scripts

Cron jobs for automated operations:

```bash
#!/bin/bash
# /etc/cron.d/shopify-sync

# Incremental sync every hour
0 * * * * /usr/local/bin/nself-shopify sync --incremental >> /var/log/shopify-sync.log 2>&1

# Full sync daily at 3 AM
0 3 * * * /usr/local/bin/nself-shopify sync >> /var/log/shopify-full-sync.log 2>&1

# Inventory alerts every 4 hours
0 */4 * * * /usr/local/bin/shopify-inventory-alerts.sh >> /var/log/shopify-alerts.log 2>&1

# Daily sales report at 9 AM
0 9 * * * /usr/local/bin/shopify-daily-report.sh >> /var/log/shopify-reports.log 2>&1

# Abandoned cart recovery every 2 hours
0 */2 * * * /usr/local/bin/shopify-cart-recovery.sh >> /var/log/shopify-recovery.log 2>&1
```

### Integration Examples

Connect Shopify data with other services:

```typescript
// Send daily sales summary to Slack
async function sendDailySalesSummary() {
  const sql = `
    SELECT
      COUNT(*) AS orders,
      SUM(total_price) AS revenue,
      AVG(total_price) AS aov,
      COUNT(DISTINCT customer_id) AS unique_customers
    FROM shopify_orders
    WHERE DATE(created_at) = CURRENT_DATE
      AND financial_status = 'paid';
  `;

  const result = await db.query(sql);
  const stats = result.rows[0];

  const message = {
    text: 'Daily Sales Summary',
    blocks: [
      {
        type: 'section',
        text: {
          type: 'mrkdwn',
          text: `*Daily Sales Summary for ${new Date().toLocaleDateString()}*\n\n` +
                `💰 Revenue: $${stats.revenue.toFixed(2)}\n` +
                `📦 Orders: ${stats.orders}\n` +
                `👥 Customers: ${stats.unique_customers}\n` +
                `💵 AOV: $${stats.aov.toFixed(2)}`
        }
      }
    ]
  };

  await fetch(process.env.SLACK_WEBHOOK_URL, {
    method: 'POST',
    body: JSON.stringify(message),
  });
}

// Export to data warehouse (Snowflake, BigQuery, etc.)
async function exportToWarehouse() {
  const orders = await db.query(`
    SELECT * FROM shopify_orders
    WHERE synced_at > NOW() - INTERVAL '1 hour'
  `);

  // Stream to BigQuery
  const bigquery = new BigQuery();
  const dataset = bigquery.dataset('ecommerce');
  const table = dataset.table('shopify_orders');

  await table.insert(orders.rows);
}
```

### Best Practices Summary

1. **Sync Strategy**
   - Use incremental sync for regular updates
   - Schedule full sync weekly during off-peak hours
   - Rely on webhooks for real-time critical data

2. **Database Management**
   - Create indexes on frequently queried columns
   - Use partitioning for tables with millions of rows
   - Vacuum and analyze tables regularly
   - Monitor database size and growth

3. **Security**
   - Never commit access tokens to git
   - Use environment variables or secret managers
   - Rotate tokens periodically
   - Implement row-level security for multi-tenant setups
   - Always verify webhook signatures

4. **Performance**
   - Use connection pooling
   - Batch database operations
   - Monitor API rate limits
   - Cache frequently accessed data
   - Use materialized views for complex analytics

5. **Monitoring**
   - Set up alerts for sync failures
   - Track webhook processing times
   - Monitor inventory levels
   - Alert on revenue anomalies
   - Log all errors with context

6. **Data Quality**
   - Validate data before inserting
   - Handle JSONB fields safely
   - Implement idempotency for webhooks
   - Regular data integrity checks
   - Archive old data appropriately
