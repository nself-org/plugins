# paypal

PayPal payment data sync with real-time webhook handling — transactions, orders, subscriptions, disputes, payouts, and invoices.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install paypal
nself build
nself start
```

## Overview

The `paypal` plugin syncs your PayPal account data into PostgreSQL and handles incoming webhook events in real time. It covers the full PayPal API surface: one-time payments, subscription billing, disputes, payouts, and invoicing. Multi-account mode allows syncing data from multiple PayPal accounts (e.g. sandbox + production, or multiple business accounts) into isolated rows via `source_account_id`.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `PAYPAL_CLIENT_ID` | Yes | — | PayPal app client ID (from PayPal Developer Dashboard) |
| `PAYPAL_CLIENT_SECRET` | Yes | — | PayPal app client secret |
| `PORT` | No | `3071` | HTTP server port |
| `PAYPAL_ENVIRONMENT` | No | `production` | `production` or `sandbox` |
| `PAYPAL_CLIENT_IDS` | No | — | Comma-separated client IDs for multi-account mode |
| `PAYPAL_CLIENT_SECRETS` | No | — | Comma-separated secrets for multi-account mode |
| `PAYPAL_ACCOUNT_LABELS` | No | — | Comma-separated labels for multi-account mode (e.g. `main,sandbox`) |
| `PAYPAL_WEBHOOK_IDS` | No | — | Comma-separated webhook IDs from PayPal (for signature verification) |
| `PAYPAL_WEBHOOK_SECRETS` | No | — | Comma-separated webhook secrets |
| `PAYPAL_SYNC_INTERVAL` | No | `3600` | Seconds between scheduled full syncs |
| `PLUGIN_INTERNAL_SECRET` | No | — | Shared secret for `X-Plugin-Secret` header authentication |

## Webhook Setup

Register your nSelf instance as a PayPal webhook endpoint:

1. In PayPal Developer Dashboard, go to **My Apps → Your App → Webhooks**
2. Set the URL to `https://your-domain.com/webhooks/paypal`
3. Select the event types you want (see Webhook Events below)
4. Copy the Webhook ID and set `PAYPAL_WEBHOOK_IDS` in your `.env.secrets`

The plugin verifies PayPal HMAC webhook signatures on all incoming payloads.

## HTTP API

All endpoints require the `X-Plugin-Secret` header (except the webhook receiver).

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |
| `POST` | `/webhooks/paypal` | PayPal webhook receiver (HMAC-verified) |
| `POST` | `/sync` | Trigger a full data sync |
| `POST` | `/reconcile` | Re-sync recent records to catch missed webhooks |
| `GET` | `/status` | Show sync status, last sync time, record counts |

## Database Tables

| Table | Purpose |
|---|---|
| `np_paypal_transactions` | Payment transactions — amount, currency, status, payer |
| `np_paypal_orders` | Checkout orders — items, amounts, approval status |
| `np_paypal_captures` | Payment captures — amount captured, status |
| `np_paypal_authorizations` | Payment authorizations — amount authorized, expiry |
| `np_paypal_refunds` | Refund records — original capture, refund amount, reason |
| `np_paypal_subscriptions` | Recurring subscription records — plan, status, billing cycle |
| `np_paypal_subscription_plans` | Subscription plan definitions — price, interval, trial |
| `np_paypal_products` | PayPal Catalog products linked to subscription plans |
| `np_paypal_disputes` | Disputes and chargebacks — reason, status, outcome |
| `np_paypal_payouts` | Payout batches and line items |
| `np_paypal_invoices` | PayPal invoices — line items, totals, payment status |
| `np_paypal_payers` | Payer profiles — name, email, shipping address |
| `np_paypal_balances` | Account balance records |
| `np_paypal_webhook_events` | Incoming webhook event log — type, payload, processed timestamp |

## Usage

```bash
# Sync all PayPal data to database
nself plugin run paypal sync

# Re-sync recent data to catch gaps from missed webhooks
nself plugin run paypal reconcile

# Start the PayPal plugin server
nself plugin run paypal server

# Show sync status and statistics
nself plugin run paypal status
```

## Webhook Events

| Event | Handled |
|---|---|
| `PAYMENT.CAPTURE.COMPLETED` | Sync completed payment capture |
| `PAYMENT.CAPTURE.DENIED` | Sync denied capture |
| `PAYMENT.CAPTURE.REFUNDED` | Sync refund |
| `PAYMENT.CAPTURE.REVERSED` | Sync reversal |
| `PAYMENT.CAPTURE.PENDING` | Sync pending capture |
| `CHECKOUT.ORDER.COMPLETED` | Sync completed checkout order |
| `CHECKOUT.ORDER.APPROVED` | Sync approved order |
| `CHECKOUT.ORDER.VOIDED` | Sync voided order |
| `BILLING.SUBSCRIPTION.CREATED` | Sync new subscription |
| `BILLING.SUBSCRIPTION.ACTIVATED` | Sync activated subscription |
| `BILLING.SUBSCRIPTION.UPDATED` | Sync updated subscription |
| `BILLING.SUBSCRIPTION.CANCELLED` | Sync cancelled subscription |
| `BILLING.SUBSCRIPTION.SUSPENDED` | Sync suspended subscription |
| `BILLING.SUBSCRIPTION.EXPIRED` | Sync expired subscription |
| `CUSTOMER.DISPUTE.CREATED` | Sync new dispute |
| `CUSTOMER.DISPUTE.UPDATED` | Sync updated dispute |
| `CUSTOMER.DISPUTE.RESOLVED` | Sync resolved dispute |
| `PAYMENT.PAYOUTSBATCH.SUCCESS` | Sync successful payout batch |
| `PAYMENT.PAYOUTSBATCH.DENIED` | Sync denied payout batch |
| `INVOICING.INVOICE.PAID` | Sync paid invoice |
| `INVOICING.INVOICE.CANCELLED` | Sync cancelled invoice |
| `PAYMENT.SALE.REFUNDED` | Sync refunded sale |
| `PAYMENT.SALE.COMPLETED` | Sync completed sale |

## Multi-Account Mode

To sync multiple PayPal accounts into isolated rows:

```bash
PAYPAL_CLIENT_IDS=id1,id2
PAYPAL_CLIENT_SECRETS=secret1,secret2
PAYPAL_ACCOUNT_LABELS=main,sandbox
PAYPAL_WEBHOOK_IDS=wh_id1,wh_id2
PAYPAL_WEBHOOK_SECRETS=wh_secret1,wh_secret2
```

Each account's data is stored with `source_account_id` matching the account label.

## Port

The plugin binds to `127.0.0.1:3071`. Access via Nginx proxy or localhost.

## See also

- [plugin-stripe](plugin-stripe.md) — Stripe payment integration
- [plugin-webhooks](plugin-webhooks.md) — outbound webhook delivery
- [nSelf CLI: nself plugin](cmd-plugin.md) — plugin management
