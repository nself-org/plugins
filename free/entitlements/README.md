# Entitlements Plugin

> Feature gating, subscription plan management, usage quota tracking, and metered billing.

**Tier:** Free (MIT) — no license required.

## Install

```bash
nself plugin install entitlements
nself build
```

## Description

Feature gating, subscription plan management, usage quota tracking, and metered billing.

Define plans, features, and quotas; attach subscriptions and trials; record metered usage and bill on schedule. Integrates with `stripe`/`paypal` for payment but works standalone too. Grants and add-ons cover the common cases (give a user a free month, raise a quota for a single account).

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Required env var |
| `ENTITLEMENTS_PLUGIN_PORT` | No | — | Optional override |
| `ENTITLEMENTS_DEFAULT_CURRENCY` | No | — | Optional override |
| `ENTITLEMENTS_DEFAULT_TRIAL_DAYS` | No | — | Optional override |
| `ENTITLEMENTS_API_KEY` | No | — | Optional override |
| `ENTITLEMENTS_RATE_LIMIT_MAX` | No | — | Optional override |
| `ENTITLEMENTS_RATE_LIMIT_WINDOW_MS` | No | — | Optional override |

Reference vault credentials. Never hardcode secrets.

## Ports

- Default port: `3735`
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_`):

- `np_entitlements_plans`
- `np_entitlements_subscriptions`
- `np_entitlements_features`
- `np_entitlements_quotas`
- `np_entitlements_usage`
- `np_entitlements_addons`
- `np_entitlements_grants`
- `np_entitlements_events`

All tables use `source_account_id` for multi-app isolation where applicable.

## REST API

Public endpoints exposed by the plugin. Internal admin endpoints exist but are not part of the public surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/` | Plugin index / capability list |

Refer to the plugin's OpenAPI spec (under `paid/entitlements/`) for the full route list.

## Examples

Create a plan:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/entitlements/plans -d '{"name":"Pro","price_cents":1999,"interval":"month"}'
```

Subscribe a user:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/entitlements/subscriptions -d '{"user_id":"u_xxx","plan_id":"plan_xxx"}'
```

Record usage:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/entitlements/usage -d '{"user_id":"u_xxx","feature":"api_calls","amount":1}'
```


## Source

MIT licensed, source included in this repository: [`free/entitlements/`](https://github.com/nself-org/plugins/tree/main/free/entitlements)

## See Also

- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the 7-tier pricing matrix
- `.github/docs/bundles.md` for the public-facing bundle guide
- `plugin.json` in this directory for the canonical manifest
