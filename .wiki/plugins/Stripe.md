# Stripe Plugin

Complete Stripe billing and payments integration for nself. Syncs all Stripe data to PostgreSQL with real-time webhook support.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [TypeScript Implementation](#typescript-implementation)
- [API Version & Compatibility](#api-version--compatibility)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Stripe plugin provides complete synchronization of your Stripe account data to a local PostgreSQL database. It supports:

- **21 Database Tables** - Comprehensive coverage of Stripe objects
- **70+ Webhook Events** - Real-time updates for all Stripe events
- **6 Analytics Views** - Pre-built SQL views for common metrics
- **Full REST API** - Query synced data via HTTP endpoints
- **CLI Interface** - Manage everything from the command line
- **Account Provenance** - `source_account_id` stored on synced rows for multi-account traceability

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Customers | Customer profiles with metadata | `np_stripe_customers` |
| Products | Product catalog | `np_stripe_products` |
| Prices | Pricing information | `np_stripe_prices` |
| Subscriptions | Active and past subscriptions | `np_stripe_subscriptions` |
| Subscription Items | Individual subscription line items | `np_stripe_subscription_items` |
| Invoices | All invoices | `np_stripe_invoices` |
| Invoice Items | Individual invoice line items | `np_stripe_invoice_items` |
| Payment Intents | Payment attempts | `np_stripe_payment_intents` |
| Payment Methods | Saved payment methods | `np_stripe_payment_methods` |
| Charges | Completed charges | `np_stripe_charges` |
| Refunds | Refund records | `np_stripe_refunds` |
| Disputes | Chargebacks and disputes | `np_stripe_disputes` |
| Balance Transactions | Account balance history | `np_stripe_balance_transactions` |
| Payouts | Payout records | `np_stripe_payouts` |
| Coupons | Discount coupons | `np_stripe_coupons` |
| Promotion Codes | Promo codes | `np_stripe_promotion_codes` |
| Tax Rates | Tax rate definitions | `np_stripe_tax_rates` |
| Setup Intents | Payment method setup attempts | `np_stripe_setup_intents` |
| Checkout Sessions | Checkout session records | `np_stripe_checkout_sessions` |
| Events | Stripe event log | `np_stripe_events` |
| Webhook Events | Received webhook events | `np_stripe_webhook_events` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install stripe

# Configure environment
echo "STRIPE_API_KEY=sk_live_xxx" >> .env
echo "STRIPE_WEBHOOK_SECRET=whsec_xxx" >> .env
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env

# Initialize database schema
nself plugin stripe init

# Sync all data
nself plugin stripe sync

# Start webhook server
nself plugin stripe server --port 3001
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `STRIPE_API_KEY` | Yes* | - | Stripe secret API key (sk_live_xxx or sk_test_xxx) |
| `STRIPE_API_KEYS` | No | - | Comma-separated API keys for unified multi-account sync |
| `STRIPE_ACCOUNT_LABELS` | No | - | Comma-separated labels aligned with `STRIPE_API_KEYS` |
| `STRIPE_WEBHOOK_SECRET` | No | - | Webhook endpoint signing secret (whsec_xxx) |
| `STRIPE_WEBHOOK_SECRETS` | No | - | Comma-separated webhook secrets aligned with `STRIPE_API_KEYS` |
| `STRIPE_ACCOUNT_ID` | No | `primary` | Label for single-account status output |
| `STRIPE_API_VERSION` | No | `2024-12-18` | Stripe API version to use |
| `PORT` | No | `3001` | HTTP server port |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

\* `STRIPE_API_KEY` is required when `STRIPE_API_KEYS` is not set.

### API Key Permissions

Your Stripe API key needs these permissions:
- **Read** access to all resources you want to sync
- **Write** access is not required (plugin is read-only)

For production, use a restricted key with only read permissions.

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Stripe API
STRIPE_API_KEY=sk_live_51ABC123DEF456...
STRIPE_WEBHOOK_SECRET=whsec_abc123def456...
STRIPE_API_VERSION=2024-12-18

# Server
PORT=3001
LOG_LEVEL=info
```

### Unified Multi-Account Example

```bash
STRIPE_API_KEYS=sk_live_legacy,sk_live_rebrand
STRIPE_ACCOUNT_LABELS=legacy,rebrand
STRIPE_WEBHOOK_SECRETS=whsec_legacy,whsec_rebrand
```

With these vars set, `sync` runs aggregate data from all configured accounts in one pass.
Each synced record keeps its origin in `source_account_id`, so data is unified by default but still account-aware for filtering and audits.
If a Stripe object ID collides across accounts, the most recently synced row for that ID is retained.

---

## CLI Commands

### Plugin Management

```bash
# Initialize database schema
nself plugin stripe init

# Check plugin status
nself plugin stripe status

# View sync statistics
nself plugin stripe stats
```

### Data Synchronization

```bash
# Full sync (all resources)
nself plugin stripe sync

# Sync specific resources
nself plugin stripe sync customers
nself plugin stripe sync subscriptions
nself plugin stripe sync invoices
nself plugin stripe sync products
nself plugin stripe sync prices
nself plugin stripe sync charges
nself plugin stripe sync payouts

# Incremental sync (only changed data)
nself plugin stripe sync --incremental

# Sync with date filter
nself plugin stripe sync --since 2024-01-01
```

### Customer Commands

```bash
# List all customers
nself plugin stripe customers list

# List with pagination
nself plugin stripe customers list --limit 50 --offset 100

# Get customer by ID
nself plugin stripe customers get cus_ABC123

# Search customers by email
nself plugin stripe customers search john@example.com

# List customers by creation date
nself plugin stripe customers list --since 2024-01-01

# Export customers to CSV
nself plugin stripe customers list --format csv > customers.csv
```

### Subscription Commands

```bash
# List active subscriptions
nself plugin stripe subscriptions list

# List all subscriptions (including canceled)
nself plugin stripe subscriptions list --all

# Get subscription details
nself plugin stripe subscriptions get sub_ABC123

# List subscriptions by status
nself plugin stripe subscriptions list --status active
nself plugin stripe subscriptions list --status canceled
nself plugin stripe subscriptions list --status past_due

# View subscription statistics
nself plugin stripe subscriptions stats
```

### Invoice Commands

```bash
# List all invoices
nself plugin stripe invoices list

# List by status
nself plugin stripe invoices list --status paid
nself plugin stripe invoices list --status open
nself plugin stripe invoices list --status uncollectible

# View failed payments
nself plugin stripe invoices failed

# Get invoice details
nself plugin stripe invoices get in_ABC123
```

### Payment Commands

```bash
# List payment intents
nself plugin stripe payments list

# List successful payments
nself plugin stripe payments list --status succeeded

# List failed payments
nself plugin stripe payments list --status failed

# Get payment details
nself plugin stripe payments get pi_ABC123

# List charges
nself plugin stripe charges list

# List refunds
nself plugin stripe refunds list
```

### Webhook Commands

```bash
# Check webhook configuration
nself plugin stripe webhook status

# List recent webhook events
nself plugin stripe webhook events

# List events by type
nself plugin stripe webhook events --type customer.created

# View failed events
nself plugin stripe webhook events --failed

# Retry a failed event
nself plugin stripe webhook retry evt_ABC123
```

### Server Commands

```bash
# Start HTTP server
nself plugin stripe server

# Start on custom port
nself plugin stripe server --port 3001

# Start with specific host
nself plugin stripe server --host 0.0.0.0 --port 3001
```

---

## REST API

The plugin exposes a REST API when running the server.

### Base URL

```
http://localhost:3001
```

### Endpoints

#### Health & Status

```http
GET /health
```
Returns server health status.

```http
GET /status
```
Returns sync status and statistics.

#### Sync

```http
POST /sync
```
Triggers a full data sync.

```http
POST /sync
Content-Type: application/json

{
  "resources": ["customers", "subscriptions"],
  "incremental": true,
  "accounts": ["legacy", "rebrand"]
}
```
Triggers sync for specific resources and optional account subsets.

#### Customers

```http
GET /api/customers
```
List all customers. Query params: `limit`, `offset`, `email`, `since`.

```http
GET /api/customers/:id
```
Get customer by ID.

```http
GET /api/customers/:id/subscriptions
```
Get customer's subscriptions.

```http
GET /api/customers/:id/invoices
```
Get customer's invoices.

```http
GET /api/customers/:id/payments
```
Get customer's payment history.

#### Subscriptions

```http
GET /api/subscriptions
```
List all subscriptions. Query params: `limit`, `offset`, `status`, `customer`.

```http
GET /api/subscriptions/:id
```
Get subscription by ID.

```http
GET /api/subscriptions/stats
```
Get subscription statistics (MRR, churn, etc.).

#### Invoices

```http
GET /api/invoices
```
List all invoices. Query params: `limit`, `offset`, `status`, `customer`.

```http
GET /api/invoices/:id
```
Get invoice by ID.

```http
GET /api/invoices/failed
```
List failed invoices.

#### Products & Prices

```http
GET /api/products
```
List all products.

```http
GET /api/products/:id
```
Get product by ID.

```http
GET /api/products/:id/prices
```
Get product's prices.

```http
GET /api/prices
```
List all prices.

#### Payments

```http
GET /api/payments
```
List payment intents.

```http
GET /api/payments/:id
```
Get payment intent by ID.

```http
GET /api/charges
```
List charges.

```http
GET /api/refunds
```
List refunds.

#### Analytics

```http
GET /api/analytics/mrr
```
Get monthly recurring revenue.

```http
GET /api/analytics/revenue
```
Get revenue by period.

```http
GET /api/analytics/churn
```
Get churn metrics.

#### Webhooks

```http
POST /webhook
```
Stripe webhook endpoint. Requires valid signature.

```http
GET /api/webhook/events
```
List received webhook events.

---

## Webhook Events

The plugin handles all Stripe webhook events. Here's the complete list:

### Customer Events

| Event | Description | Action |
|-------|-------------|--------|
| `customer.created` | New customer created | Insert customer record |
| `customer.updated` | Customer data changed | Update customer record |
| `customer.deleted` | Customer deleted | Mark as deleted |
| `customer.subscription.created` | Customer got new subscription | Insert subscription |
| `customer.subscription.updated` | Subscription changed | Update subscription |
| `customer.subscription.deleted` | Subscription canceled | Mark as deleted |
| `customer.subscription.paused` | Subscription paused | Update status |
| `customer.subscription.resumed` | Subscription resumed | Update status |
| `customer.subscription.pending_update_applied` | Pending update applied | Update subscription |
| `customer.subscription.pending_update_expired` | Pending update expired | Update subscription |
| `customer.subscription.trial_will_end` | Trial ending soon | Log event |
| `customer.source.created` | Payment source added | Insert payment method |
| `customer.source.deleted` | Payment source removed | Delete payment method |
| `customer.source.expiring` | Payment source expiring | Log event |
| `customer.source.updated` | Payment source updated | Update payment method |
| `customer.discount.created` | Discount applied | Update customer |
| `customer.discount.deleted` | Discount removed | Update customer |
| `customer.discount.updated` | Discount changed | Update customer |
| `customer.tax_id.created` | Tax ID added | Update customer |
| `customer.tax_id.deleted` | Tax ID removed | Update customer |
| `customer.tax_id.updated` | Tax ID updated | Update customer |

### Subscription Events

| Event | Description | Action |
|-------|-------------|--------|
| `subscription_schedule.aborted` | Schedule aborted | Update schedule |
| `subscription_schedule.canceled` | Schedule canceled | Update schedule |
| `subscription_schedule.completed` | Schedule completed | Update schedule |
| `subscription_schedule.created` | Schedule created | Insert schedule |
| `subscription_schedule.expiring` | Schedule expiring | Log event |
| `subscription_schedule.released` | Schedule released | Update schedule |
| `subscription_schedule.updated` | Schedule updated | Update schedule |

### Invoice Events

| Event | Description | Action |
|-------|-------------|--------|
| `invoice.created` | Invoice created | Insert invoice |
| `invoice.deleted` | Invoice deleted | Mark as deleted |
| `invoice.finalization_failed` | Finalization failed | Update status |
| `invoice.finalized` | Invoice finalized | Update status |
| `invoice.marked_uncollectible` | Marked uncollectible | Update status |
| `invoice.paid` | Invoice paid | Update status |
| `invoice.payment_action_required` | Payment action required | Update status |
| `invoice.payment_failed` | Payment failed | Update status |
| `invoice.payment_succeeded` | Payment succeeded | Update status |
| `invoice.sent` | Invoice sent | Update status |
| `invoice.upcoming` | Invoice upcoming | Log event |
| `invoice.updated` | Invoice updated | Update invoice |
| `invoice.voided` | Invoice voided | Update status |
| `invoiceitem.created` | Invoice item created | Insert invoice item |
| `invoiceitem.deleted` | Invoice item deleted | Delete invoice item |

### Payment Events

| Event | Description | Action |
|-------|-------------|--------|
| `payment_intent.amount_capturable_updated` | Amount capturable updated | Update payment intent |
| `payment_intent.canceled` | Payment canceled | Update status |
| `payment_intent.created` | Payment created | Insert payment intent |
| `payment_intent.partially_funded` | Partially funded | Update status |
| `payment_intent.payment_failed` | Payment failed | Update status |
| `payment_intent.processing` | Payment processing | Update status |
| `payment_intent.requires_action` | Requires action | Update status |
| `payment_intent.succeeded` | Payment succeeded | Update status |
| `payment_method.attached` | Payment method attached | Insert payment method |
| `payment_method.automatically_updated` | Auto updated | Update payment method |
| `payment_method.detached` | Payment method detached | Delete payment method |
| `payment_method.updated` | Payment method updated | Update payment method |

### Charge Events

| Event | Description | Action |
|-------|-------------|--------|
| `charge.captured` | Charge captured | Update charge |
| `charge.expired` | Charge expired | Update status |
| `charge.failed` | Charge failed | Update status |
| `charge.pending` | Charge pending | Update status |
| `charge.refunded` | Charge refunded | Update charge |
| `charge.refund.updated` | Refund updated | Update refund |
| `charge.succeeded` | Charge succeeded | Insert/update charge |
| `charge.updated` | Charge updated | Update charge |
| `charge.dispute.closed` | Dispute closed | Update dispute |
| `charge.dispute.created` | Dispute created | Insert dispute |
| `charge.dispute.funds_reinstated` | Funds reinstated | Update dispute |
| `charge.dispute.funds_withdrawn` | Funds withdrawn | Update dispute |
| `charge.dispute.updated` | Dispute updated | Update dispute |

### Payout Events

| Event | Description | Action |
|-------|-------------|--------|
| `payout.canceled` | Payout canceled | Update status |
| `payout.created` | Payout created | Insert payout |
| `payout.failed` | Payout failed | Update status |
| `payout.paid` | Payout completed | Update status |
| `payout.reconciliation_completed` | Reconciliation done | Update payout |
| `payout.updated` | Payout updated | Update payout |

### Product & Price Events

| Event | Description | Action |
|-------|-------------|--------|
| `product.created` | Product created | Insert product |
| `product.deleted` | Product deleted | Mark as deleted |
| `product.updated` | Product updated | Update product |
| `price.created` | Price created | Insert price |
| `price.deleted` | Price deleted | Mark as deleted |
| `price.updated` | Price updated | Update price |

### Coupon & Promotion Events

| Event | Description | Action |
|-------|-------------|--------|
| `coupon.created` | Coupon created | Insert coupon |
| `coupon.deleted` | Coupon deleted | Mark as deleted |
| `coupon.updated` | Coupon updated | Update coupon |
| `promotion_code.created` | Promo code created | Insert promo code |
| `promotion_code.updated` | Promo code updated | Update promo code |

### Other Events

| Event | Description | Action |
|-------|-------------|--------|
| `balance.available` | Balance available | Log event |
| `setup_intent.canceled` | Setup canceled | Update status |
| `setup_intent.created` | Setup created | Insert setup intent |
| `setup_intent.requires_action` | Requires action | Update status |
| `setup_intent.setup_failed` | Setup failed | Update status |
| `setup_intent.succeeded` | Setup succeeded | Update status |
| `checkout.session.async_payment_failed` | Async payment failed | Update session |
| `checkout.session.async_payment_succeeded` | Async payment succeeded | Update session |
| `checkout.session.completed` | Checkout completed | Update session |
| `checkout.session.expired` | Checkout expired | Update session |
| `tax_rate.created` | Tax rate created | Insert tax rate |
| `tax_rate.updated` | Tax rate updated | Update tax rate |

---

## Database Schema

### np_stripe_customers

```sql
CREATE TABLE np_stripe_customers (
    id VARCHAR(255) PRIMARY KEY,              -- cus_xxx
    email VARCHAR(255),
    name VARCHAR(255),
    phone VARCHAR(50),
    description TEXT,
    address JSONB,                            -- {line1, line2, city, state, postal_code, country}
    shipping JSONB,                           -- {name, address, phone}
    metadata JSONB DEFAULT '{}',
    currency VARCHAR(3),                      -- usd, eur, etc.
    balance INTEGER DEFAULT 0,                -- Account balance in cents
    delinquent BOOLEAN DEFAULT FALSE,
    default_source VARCHAR(255),              -- Payment source ID
    invoice_prefix VARCHAR(50),
    invoice_settings JSONB,
    tax_exempt VARCHAR(20),                   -- none, exempt, reverse
    tax_ids JSONB DEFAULT '[]',
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_customers_email ON np_stripe_customers(email);
CREATE INDEX idx_stripe_customers_created ON np_stripe_customers(created_at DESC);
CREATE INDEX idx_stripe_customers_synced ON np_stripe_customers(synced_at DESC);
```

### np_stripe_products

```sql
CREATE TABLE np_stripe_products (
    id VARCHAR(255) PRIMARY KEY,              -- prod_xxx
    name VARCHAR(255) NOT NULL,
    description TEXT,
    active BOOLEAN DEFAULT TRUE,
    type VARCHAR(20),                         -- good, service
    attributes JSONB DEFAULT '[]',
    caption VARCHAR(255),
    images JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    package_dimensions JSONB,
    shippable BOOLEAN,
    statement_descriptor VARCHAR(22),
    tax_code VARCHAR(50),
    unit_label VARCHAR(50),
    url VARCHAR(2048),
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_products_active ON np_stripe_products(active);
CREATE INDEX idx_stripe_products_type ON np_stripe_products(type);
```

### np_stripe_prices

```sql
CREATE TABLE np_stripe_prices (
    id VARCHAR(255) PRIMARY KEY,              -- price_xxx
    product_id VARCHAR(255) REFERENCES np_stripe_products(id),
    active BOOLEAN DEFAULT TRUE,
    currency VARCHAR(3) NOT NULL,
    unit_amount INTEGER,                      -- Amount in cents (null for metered)
    unit_amount_decimal VARCHAR(50),          -- For high precision
    billing_scheme VARCHAR(20),               -- per_unit, tiered
    type VARCHAR(20),                         -- one_time, recurring
    recurring_interval VARCHAR(10),           -- day, week, month, year
    recurring_interval_count INTEGER,
    recurring_usage_type VARCHAR(20),         -- licensed, metered
    recurring_aggregate_usage VARCHAR(20),    -- sum, last_during_period, last_ever, max
    tiers JSONB,                              -- Tiered pricing tiers
    tiers_mode VARCHAR(20),                   -- graduated, volume
    transform_quantity JSONB,
    lookup_key VARCHAR(255),
    nickname VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    tax_behavior VARCHAR(20),                 -- inclusive, exclusive, unspecified
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_prices_product ON np_stripe_prices(product_id);
CREATE INDEX idx_stripe_prices_active ON np_stripe_prices(active);
CREATE INDEX idx_stripe_prices_type ON np_stripe_prices(type);
```

### np_stripe_subscriptions

```sql
CREATE TABLE np_stripe_subscriptions (
    id VARCHAR(255) PRIMARY KEY,              -- sub_xxx
    customer_id VARCHAR(255) REFERENCES np_stripe_customers(id),
    status VARCHAR(20) NOT NULL,              -- active, past_due, unpaid, canceled, incomplete, incomplete_expired, trialing, paused
    current_period_start TIMESTAMP WITH TIME ZONE,
    current_period_end TIMESTAMP WITH TIME ZONE,
    cancel_at TIMESTAMP WITH TIME ZONE,
    canceled_at TIMESTAMP WITH TIME ZONE,
    cancel_at_period_end BOOLEAN DEFAULT FALSE,
    collection_method VARCHAR(20),            -- charge_automatically, send_invoice
    billing_cycle_anchor TIMESTAMP WITH TIME ZONE,
    billing_cycle_anchor_config JSONB,
    days_until_due INTEGER,
    default_payment_method VARCHAR(255),
    default_source VARCHAR(255),
    discount JSONB,
    ended_at TIMESTAMP WITH TIME ZONE,
    items JSONB DEFAULT '[]',                 -- Array of subscription items
    latest_invoice VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    pause_collection JSONB,
    payment_settings JSONB,
    pending_invoice_item_interval JSONB,
    pending_setup_intent VARCHAR(255),
    pending_update JSONB,
    schedule VARCHAR(255),
    start_date TIMESTAMP WITH TIME ZONE,
    trial_start TIMESTAMP WITH TIME ZONE,
    trial_end TIMESTAMP WITH TIME ZONE,
    trial_settings JSONB,
    application VARCHAR(255),
    application_fee_percent DECIMAL(5,2),
    automatic_tax JSONB,
    on_behalf_of VARCHAR(255),
    transfer_data JSONB,
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_subscriptions_customer ON np_stripe_subscriptions(customer_id);
CREATE INDEX idx_stripe_subscriptions_status ON np_stripe_subscriptions(status);
CREATE INDEX idx_stripe_subscriptions_created ON np_stripe_subscriptions(created_at DESC);
CREATE INDEX idx_stripe_subscriptions_period ON np_stripe_subscriptions(current_period_end);
```

### np_stripe_invoices

```sql
CREATE TABLE np_stripe_invoices (
    id VARCHAR(255) PRIMARY KEY,              -- in_xxx
    customer_id VARCHAR(255) REFERENCES np_stripe_customers(id),
    subscription_id VARCHAR(255),
    status VARCHAR(20),                       -- draft, open, paid, uncollectible, void
    collection_method VARCHAR(20),
    currency VARCHAR(3),
    amount_due INTEGER,
    amount_paid INTEGER,
    amount_remaining INTEGER,
    subtotal INTEGER,
    subtotal_excluding_tax INTEGER,
    total INTEGER,
    total_excluding_tax INTEGER,
    tax INTEGER,
    total_discount_amounts JSONB,
    total_tax_amounts JSONB,
    attempt_count INTEGER DEFAULT 0,
    attempted BOOLEAN DEFAULT FALSE,
    auto_advance BOOLEAN DEFAULT TRUE,
    billing_reason VARCHAR(50),               -- subscription_create, subscription_cycle, subscription_update, subscription_threshold, manual, upcoming
    charge VARCHAR(255),
    customer_email VARCHAR(255),
    customer_name VARCHAR(255),
    customer_phone VARCHAR(50),
    customer_address JSONB,
    customer_shipping JSONB,
    customer_tax_exempt VARCHAR(20),
    customer_tax_ids JSONB,
    default_payment_method VARCHAR(255),
    default_source VARCHAR(255),
    description TEXT,
    discount JSONB,
    discounts JSONB DEFAULT '[]',
    due_date TIMESTAMP WITH TIME ZONE,
    effective_at TIMESTAMP WITH TIME ZONE,
    ending_balance INTEGER,
    footer TEXT,
    from_invoice JSONB,
    hosted_invoice_url VARCHAR(2048),
    invoice_pdf VARCHAR(2048),
    issuer JSONB,
    last_finalization_error JSONB,
    latest_revision VARCHAR(255),
    lines JSONB DEFAULT '[]',                 -- Invoice line items
    metadata JSONB DEFAULT '{}',
    next_payment_attempt TIMESTAMP WITH TIME ZONE,
    number VARCHAR(255),
    on_behalf_of VARCHAR(255),
    paid BOOLEAN DEFAULT FALSE,
    paid_out_of_band BOOLEAN DEFAULT FALSE,
    payment_intent VARCHAR(255),
    payment_settings JSONB,
    period_start TIMESTAMP WITH TIME ZONE,
    period_end TIMESTAMP WITH TIME ZONE,
    post_payment_credit_notes_amount INTEGER DEFAULT 0,
    pre_payment_credit_notes_amount INTEGER DEFAULT 0,
    quote VARCHAR(255),
    receipt_number VARCHAR(255),
    rendering JSONB,
    rendering_options JSONB,
    shipping_cost JSONB,
    shipping_details JSONB,
    starting_balance INTEGER DEFAULT 0,
    statement_descriptor VARCHAR(22),
    subscription_details JSONB,
    subscription_proration_date TIMESTAMP WITH TIME ZONE,
    test_clock VARCHAR(255),
    threshold_reason JSONB,
    transfer_data JSONB,
    webhooks_delivered_at TIMESTAMP WITH TIME ZONE,
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    finalized_at TIMESTAMP WITH TIME ZONE,
    voided_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_invoices_customer ON np_stripe_invoices(customer_id);
CREATE INDEX idx_stripe_invoices_subscription ON np_stripe_invoices(subscription_id);
CREATE INDEX idx_stripe_invoices_status ON np_stripe_invoices(status);
CREATE INDEX idx_stripe_invoices_created ON np_stripe_invoices(created_at DESC);
CREATE INDEX idx_stripe_invoices_due ON np_stripe_invoices(due_date);
```

### np_stripe_payment_intents

```sql
CREATE TABLE np_stripe_payment_intents (
    id VARCHAR(255) PRIMARY KEY,              -- pi_xxx
    customer_id VARCHAR(255),
    amount INTEGER NOT NULL,
    amount_capturable INTEGER DEFAULT 0,
    amount_received INTEGER DEFAULT 0,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(30) NOT NULL,              -- requires_payment_method, requires_confirmation, requires_action, processing, requires_capture, canceled, succeeded
    capture_method VARCHAR(20),               -- automatic, automatic_async, manual
    confirmation_method VARCHAR(20),          -- automatic, manual
    cancellation_reason VARCHAR(50),
    canceled_at TIMESTAMP WITH TIME ZONE,
    client_secret VARCHAR(255),
    description TEXT,
    invoice VARCHAR(255),
    last_payment_error JSONB,
    latest_charge VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    next_action JSONB,
    on_behalf_of VARCHAR(255),
    payment_method VARCHAR(255),
    payment_method_configuration_details JSONB,
    payment_method_options JSONB,
    payment_method_types JSONB DEFAULT '[]',
    processing JSONB,
    receipt_email VARCHAR(255),
    review VARCHAR(255),
    setup_future_usage VARCHAR(20),           -- off_session, on_session
    shipping JSONB,
    source VARCHAR(255),
    statement_descriptor VARCHAR(22),
    statement_descriptor_suffix VARCHAR(22),
    transfer_data JSONB,
    transfer_group VARCHAR(255),
    application VARCHAR(255),
    application_fee_amount INTEGER,
    automatic_payment_methods JSONB,
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_payment_intents_customer ON np_stripe_payment_intents(customer_id);
CREATE INDEX idx_stripe_payment_intents_status ON np_stripe_payment_intents(status);
CREATE INDEX idx_stripe_payment_intents_created ON np_stripe_payment_intents(created_at DESC);
```

### np_stripe_charges

```sql
CREATE TABLE np_stripe_charges (
    id VARCHAR(255) PRIMARY KEY,              -- ch_xxx
    customer_id VARCHAR(255),
    payment_intent VARCHAR(255),
    invoice VARCHAR(255),
    amount INTEGER NOT NULL,
    amount_captured INTEGER DEFAULT 0,
    amount_refunded INTEGER DEFAULT 0,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,              -- succeeded, pending, failed
    paid BOOLEAN DEFAULT FALSE,
    refunded BOOLEAN DEFAULT FALSE,
    captured BOOLEAN DEFAULT FALSE,
    disputed BOOLEAN DEFAULT FALSE,
    balance_transaction VARCHAR(255),
    billing_details JSONB,
    calculated_statement_descriptor VARCHAR(22),
    description TEXT,
    destination VARCHAR(255),
    failure_balance_transaction VARCHAR(255),
    failure_code VARCHAR(50),
    failure_message TEXT,
    fraud_details JSONB,
    metadata JSONB DEFAULT '{}',
    on_behalf_of VARCHAR(255),
    outcome JSONB,
    payment_method VARCHAR(255),
    payment_method_details JSONB,
    radar_options JSONB,
    receipt_email VARCHAR(255),
    receipt_number VARCHAR(255),
    receipt_url VARCHAR(2048),
    refunds JSONB,
    review VARCHAR(255),
    shipping JSONB,
    source JSONB,
    source_transfer VARCHAR(255),
    statement_descriptor VARCHAR(22),
    statement_descriptor_suffix VARCHAR(22),
    transfer VARCHAR(255),
    transfer_data JSONB,
    transfer_group VARCHAR(255),
    application VARCHAR(255),
    application_fee VARCHAR(255),
    application_fee_amount INTEGER,
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_charges_customer ON np_stripe_charges(customer_id);
CREATE INDEX idx_stripe_charges_payment_intent ON np_stripe_charges(payment_intent);
CREATE INDEX idx_stripe_charges_status ON np_stripe_charges(status);
CREATE INDEX idx_stripe_charges_created ON np_stripe_charges(created_at DESC);
```

### np_stripe_refunds

```sql
CREATE TABLE np_stripe_refunds (
    id VARCHAR(255) PRIMARY KEY,              -- re_xxx
    charge_id VARCHAR(255) REFERENCES np_stripe_charges(id),
    payment_intent VARCHAR(255),
    amount INTEGER NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,              -- succeeded, pending, failed, canceled
    reason VARCHAR(50),                       -- duplicate, fraudulent, requested_by_customer
    receipt_number VARCHAR(255),
    balance_transaction VARCHAR(255),
    destination_details JSONB,
    failure_balance_transaction VARCHAR(255),
    failure_reason VARCHAR(50),
    instructions_email VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    next_action JSONB,
    source_transfer_reversal VARCHAR(255),
    transfer_reversal VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_refunds_charge ON np_stripe_refunds(charge_id);
CREATE INDEX idx_stripe_refunds_status ON np_stripe_refunds(status);
CREATE INDEX idx_stripe_refunds_created ON np_stripe_refunds(created_at DESC);
```

### np_stripe_disputes

```sql
CREATE TABLE np_stripe_disputes (
    id VARCHAR(255) PRIMARY KEY,              -- dp_xxx
    charge_id VARCHAR(255) REFERENCES np_stripe_charges(id),
    payment_intent VARCHAR(255),
    amount INTEGER NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(30) NOT NULL,              -- warning_needs_response, warning_under_review, warning_closed, needs_response, under_review, won, lost
    reason VARCHAR(50) NOT NULL,              -- bank_cannot_process, credit_not_processed, customer_initiated, debit_not_authorized, duplicate, fraudulent, general, incorrect_account_details, insufficient_funds, product_not_received, product_unacceptable, subscription_canceled, unrecognized
    balance_transactions JSONB DEFAULT '[]',
    evidence JSONB,
    evidence_details JSONB,
    is_charge_refundable BOOLEAN DEFAULT FALSE,
    metadata JSONB DEFAULT '{}',
    network_reason_code VARCHAR(50),
    payment_method_details JSONB,
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_disputes_charge ON np_stripe_disputes(charge_id);
CREATE INDEX idx_stripe_disputes_status ON np_stripe_disputes(status);
CREATE INDEX idx_stripe_disputes_created ON np_stripe_disputes(created_at DESC);
```

### np_stripe_balance_transactions

```sql
CREATE TABLE np_stripe_balance_transactions (
    id VARCHAR(255) PRIMARY KEY,              -- txn_xxx
    amount INTEGER NOT NULL,
    available_on TIMESTAMP WITH TIME ZONE,
    currency VARCHAR(3) NOT NULL,
    description TEXT,
    exchange_rate DECIMAL(10,6),
    fee INTEGER DEFAULT 0,
    fee_details JSONB DEFAULT '[]',
    net INTEGER NOT NULL,
    reporting_category VARCHAR(50),           -- charge, refund, dispute, etc.
    source VARCHAR(255),                      -- ID of source object
    status VARCHAR(20) NOT NULL,              -- available, pending
    type VARCHAR(50) NOT NULL,                -- charge, refund, adjustment, application_fee, etc.
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_balance_transactions_source ON np_stripe_balance_transactions(source);
CREATE INDEX idx_stripe_balance_transactions_type ON np_stripe_balance_transactions(type);
CREATE INDEX idx_stripe_balance_transactions_created ON np_stripe_balance_transactions(created_at DESC);
CREATE INDEX idx_stripe_balance_transactions_available ON np_stripe_balance_transactions(available_on);
```

### np_stripe_payouts

```sql
CREATE TABLE np_stripe_payouts (
    id VARCHAR(255) PRIMARY KEY,              -- po_xxx
    amount INTEGER NOT NULL,
    currency VARCHAR(3) NOT NULL,
    status VARCHAR(20) NOT NULL,              -- paid, pending, in_transit, canceled, failed
    type VARCHAR(20) NOT NULL,                -- bank_account, card
    method VARCHAR(20),                       -- standard, instant
    arrival_date TIMESTAMP WITH TIME ZONE,
    automatic BOOLEAN DEFAULT TRUE,
    balance_transaction VARCHAR(255),
    description TEXT,
    destination VARCHAR(255),
    failure_balance_transaction VARCHAR(255),
    failure_code VARCHAR(50),
    failure_message TEXT,
    metadata JSONB DEFAULT '{}',
    original_payout VARCHAR(255),
    reconciliation_status VARCHAR(20),        -- completed, in_progress, not_applicable
    reversed_by VARCHAR(255),
    source_type VARCHAR(20),                  -- bank_account, card, fpx
    statement_descriptor VARCHAR(22),
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_payouts_status ON np_stripe_payouts(status);
CREATE INDEX idx_stripe_payouts_created ON np_stripe_payouts(created_at DESC);
CREATE INDEX idx_stripe_payouts_arrival ON np_stripe_payouts(arrival_date);
```

### np_stripe_coupons

```sql
CREATE TABLE np_stripe_coupons (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255),
    amount_off INTEGER,
    currency VARCHAR(3),
    percent_off DECIMAL(5,2),
    duration VARCHAR(20) NOT NULL,            -- forever, once, repeating
    duration_in_months INTEGER,
    max_redemptions INTEGER,
    times_redeemed INTEGER DEFAULT 0,
    redeem_by TIMESTAMP WITH TIME ZONE,
    applies_to JSONB,
    currency_options JSONB,
    metadata JSONB DEFAULT '{}',
    valid BOOLEAN DEFAULT TRUE,
    livemode BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_coupons_valid ON np_stripe_coupons(valid);
```

### np_stripe_webhook_events

```sql
CREATE TABLE np_stripe_webhook_events (
    id VARCHAR(255) PRIMARY KEY,              -- evt_xxx or generated
    type VARCHAR(100) NOT NULL,               -- Event type
    data JSONB NOT NULL,                      -- Full event payload
    api_version VARCHAR(20),
    signature VARCHAR(255),
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_stripe_webhook_events_type ON np_stripe_webhook_events(type);
CREATE INDEX idx_stripe_webhook_events_processed ON np_stripe_webhook_events(processed);
CREATE INDEX idx_stripe_webhook_events_received ON np_stripe_webhook_events(received_at DESC);
```

---

## Analytics Views

### np_stripe_active_subscriptions

Active subscriptions with customer details.

```sql
CREATE VIEW np_stripe_active_subscriptions AS
SELECT
    s.id AS subscription_id,
    s.status,
    s.current_period_start,
    s.current_period_end,
    s.cancel_at_period_end,
    c.id AS customer_id,
    c.email AS customer_email,
    c.name AS customer_name,
    s.items,
    s.metadata,
    s.created_at
FROM np_stripe_subscriptions s
JOIN np_stripe_customers c ON s.customer_id = c.id
WHERE s.status IN ('active', 'trialing')
  AND c.deleted_at IS NULL
ORDER BY s.created_at DESC;
```

### np_stripe_mrr

Monthly recurring revenue calculation.

```sql
CREATE VIEW np_stripe_mrr AS
WITH active_subs AS (
    SELECT
        s.id,
        s.customer_id,
        s.items,
        s.current_period_start,
        s.current_period_end
    FROM np_stripe_subscriptions s
    WHERE s.status = 'active'
),
sub_amounts AS (
    SELECT
        a.id,
        a.customer_id,
        jsonb_array_elements(a.items) AS item
    FROM active_subs a
),
monthly_amounts AS (
    SELECT
        id,
        customer_id,
        CASE
            WHEN item->'price'->>'recurring_interval' = 'month' THEN
                (item->'price'->>'unit_amount')::INTEGER * (item->>'quantity')::INTEGER
            WHEN item->'price'->>'recurring_interval' = 'year' THEN
                ((item->'price'->>'unit_amount')::INTEGER * (item->>'quantity')::INTEGER) / 12
            WHEN item->'price'->>'recurring_interval' = 'week' THEN
                ((item->'price'->>'unit_amount')::INTEGER * (item->>'quantity')::INTEGER) * 4
            ELSE 0
        END AS monthly_amount_cents
    FROM sub_amounts
)
SELECT
    DATE_TRUNC('month', NOW()) AS month,
    COUNT(DISTINCT id) AS active_subscriptions,
    COUNT(DISTINCT customer_id) AS unique_customers,
    SUM(monthly_amount_cents) AS mrr_cents,
    SUM(monthly_amount_cents) / 100.0 AS mrr_dollars
FROM monthly_amounts;
```

### np_stripe_failed_payments

Recent failed payment attempts.

```sql
CREATE VIEW np_stripe_failed_payments AS
SELECT
    pi.id AS payment_intent_id,
    pi.amount / 100.0 AS amount,
    pi.currency,
    pi.status,
    pi.last_payment_error->>'message' AS error_message,
    pi.last_payment_error->>'code' AS error_code,
    c.id AS customer_id,
    c.email AS customer_email,
    c.name AS customer_name,
    pi.created_at
FROM np_stripe_payment_intents pi
LEFT JOIN np_stripe_customers c ON pi.customer_id = c.id
WHERE pi.status IN ('requires_payment_method', 'canceled')
  AND pi.last_payment_error IS NOT NULL
ORDER BY pi.created_at DESC
LIMIT 100;
```

### np_stripe_revenue_by_month

Revenue aggregated by month.

```sql
CREATE VIEW np_stripe_revenue_by_month AS
SELECT
    DATE_TRUNC('month', created_at) AS month,
    SUM(amount) AS gross_amount_cents,
    SUM(amount) / 100.0 AS gross_amount,
    COUNT(*) AS charge_count,
    SUM(CASE WHEN refunded THEN amount_refunded ELSE 0 END) AS refunded_cents,
    SUM(amount - COALESCE(amount_refunded, 0)) AS net_amount_cents,
    (SUM(amount) - SUM(COALESCE(amount_refunded, 0))) / 100.0 AS net_amount
FROM np_stripe_charges
WHERE status = 'succeeded'
GROUP BY DATE_TRUNC('month', created_at)
ORDER BY month DESC;
```

### np_stripe_customer_lifetime_value

Customer lifetime value calculation.

```sql
CREATE VIEW np_stripe_customer_lifetime_value AS
SELECT
    c.id AS customer_id,
    c.email,
    c.name,
    c.created_at AS customer_since,
    COUNT(DISTINCT ch.id) AS total_charges,
    SUM(ch.amount) / 100.0 AS total_revenue,
    AVG(ch.amount) / 100.0 AS average_charge,
    MAX(ch.created_at) AS last_charge_date,
    EXTRACT(DAYS FROM NOW() - c.created_at) AS customer_age_days
FROM np_stripe_customers c
LEFT JOIN np_stripe_charges ch ON c.id = ch.customer_id AND ch.status = 'succeeded'
WHERE c.deleted_at IS NULL
GROUP BY c.id, c.email, c.name, c.created_at
HAVING COUNT(ch.id) > 0
ORDER BY SUM(ch.amount) DESC;
```

### np_stripe_subscription_churn

Subscription churn metrics.

```sql
CREATE VIEW np_stripe_subscription_churn AS
WITH monthly_data AS (
    SELECT
        DATE_TRUNC('month', created_at) AS month,
        COUNT(*) AS new_subscriptions
    FROM np_stripe_subscriptions
    GROUP BY DATE_TRUNC('month', created_at)
),
canceled_data AS (
    SELECT
        DATE_TRUNC('month', canceled_at) AS month,
        COUNT(*) AS canceled_subscriptions
    FROM np_stripe_subscriptions
    WHERE canceled_at IS NOT NULL
    GROUP BY DATE_TRUNC('month', canceled_at)
)
SELECT
    COALESCE(m.month, c.month) AS month,
    COALESCE(m.new_subscriptions, 0) AS new_subscriptions,
    COALESCE(c.canceled_subscriptions, 0) AS canceled_subscriptions,
    COALESCE(m.new_subscriptions, 0) - COALESCE(c.canceled_subscriptions, 0) AS net_change
FROM monthly_data m
FULL OUTER JOIN canceled_data c ON m.month = c.month
ORDER BY month DESC;
```

---

## TypeScript Implementation

### Project Structure

```
plugins/stripe/ts/
├── src/
│   ├── types.ts        # Type definitions
│   ├── config.ts       # Environment configuration
│   ├── client.ts       # Stripe API client
│   ├── database.ts     # PostgreSQL operations
│   ├── sync.ts         # Data synchronization
│   ├── webhooks.ts     # Webhook handlers
│   ├── server.ts       # HTTP server
│   ├── cli.ts          # CLI commands
│   └── index.ts        # Module exports
├── package.json
├── tsconfig.json
└── README.md
```

### Dependencies

```json
{
  "dependencies": {
    "@nself/plugin-utils": "workspace:*",
    "commander": "^12.0.0",
    "fastify": "^4.26.0",
    "pg": "^8.11.0",
    "stripe": "^14.0.0"
  },
  "devDependencies": {
    "@types/node": "^20.0.0",
    "@types/pg": "^8.11.0",
    "typescript": "^5.4.0"
  }
}
```

### Key Implementation Patterns

**Rate Limiting:**
```typescript
import { RateLimiter } from '@nself/plugin-utils';

const rateLimiter = new RateLimiter(25); // 25 requests per second

async function fetchCustomers(): Promise<Customer[]> {
    await rateLimiter.acquire();
    return stripe.customers.list({ limit: 100 });
}
```

**Pagination:**
```typescript
async function* listAllCustomers(): AsyncGenerator<Stripe.Customer[]> {
    let hasMore = true;
    let startingAfter: string | undefined;

    while (hasMore) {
        await rateLimiter.acquire();
        const response = await stripe.customers.list({
            limit: 100,
            starting_after: startingAfter,
        });

        if (response.data.length > 0) {
            yield response.data;
            startingAfter = response.data[response.data.length - 1].id;
        }

        hasMore = response.has_more;
    }
}
```

**Upsert Pattern:**
```typescript
async function upsertCustomer(customer: CustomerRecord): Promise<void> {
    await db.query(`
        INSERT INTO np_stripe_customers (id, email, name, metadata, created_at, synced_at)
        VALUES ($1, $2, $3, $4, $5, NOW())
        ON CONFLICT (id) DO UPDATE SET
            email = EXCLUDED.email,
            name = EXCLUDED.name,
            metadata = EXCLUDED.metadata,
            synced_at = NOW()
    `, [customer.id, customer.email, customer.name, customer.metadata, customer.created_at]);
}
```

---

## API Version & Compatibility

### Stripe API Version

The plugin uses Stripe API version **2024-12-18** by default.

You can override this with the `STRIPE_API_VERSION` environment variable.

### API Changelog

Key changes in recent API versions that affect the plugin:

| Version | Changes |
|---------|---------|
| 2024-12-18 | Current default. Full support for all features. |
| 2024-10-28 | Added subscription pause/resume. |
| 2024-09-30 | Updated payment method structure. |
| 2024-06-20 | New invoice rendering options. |

### Breaking Changes

When upgrading Stripe API versions, watch for:
- Field renames or removals in response objects
- New required fields in webhook payloads
- Changed enum values

The plugin handles most version differences automatically, but review the Stripe changelog for your specific version.

### Compatibility Matrix

| nself Version | Plugin Version | Stripe API |
|---------------|----------------|------------|
| 0.4.8+ | 1.0.0 | 2024-12-18 |

---

## Examples

### Example: Get All Active Subscribers with MRR

```bash
# Using CLI
nself plugin stripe subscriptions list --status active --format json | \
    jq '[.[] | {email: .customer_email, mrr: (.amount / 100)}]'
```

```sql
-- Using SQL
SELECT
    c.email,
    s.status,
    SUM(
        CASE
            WHEN p.recurring_interval = 'month' THEN p.unit_amount
            WHEN p.recurring_interval = 'year' THEN p.unit_amount / 12
            ELSE 0
        END
    ) / 100.0 AS monthly_value
FROM np_stripe_subscriptions s
JOIN np_stripe_customers c ON s.customer_id = c.id
JOIN np_stripe_subscription_items si ON s.id = si.subscription_id
JOIN np_stripe_prices p ON si.price_id = p.id
WHERE s.status = 'active'
GROUP BY c.email, s.status
ORDER BY monthly_value DESC;
```

### Example: Find Customers at Risk of Churn

```sql
SELECT
    c.email,
    c.name,
    s.current_period_end,
    s.cancel_at_period_end,
    i.amount / 100.0 AS last_invoice_amount,
    i.status AS last_invoice_status
FROM np_stripe_customers c
JOIN np_stripe_subscriptions s ON c.id = s.customer_id
LEFT JOIN np_stripe_invoices i ON s.latest_invoice = i.id
WHERE s.status = 'active'
  AND (
      s.cancel_at_period_end = TRUE
      OR i.status IN ('open', 'uncollectible')
      OR s.current_period_end < NOW() + INTERVAL '7 days'
  )
ORDER BY s.current_period_end;
```

### Example: Revenue Report by Product

```sql
SELECT
    pr.name AS product_name,
    p.nickname AS price_name,
    p.unit_amount / 100.0 AS price,
    p.currency,
    COUNT(DISTINCT s.id) AS active_subscriptions,
    SUM(p.unit_amount) / 100.0 AS total_mrr
FROM np_stripe_products pr
JOIN np_stripe_prices p ON pr.id = p.product_id
JOIN np_stripe_subscription_items si ON p.id = si.price_id
JOIN np_stripe_subscriptions s ON si.subscription_id = s.id
WHERE s.status = 'active'
GROUP BY pr.name, p.nickname, p.unit_amount, p.currency
ORDER BY total_mrr DESC;
```

---

## Troubleshooting

### Common Issues

#### "Invalid API Key"

```
Error: Invalid API Key provided
```

**Solution:** Check that `STRIPE_API_KEY` (or each key in `STRIPE_API_KEYS`) is set correctly. Ensure you're using the correct key for your environment (test vs live).

```bash
# Verify key format
echo $STRIPE_API_KEY | head -c 10
# Should show: sk_live_xx or sk_test_xx
```

#### "Webhook Signature Verification Failed"

```
Error: Webhook signature verification failed
```

**Solutions:**
1. Verify `STRIPE_WEBHOOK_SECRET` matches the endpoint in Stripe Dashboard
2. Ensure you're using the raw request body for verification
3. Check that the webhook endpoint URL is correct

```bash
# Get the correct secret
# Stripe Dashboard > Developers > Webhooks > [Your Endpoint] > Signing secret
```

#### "Rate Limit Exceeded"

```
Error: Rate limit exceeded
```

**Solution:** The plugin has built-in rate limiting, but if you're hitting limits:
1. Use incremental sync instead of full sync
2. Reduce concurrent operations
3. Check for other applications using the same API key

#### "Database Connection Failed"

```
Error: Connection refused
```

**Solutions:**
1. Verify PostgreSQL is running
2. Check `DATABASE_URL` format
3. Verify database exists and user has permissions

```bash
# Test connection
psql $DATABASE_URL -c "SELECT 1"
```

#### "Missing Required Field"

```
Error: Column 'xxx' cannot be null
```

**Solution:** This usually indicates a Stripe API version mismatch. Update to the expected API version:

```bash
STRIPE_API_VERSION=2024-12-18
```

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
LOG_LEVEL=debug nself plugin stripe sync
```

### Health Checks

```bash
# Check plugin status
nself plugin stripe status

# Check database connectivity
curl http://localhost:3001/health

# Check sync status
curl http://localhost:3001/api/status
```

---

## Performance Considerations

### Rate Limiting

The Stripe plugin includes built-in rate limiting to prevent hitting Stripe's API limits:

- **Default Rate**: 25 requests per second per API key
- **Burst Handling**: Automatic queueing of requests during high load
- **Exponential Backoff**: Automatic retry with exponential backoff on rate limit errors

```typescript
// Configured automatically in client.ts
const rateLimiter = new RateLimiter(25); // 25 req/sec
```

### Database Performance

For optimal performance with large datasets:

```sql
-- Create additional indexes for common query patterns
CREATE INDEX CONCURRENTLY idx_stripe_subscriptions_cancel_at
    ON np_stripe_subscriptions(cancel_at) WHERE cancel_at IS NOT NULL;

CREATE INDEX CONCURRENTLY idx_stripe_invoices_amount
    ON np_stripe_invoices(amount_due DESC) WHERE status = 'open';

CREATE INDEX CONCURRENTLY idx_stripe_customers_balance
    ON np_stripe_customers(balance) WHERE balance != 0;

-- Analyze tables for query optimization
ANALYZE np_stripe_customers;
ANALYZE np_stripe_subscriptions;
ANALYZE np_stripe_invoices;
```

### Sync Optimization

**Incremental Sync Strategy:**
```bash
# Full sync (first time only)
nself plugin stripe sync

# Incremental sync (daily via cron)
0 */6 * * * nself plugin stripe sync --incremental --since "6 hours ago"
```

**Parallel Sync:**
```typescript
// Sync multiple resources in parallel
await Promise.all([
  syncCustomers({ incremental: true }),
  syncSubscriptions({ incremental: true }),
  syncInvoices({ incremental: true }),
]);
```

### Connection Pooling

For high-traffic deployments:

```bash
# Increase PostgreSQL connection pool
DATABASE_URL="postgresql://user:pass@localhost:5432/nself?pool_max=20&pool_min=5"
```

### Memory Management

Monitor memory usage for large syncs:

```bash
# Set Node.js heap size
NODE_OPTIONS="--max-old-space-size=4096" nself plugin stripe sync
```

---

## Security Notes

### API Key Management

**Production Best Practices:**

1. **Use Restricted Keys**: Create a restricted Stripe API key with only read permissions
2. **Key Rotation**: Rotate API keys every 90 days
3. **Environment Separation**: Use different keys for test/live environments
4. **Secret Storage**: Never commit API keys to git; use environment variables

```bash
# Set via environment variable
export STRIPE_API_KEY="sk_live_..."

# Or use a secret manager
aws secretsmanager get-secret-value --secret-id stripe-api-key
```

### Webhook Security

**Signature Verification:**
The plugin automatically verifies all incoming webhooks using Stripe's signature verification:

```typescript
// Automatic verification in webhooks.ts
function verifyWebhookSignature(
  payload: string,
  signature: string,
  secret: string
): boolean {
  // HMAC-SHA256 verification with timestamp validation
  // Protects against replay attacks (5-minute tolerance)
}
```

**Webhook Security Checklist:**
- [ ] HTTPS endpoint (required)
- [ ] Signature verification enabled (STRIPE_WEBHOOK_SECRET set)
- [ ] 5-minute timestamp tolerance enforced
- [ ] Raw request body preserved (no parsing before verification)
- [ ] Webhook events logged for audit trail

### Data Security

**Sensitive Data Handling:**

```sql
-- Encrypt sensitive customer data at rest
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Example: Encrypt customer notes (if needed)
ALTER TABLE np_stripe_customers
    ADD COLUMN encrypted_notes BYTEA;

-- Store encrypted:
-- pgp_sym_encrypt('sensitive text', 'encryption_key')

-- Retrieve decrypted:
-- pgp_sym_decrypt(encrypted_notes, 'encryption_key')
```

**PCI Compliance:**
- Plugin never stores full credit card numbers
- Only stores Stripe payment method IDs (e.g., `pm_xxx`)
- All payment data remains in Stripe's PCI-compliant environment

### Access Control

**Database Permissions:**
```sql
-- Create read-only user for analytics
CREATE USER np_stripe_readonly WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE nself TO np_stripe_readonly;
GRANT USAGE ON SCHEMA public TO np_stripe_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO np_stripe_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO np_stripe_readonly;

-- Create restricted user for plugin (no DELETE)
CREATE USER np_stripe_plugin WITH PASSWORD 'secure_password';
GRANT CONNECT ON DATABASE nself TO np_stripe_plugin;
GRANT USAGE ON SCHEMA public TO np_stripe_plugin;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO np_stripe_plugin;
```

### Network Security

**Firewall Rules:**
```bash
# Allow only Stripe webhook IPs
# Stripe IP ranges: https://stripe.com/docs/ips

# Example iptables rules
iptables -A INPUT -p tcp --dport 3001 -s 3.18.12.63/32 -j ACCEPT
iptables -A INPUT -p tcp --dport 3001 -s 3.130.192.231/32 -j ACCEPT
# ... add all Stripe IPs
```

**Rate Limiting:**
```nginx
# Nginx rate limiting for webhook endpoint
limit_req_zone $binary_remote_addr zone=np_stripe_webhook:10m rate=10r/s;

location /webhook {
    limit_req zone=np_stripe_webhook burst=20;
    proxy_pass http://localhost:3001;
}
```

---

## Advanced Code Examples

### Custom Sync Logic

```typescript
import { StripeClient, DatabaseService } from '@nself/stripe-plugin';

async function syncCustomSegment() {
  const client = new StripeClient(process.env.STRIPE_API_KEY);
  const db = new DatabaseService();

  // Sync only enterprise customers
  const customers = await client.listCustomers({
    limit: 100,
    // Use Stripe metadata for filtering
  });

  for (const customer of customers) {
    if (customer.metadata.segment === 'enterprise') {
      await db.upsertCustomer(customer);
    }
  }
}
```

### Real-time MRR Calculation

```typescript
import { DatabaseService } from '@nself/stripe-plugin';

async function calculateRealTimeMRR(): Promise<number> {
  const db = new DatabaseService();

  const result = await db.query(`
    SELECT SUM(
      CASE
        WHEN p.recurring_interval = 'month' THEN p.unit_amount
        WHEN p.recurring_interval = 'year' THEN p.unit_amount / 12
        WHEN p.recurring_interval = 'week' THEN p.unit_amount * 4.33
        WHEN p.recurring_interval = 'day' THEN p.unit_amount * 30
        ELSE 0
      END * si.quantity
    ) AS mrr_cents
    FROM np_stripe_subscriptions s
    JOIN np_stripe_subscription_items si ON s.id = si.subscription_id
    JOIN np_stripe_prices p ON si.price_id = p.id
    WHERE s.status IN ('active', 'trialing')
  `);

  return result.rows[0].mrr_cents / 100;
}
```

### Churn Prediction

```typescript
async function identifyChurnRisk() {
  const db = new DatabaseService();

  return db.query(`
    SELECT
      c.id,
      c.email,
      c.name,
      s.current_period_end,
      s.cancel_at_period_end,
      COUNT(i.*) FILTER (WHERE i.status = 'open') AS unpaid_invoices,
      COUNT(pi.*) FILTER (WHERE pi.status = 'requires_payment_method') AS failed_payments
    FROM np_stripe_customers c
    JOIN np_stripe_subscriptions s ON c.id = s.customer_id
    LEFT JOIN np_stripe_invoices i ON c.id = i.customer_id
        AND i.created_at > NOW() - INTERVAL '30 days'
    LEFT JOIN np_stripe_payment_intents pi ON c.id = pi.customer_id
        AND pi.created_at > NOW() - INTERVAL '30 days'
    WHERE s.status = 'active'
      AND (
        s.cancel_at_period_end = TRUE
        OR s.current_period_end < NOW() + INTERVAL '7 days'
        OR COUNT(i.*) FILTER (WHERE i.status = 'open') > 0
        OR COUNT(pi.*) FILTER (WHERE pi.status = 'requires_payment_method') > 1
      )
    GROUP BY c.id, c.email, c.name, s.current_period_end, s.cancel_at_period_end
    ORDER BY s.current_period_end
  `);
}
```

### Subscription Lifecycle Webhooks

```typescript
import { FastifyRequest, FastifyReply } from 'fastify';

async function handleSubscriptionWebhook(
  request: FastifyRequest,
  reply: FastifyReply
) {
  const event = request.body as StripeEvent;

  switch (event.type) {
    case 'customer.subscription.created':
      await handleSubscriptionCreated(event.data.object);
      break;

    case 'customer.subscription.trial_will_end':
      // Send reminder email 3 days before trial ends
      const subscription = event.data.object;
      await sendTrialEndingEmail(subscription.customer, subscription.trial_end);
      break;

    case 'customer.subscription.deleted':
      // Log churn and send exit survey
      await logChurnEvent(event.data.object);
      await sendExitSurvey(event.data.object.customer);
      break;

    case 'invoice.payment_failed':
      // Implement dunning management
      const invoice = event.data.object;
      await handleFailedPayment(invoice);
      break;
  }

  reply.send({ received: true });
}

async function handleFailedPayment(invoice: Stripe.Invoice) {
  const attemptCount = invoice.attempt_count || 0;

  if (attemptCount === 1) {
    await sendPaymentFailedEmail(invoice.customer, invoice);
  } else if (attemptCount === 2) {
    await sendFinalNotice(invoice.customer, invoice);
  } else if (attemptCount >= 3) {
    await pauseSubscription(invoice.subscription);
  }
}
```

### Dunning Management

```sql
-- Create dunning management view
CREATE VIEW np_stripe_dunning_candidates AS
SELECT
  c.id AS customer_id,
  c.email,
  c.name,
  s.id AS subscription_id,
  i.id AS invoice_id,
  i.amount_due / 100.0 AS amount,
  i.attempt_count,
  i.next_payment_attempt,
  CASE
    WHEN i.attempt_count = 1 THEN 'Send reminder email'
    WHEN i.attempt_count = 2 THEN 'Send urgent notice'
    WHEN i.attempt_count >= 3 THEN 'Pause subscription'
  END AS recommended_action
FROM np_stripe_customers c
JOIN np_stripe_subscriptions s ON c.id = s.customer_id
JOIN np_stripe_invoices i ON s.latest_invoice = i.id
WHERE i.status IN ('open', 'uncollectible')
  AND s.status = 'active'
ORDER BY i.attempt_count DESC, i.next_payment_attempt;
```

### Revenue Recognition

```sql
-- Deferred revenue calculation
CREATE VIEW np_stripe_deferred_revenue AS
SELECT
  DATE_TRUNC('month', s.current_period_start) AS period_month,
  SUM(
    CASE
      WHEN p.recurring_interval = 'month' THEN p.unit_amount
      WHEN p.recurring_interval = 'year' THEN p.unit_amount / 12
    END * si.quantity
  ) / 100.0 AS monthly_recognized_revenue,
  COUNT(DISTINCT s.id) AS active_subscriptions
FROM np_stripe_subscriptions s
JOIN np_stripe_subscription_items si ON s.id = si.subscription_id
JOIN np_stripe_prices p ON si.price_id = p.id
WHERE s.status IN ('active', 'trialing')
  AND s.current_period_start >= DATE_TRUNC('month', NOW() - INTERVAL '12 months')
GROUP BY DATE_TRUNC('month', s.current_period_start)
ORDER BY period_month DESC;
```

---

## Monitoring & Alerting

### Health Checks

```bash
# Monitor sync health
*/5 * * * * curl -s http://localhost:3001/health | jq -e '.status == "ok"' || alert-team

# Monitor webhook processing
*/10 * * * * psql $DATABASE_URL -c "SELECT COUNT(*) FROM np_stripe_webhook_events WHERE processed = FALSE AND received_at < NOW() - INTERVAL '1 hour'" | grep -q "^0$" || alert-team
```

### Key Metrics to Monitor

```sql
-- Failed webhook events
SELECT COUNT(*) FROM np_stripe_webhook_events
WHERE processed = FALSE
  AND received_at > NOW() - INTERVAL '24 hours';

-- Sync lag (time since last successful sync)
SELECT MAX(synced_at) AS last_sync,
       NOW() - MAX(synced_at) AS lag
FROM np_stripe_customers;

-- Failed payments last 24h
SELECT COUNT(*) FROM np_stripe_payment_intents
WHERE status IN ('requires_payment_method', 'canceled')
  AND created_at > NOW() - INTERVAL '24 hours';

-- Subscription churn rate (monthly)
WITH current_month AS (
  SELECT COUNT(*) AS total
  FROM np_stripe_subscriptions
  WHERE status = 'active'
    AND created_at < DATE_TRUNC('month', NOW())
),
churned_this_month AS (
  SELECT COUNT(*) AS churned
  FROM np_stripe_subscriptions
  WHERE status = 'canceled'
    AND canceled_at >= DATE_TRUNC('month', NOW())
)
SELECT
  churned::DECIMAL / NULLIF(total, 0) * 100 AS churn_rate_pct
FROM current_month, churned_this_month;
```

### Prometheus Metrics

```typescript
import { Registry, Counter, Gauge, Histogram } from 'prom-client';

const registry = new Registry();

// Define metrics
const webhookCounter = new Counter({
  name: 'np_stripe_webhooks_total',
  help: 'Total Stripe webhooks received',
  labelNames: ['type', 'status'],
  registers: [registry]
});

const syncDuration = new Histogram({
  name: 'np_stripe_sync_duration_seconds',
  help: 'Stripe sync duration',
  labelNames: ['resource'],
  registers: [registry]
});

const mrrGauge = new Gauge({
  name: 'np_stripe_mrr_cents',
  help: 'Monthly recurring revenue in cents',
  registers: [registry]
});

// Export metrics endpoint
app.get('/metrics', async (req, reply) => {
  reply.header('Content-Type', registry.contentType);
  return registry.metrics();
});
```

---

## Support

- **GitHub Issues:** [nself-plugins/issues](https://github.com/acamarata/nself-plugins/issues)
- **Stripe Documentation:** [stripe.com/docs/api](https://stripe.com/docs/api)
- **Stripe API Changelog:** [stripe.com/docs/upgrades](https://stripe.com/docs/upgrades)
- **Stripe Support:** [support.stripe.com](https://support.stripe.com)
- **Plugin Documentation:** [github.com/acamarata/nself-plugins/wiki/Stripe](https://github.com/acamarata/nself-plugins/wiki/Stripe)

---

*Last Updated: January 30, 2026*
*Plugin Version: 1.0.0*
*Stripe API Version: 2024-12-18*
*nself Version: 0.4.8+*
