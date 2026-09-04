# nSelf Cloud Plugin

> ɳCloud managed hosting infrastructure: tenant lifecycle, server provisioning saga, Stripe metered billing, custom domain management, and team memberships for cloud.nself.org. **Free — MIT licensed.**

## Install

```bash
nself plugin install nself-cloud
```

No license key required.

## Description

ɳCloud managed hosting infrastructure: tenant lifecycle, server provisioning saga, Stripe metered billing, custom domain management, and team memberships for cloud.nself.org.

Category: `infrastructure`. Current version: `0.1.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `HETZNER_API_TOKEN` | `(required)` | - |
| `STRIPE_PLATFORM_SECRET_KEY` | `(required)` | - |
| `STRIPE_PLATFORM_WEBHOOK_SECRET` | `(required)` | - |
| `NSELF_CLOUD_BASE_URL` | `(required)` | - |
| `NSELF_DB_ENCRYPTION_KEY` | `(required)` | - |
| `PORT` | `3855` | HTTP server port |
| `NCLOUD_PROVISIONING_ENABLED` | `-` | - |
| `BILLING_PROVIDER` | `-` | - |
| `NCLOUD_TRIAL_DAYS` | `-` | - |
| `NCLOUD_MANAGEMENT_FEE_CENTS` | `-` | - |
| `NCLOUD_BYOH_FEE_CENTS` | `-` | - |
| `HETZNER_SSH_KEY_ID` | `-` | - |
| `NCLOUD_PROVISION_TIMEOUT_MINUTES` | `-` | - |
| `LS_API_KEY` | `-` | - |
| `LS_WEBHOOK_SECRET` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3855 | nSelf Cloud service port |

## Database Schema

6 table(s) added to your Postgres database:

- `np_cloud_tenants`
- `np_cloud_instances`
- `np_cloud_billing_events`
- `np_cloud_domains`
- `np_cloud_invitations`
- `np_cloud_team_memberships`

## REST API

```
POST   /api/cloud/signup
POST   /api/cloud/login
GET    /api/cloud/me
POST   /api/cloud/waitlist
POST   /api/cloud/instances
GET    /api/cloud/instances
GET    /api/cloud/instances/{id}
DELETE /api/cloud/instances/{id}
POST   /api/cloud/instances/{id}/domain
GET    /api/cloud/instances/{id}/domain/verify
GET    /api/cloud/instances/{id}/logs
POST   /api/cloud/invitations
```

## Examples

### Health check

```bash
curl http://localhost:3855/health
```

## Source

[`plugins/nself-cloud/`](https://github.com/nself-org/plugins/tree/main/nself-cloud)

Manifest: [`plugins/nself-cloud/plugin.json`](https://github.com/nself-org/plugins/tree/main/nself-cloud/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
