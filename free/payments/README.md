# ɳSelf Payments Plugin (Pro)

Unified payments abstraction supporting Stripe, PayPal, and Apple/Google Pay with normalized webhook handling and multi-provider order management. Port 3086. **Pro plugin — requires a license.**

## Install

```bash
nself license set nself_pro_xxxxx...
nself plugin install payments
```

## What It Does

Provides a single consistent API across payment providers so your application code does not need to know which gateway processed a transaction. The plugin normalizes order state, transaction records, and webhook events from Stripe and PayPal into a unified schema in Postgres.

Key features:
- **Provider abstraction** — identical API surface for Stripe, PayPal, and Apple/Google Pay token processing.
- **Webhook normalization** — incoming Stripe and PayPal webhook events are mapped to a common `np_payments_transactions` event type.
- **Dunning management** — automatic retry logic for failed subscription charges.
- **Metrics** — Prometheus metrics for payment success rate, latency, and provider error rates.

> **Note:** This plugin (`payments`, port 3086) is the *unified abstraction layer*. The `stripe` plugin (port 3740) syncs raw Stripe billing data directly and is a separate, standalone plugin for teams that only use Stripe. Use `payments` when you need multi-provider support or a normalized schema; use `stripe` for deep Stripe-native billing sync.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `PAYMENTS_PORT` | `3086` | Service port |
| `DATABASE_URL` | — | Postgres connection string (required) |
| `STRIPE_SECRET_KEY` | — | Stripe secret key (`sk_live_...` or `sk_test_...`) |
| `PAYPAL_CLIENT_ID` | — | PayPal OAuth2 client ID |
| `PAYPAL_CLIENT_SECRET` | — | PayPal OAuth2 client secret |

## Database Tables

| Table | Purpose |
|-------|---------|
| `np_payments_orders` | Normalized order records across providers |
| `np_payments_transactions` | Individual transaction and webhook event log |

All tables include `source_account_id` for multi-app isolation.

## API

```
GET  /health                        — Liveness check
POST /orders                        — Create an order
GET  /orders/{id}                   — Get order status
POST /orders/{id}/capture           — Capture a PayPal order
POST /webhook/stripe                — Stripe webhook receiver
POST /webhook/paypal                — PayPal webhook receiver
GET  /metrics                       — Prometheus metrics
```
