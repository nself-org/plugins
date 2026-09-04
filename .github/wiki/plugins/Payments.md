# Payments Plugin

> Unified payments abstraction supporting Stripe, PayPal, and Apple/Google Pay with webhook normalization. **Free — MIT licensed.**

## Install

```bash
nself plugin install payments
```

No license key required.

## Description

Unified payments abstraction supporting Stripe, PayPal, and Apple/Google Pay with webhook normalization.

Category: `commerce`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `PAYMENTS_PORT` | `3086` | - |
| `STRIPE_SECRET_KEY` | `-` | - |
| `PAYPAL_CLIENT_ID` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3086 | Payments service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_payments_orders`
- `np_payments_transactions`

## REST API

```
GET    /health
```

## Examples

### Health check

```bash
curl http://localhost:3086/health
```

## Source

[`plugins/payments/`](https://github.com/nself-org/plugins/tree/main/payments)

Manifest: [`plugins/payments/plugin.json`](https://github.com/nself-org/plugins/tree/main/payments/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
