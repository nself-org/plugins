# PayPal Plugin

Complete PayPal payment data integration for nself. Syncs all PayPal data to PostgreSQL with real-time webhook support and OAuth2 authentication.

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
- [Cross-Plugin Integration](#cross-plugin-integration)
- [Troubleshooting](#troubleshooting)

---

## Overview

The PayPal plugin provides complete synchronization of your PayPal account data to a local PostgreSQL database. It supports:

- **14 Database Tables** - Comprehensive coverage of PayPal objects
- **24 Webhook Events** - Real-time updates via PayPal postback verification
- **6 Analytics Views** - Pre-built SQL views for common metrics
- **Full REST API** - Query synced data via HTTP endpoints
- **CLI Interface** - Manage everything from the command line
- **Multi-Account Support** - Sync multiple PayPal accounts into one database
- **OAuth2 Authentication** - Secure client credentials flow with token caching
- **Transaction Search Windowing** - Automatic 31-day window splitting for full history

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Transactions | Transaction Search API results | `paypal_transactions` |
| Orders | Checkout orders | `paypal_orders` |
| Captures | Payment captures | `paypal_captures` |
| Authorizations | Payment authorizations | `paypal_authorizations` |
| Refunds | Refund records | `paypal_refunds` |
| Subscriptions | Billing subscriptions | `paypal_subscriptions` |
| Subscription Plans | Plan definitions | `paypal_subscription_plans` |
| Products | Catalog products | `paypal_products` |
| Disputes | Payment disputes | `paypal_disputes` |
| Payouts | Payout batches | `paypal_payouts` |
| Invoices | Invoice records | `paypal_invoices` |
| Payers | Deduplicated payer records | `paypal_payers` |
| Balances | Balance snapshots | `paypal_balances` |
| Webhook Events | Raw event log | `paypal_webhook_events` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install paypal

# Configure environment
echo "PAYPAL_CLIENT_ID=your_client_id" >> .env
echo "PAYPAL_CLIENT_SECRET=your_client_secret" >> .env
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env

# Sync all data
nself plugin paypal sync

# Start webhook server
nself plugin paypal server --port 3004
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `PAYPAL_CLIENT_ID` | Yes* | - | PayPal OAuth2 client ID |
| `PAYPAL_CLIENT_SECRET` | Yes* | - | PayPal OAuth2 client secret |
| `PAYPAL_CLIENT_IDS` | No | - | Comma-separated client IDs for multi-account sync |
| `PAYPAL_CLIENT_SECRETS` | No | - | Comma-separated client secrets matching `PAYPAL_CLIENT_IDS` |
| `PAYPAL_ACCOUNT_LABELS` | No | - | Comma-separated labels matching `PAYPAL_CLIENT_IDS` |
| `PAYPAL_WEBHOOK_IDS` | No | - | Comma-separated webhook IDs for postback verification |
| `PAYPAL_WEBHOOK_SECRETS` | No | - | Comma-separated webhook secrets for multi-account |
| `PAYPAL_ENVIRONMENT` | No | `live` | PayPal environment (`sandbox` or `live`) |
| `PAYPAL_SYNC_INTERVAL` | No | `3600` | Sync interval in seconds |
| `PORT` | No | `3004` | HTTP server port |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

\* `PAYPAL_CLIENT_ID` and `PAYPAL_CLIENT_SECRET` are required when `PAYPAL_CLIENT_IDS` is not set.

### API Credentials

1. Go to [PayPal Developer Dashboard](https://developer.paypal.com/dashboard/applications)
2. Create or select an app
3. Copy Client ID and Secret (use Live credentials for production)
4. The plugin only needs **read** access

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# PayPal API
PAYPAL_CLIENT_ID=AbCdEfGhIjKlMnOpQrStUvWx...
PAYPAL_CLIENT_SECRET=EfGhIjKlMnOpQrStUvWxYz...
PAYPAL_ENVIRONMENT=live

# Server
PORT=3004
LOG_LEVEL=info
```

### Multi-Account Example

```bash
PAYPAL_CLIENT_IDS=client_id_org1,client_id_org2
PAYPAL_CLIENT_SECRETS=secret_org1,secret_org2
PAYPAL_ACCOUNT_LABELS=charity-main,charity-events
PAYPAL_WEBHOOK_IDS=wh_id_org1,wh_id_org2
```

Each synced record stores its origin in `source_account_id`, so data is unified by default but still account-aware for filtering and audits.

---

## CLI Commands

### Data Synchronization

```bash
# Full sync (all resources)
nself plugin paypal sync

# Incremental sync (only recent data)
nself plugin paypal sync --incremental

# Sync specific account only
nself plugin paypal sync --account charity-main
```

### Reconciliation

```bash
# Re-sync recent data (default 7-day lookback)
nself plugin paypal reconcile

# Custom lookback window
nself plugin paypal reconcile --days 14

# Reconcile specific account
nself plugin paypal reconcile --account charity-events
```

### Server Management

```bash
# Start HTTP server
nself plugin paypal server

# Start on custom port
nself plugin paypal server --port 3004
```

### Status

```bash
# Show sync status and statistics
nself plugin paypal status
```

---

## REST API

### Base URL

```
http://localhost:3004
```

### Health & Status

```http
GET /health          # Basic liveness check
GET /ready           # Readiness check (verifies database)
GET /live            # Liveness check with sync info
GET /status          # Full status with account info and stats
```

### Sync

```http
POST /sync           # Trigger full data sync
POST /reconcile      # Reconcile recent data
```

Both endpoints accept optional JSON body:
```json
{
  "accounts": ["charity-main"]
}
```

### Webhooks

```http
POST /webhooks/paypal    # PayPal webhook receiver
```

PayPal webhooks use postback verification (POST to `/v1/notifications/verify-webhook-signature`), not local HMAC. Multi-account webhook routing is supported.

### Data Queries

```http
GET /api/transactions    # List transactions (limit, offset, status)
GET /api/orders          # List orders (limit, offset)
GET /api/subscriptions   # List subscriptions (limit, offset, status)
GET /api/disputes        # List disputes (limit, offset)
GET /api/refunds         # List refunds (limit, offset)
GET /api/stats           # Aggregated statistics
GET /api/events          # List webhook events (limit)
```

---

## Webhook Events

PayPal webhook verification uses postback to PayPal's API rather than local HMAC signature verification. This adds ~100ms latency per webhook but is PayPal's recommended approach.

### Payment Events

| Event | Description | Action |
|-------|-------------|--------|
| `PAYMENT.CAPTURE.COMPLETED` | Payment captured | Upsert capture record |
| `PAYMENT.CAPTURE.DENIED` | Capture denied | Upsert capture with denied status |
| `PAYMENT.CAPTURE.REFUNDED` | Capture refunded | Upsert refund record |
| `PAYMENT.CAPTURE.REVERSED` | Capture reversed | Upsert capture with reversed status |
| `PAYMENT.CAPTURE.PENDING` | Capture pending | Upsert capture with pending status |

### Order Events

| Event | Description | Action |
|-------|-------------|--------|
| `CHECKOUT.ORDER.COMPLETED` | Order completed | Upsert order record |
| `CHECKOUT.ORDER.APPROVED` | Order approved | Upsert order record |
| `CHECKOUT.ORDER.VOIDED` | Order voided | Upsert order with void status |

### Subscription Events

| Event | Description | Action |
|-------|-------------|--------|
| `BILLING.SUBSCRIPTION.CREATED` | New subscription | Insert subscription |
| `BILLING.SUBSCRIPTION.ACTIVATED` | Subscription activated | Update status |
| `BILLING.SUBSCRIPTION.UPDATED` | Subscription changed | Update subscription |
| `BILLING.SUBSCRIPTION.CANCELLED` | Subscription cancelled | Update status |
| `BILLING.SUBSCRIPTION.SUSPENDED` | Subscription suspended | Update status |
| `BILLING.SUBSCRIPTION.EXPIRED` | Subscription expired | Update status |

### Dispute Events

| Event | Description | Action |
|-------|-------------|--------|
| `CUSTOMER.DISPUTE.CREATED` | New dispute | Insert dispute |
| `CUSTOMER.DISPUTE.UPDATED` | Dispute updated | Update dispute |
| `CUSTOMER.DISPUTE.RESOLVED` | Dispute resolved | Update status |
| `CUSTOMER.DISPUTE.OTHER` | Other dispute event | Log event |

### Payout Events

| Event | Description | Action |
|-------|-------------|--------|
| `PAYMENT.PAYOUTSBATCH.SUCCESS` | Payout succeeded | Upsert payout |
| `PAYMENT.PAYOUTSBATCH.DENIED` | Payout denied | Upsert payout |
| `PAYMENT.PAYOUTSBATCH.PROCESSING` | Payout processing | Upsert payout |

### Invoice & Refund Events

| Event | Description | Action |
|-------|-------------|--------|
| `INVOICING.INVOICE.PAID` | Invoice paid | Upsert invoice |
| `INVOICING.INVOICE.CANCELLED` | Invoice cancelled | Upsert invoice |
| `PAYMENT.SALE.REFUNDED` | Sale refunded | Upsert refund |

---

## Database Schema

All tables include `source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary'` for multi-account support.

### paypal_transactions

```sql
CREATE TABLE paypal_transactions (
    id VARCHAR(255) NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    event_code VARCHAR(50),
    initiation_date TIMESTAMP WITH TIME ZONE,
    updated_date TIMESTAMP WITH TIME ZONE,
    amount NUMERIC(20, 2) DEFAULT 0,
    fee_amount NUMERIC(20, 2),
    currency VARCHAR(10) DEFAULT 'USD',
    status VARCHAR(50),
    subject TEXT,
    note TEXT,
    payer_email VARCHAR(255),
    payer_id VARCHAR(255),
    payer_name VARCHAR(255),
    invoice_id VARCHAR(255),
    custom_field TEXT,
    metadata JSONB DEFAULT '{}',
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### paypal_orders

```sql
CREATE TABLE paypal_orders (
    id VARCHAR(255) NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    status VARCHAR(50),
    intent VARCHAR(50),
    payer_email VARCHAR(255),
    payer_id VARCHAR(255),
    payer_name VARCHAR(255),
    total_amount NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    description TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### paypal_captures

```sql
CREATE TABLE paypal_captures (
    id VARCHAR(255) NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    order_id VARCHAR(255),
    status VARCHAR(50),
    amount NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    fee_amount NUMERIC(20, 2),
    net_amount NUMERIC(20, 2),
    final_capture BOOLEAN DEFAULT false,
    invoice_id VARCHAR(255),
    custom_id VARCHAR(255),
    seller_protection VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### paypal_subscriptions

```sql
CREATE TABLE paypal_subscriptions (
    id VARCHAR(255) NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    plan_id VARCHAR(255),
    status VARCHAR(50),
    subscriber_email VARCHAR(255),
    subscriber_payer_id VARCHAR(255),
    subscriber_name VARCHAR(255),
    start_time TIMESTAMP WITH TIME ZONE,
    quantity VARCHAR(50),
    outstanding_balance NUMERIC(20, 2),
    last_payment_amount NUMERIC(20, 2),
    last_payment_time TIMESTAMP WITH TIME ZONE,
    next_billing_time TIMESTAMP WITH TIME ZONE,
    failed_payments_count INTEGER DEFAULT 0,
    currency VARCHAR(10),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### paypal_disputes

```sql
CREATE TABLE paypal_disputes (
    id VARCHAR(255) NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    reason VARCHAR(100),
    status VARCHAR(50),
    amount NUMERIC(20, 2) DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'USD',
    outcome_code VARCHAR(50),
    refunded_amount NUMERIC(20, 2),
    life_cycle_stage VARCHAR(50),
    channel VARCHAR(50),
    seller_transaction_id VARCHAR(255),
    buyer_transaction_id VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### paypal_payers

```sql
CREATE TABLE paypal_payers (
    id VARCHAR(255) NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    email VARCHAR(255),
    name VARCHAR(255),
    given_name VARCHAR(255),
    surname VARCHAR(255),
    phone VARCHAR(50),
    country_code VARCHAR(10),
    first_seen TIMESTAMP WITH TIME ZONE,
    last_seen TIMESTAMP WITH TIME ZONE,
    total_amount NUMERIC(20, 2) DEFAULT 0,
    transaction_count INTEGER DEFAULT 0,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, source_account_id)
);
```

### paypal_webhook_events

```sql
CREATE TABLE paypal_webhook_events (
    id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(255),
    resource_type VARCHAR(255),
    summary TEXT,
    resource JSONB DEFAULT '{}',
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    processed BOOLEAN DEFAULT false,
    processed_at TIMESTAMP WITH TIME ZONE,
    error TEXT,
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

Additional tables: `paypal_authorizations`, `paypal_refunds`, `paypal_subscription_plans`, `paypal_products`, `paypal_payouts`, `paypal_invoices`, `paypal_balances`.

---

## Analytics Views

### paypal_donation_summary

Aggregate donations by payer with total amounts and donation count.

```sql
SELECT * FROM paypal_donation_summary;
-- payer_id, payer_email, payer_name, source_account_id,
-- donation_count, total_donated, first_donation, last_donation, currency
```

### paypal_active_subscriptions

Active subscriptions with plan details.

```sql
SELECT * FROM paypal_active_subscriptions;
-- id, plan_id, plan_name, subscriber_email, subscriber_name,
-- start_time, next_billing_time, last_payment_amount, currency
```

### paypal_recurring_revenue

Estimated monthly recurring revenue by account.

```sql
SELECT * FROM paypal_recurring_revenue;
-- source_account_id, currency, active_subscriptions, estimated_mrr
```

### paypal_dispute_summary

Dispute statistics grouped by status.

```sql
SELECT * FROM paypal_dispute_summary;
-- source_account_id, status, dispute_count, total_disputed, currency
```

### paypal_top_donors

Ranked donors by total amount.

```sql
SELECT * FROM paypal_top_donors;
-- payer_id, email, name, total_amount, transaction_count, first_seen, last_seen
```

### paypal_unified_payments

Cross-account payment aggregation view.

```sql
SELECT * FROM paypal_unified_payments;
-- payment_id, payment_type, source_account_id, payer_id, payer_email,
-- amount, fee_amount, net_amount, currency, status, description, created_at
```

---

## Cross-Plugin Integration

The PayPal plugin integrates with the Donorbox plugin through cross-reference columns. Donorbox donations include `paypal_transaction_id` which can be joined with `paypal_transactions.id`:

```sql
-- Find PayPal fee for a Donorbox donation
SELECT
    dd.donor_name,
    dd.amount AS donorbox_amount,
    pt.fee_amount AS paypal_fee,
    pt.amount AS paypal_amount
FROM donorbox_donations dd
JOIN paypal_transactions pt ON dd.paypal_transaction_id = pt.id
WHERE dd.paypal_transaction_id IS NOT NULL;
```

---

## Troubleshooting

### "Invalid Client" or OAuth Error

Verify your `PAYPAL_CLIENT_ID` and `PAYPAL_CLIENT_SECRET` are correct and match the `PAYPAL_ENVIRONMENT` setting (sandbox vs live).

### Transaction Search Returns No Results

PayPal Transaction Search requires max 31-day windows. The plugin handles this automatically, but the initial full sync may take time for accounts with years of history.

### Webhook Verification Failed

PayPal uses postback verification (not local HMAC). Ensure:
1. `PAYPAL_WEBHOOK_IDS` matches your webhook configuration in the PayPal dashboard
2. The server can reach `api-m.paypal.com` for verification

### Rate Limiting

The plugin uses a 30 req/sec rate limiter. If you're hitting PayPal's limits, reduce concurrent operations or use incremental sync.

### Debug Mode

```bash
LOG_LEVEL=debug nself plugin paypal sync
```

---

*Last Updated: February 10, 2026*
*Plugin Version: 1.0.0*
*nself Version: 0.4.8+*
