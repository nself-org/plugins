# nself-cloud

**Bundle:** ɳCloud (internal) | **Tier:** cloud | **Status:** planned | **Port:** 3845

Managed hosting infrastructure plugin for cloud.nself.org. Handles tenant lifecycle, Hetzner server provisioning sagas, Stripe metered billing, custom domain management, and team memberships.

This plugin is internal-only (`visibility: internal`). It powers cloud.nself.org and is not sold as a standalone bundle.

## What it does

- Manages ɳCloud tenant accounts (signup, trial, billing status)
- Runs idempotent Hetzner provisioning sagas (provision → bootstrap → running)
- Records Stripe metered billing events with idempotency guarantees
- Verifies CNAME propagation and tracks Let's Encrypt certificate lifecycle
- Handles team invitations and membership roles per instance
- Supports BYOH (Bring Your Own Hetzner) mode — user supplies API token

## Port

`3845` (reserved in nSelf port registry)

## Auth

All `/api/cloud/*` routes require a Bearer JWT. `/health`, `/ready`, `/webhooks/*`, and public signup/login are unauthenticated.

## Multi-Tenant Convention Wall

This plugin uses `tenant_id UUID` for row-level isolation (Cloud multi-tenancy), NOT `source_account_id`. Do not mix conventions. See PPI `Multi-Tenant Convention Wall — Hard Rule`.

## Basic API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/cloud/signup` | None | Create tenant + start 14-day trial |
| POST | `/api/cloud/login` | None | Email+password or magic link |
| GET  | `/api/cloud/me` | Bearer | Current tenant + billing |
| POST | `/api/cloud/instances` | Bearer | Provision a new instance |
| GET  | `/api/cloud/instances/{id}` | Bearer | Instance detail + health |
| DELETE | `/api/cloud/instances/{id}` | Bearer | Initiate teardown |
| POST | `/api/cloud/instances/{id}/domain` | Bearer | Bind custom domain |
| POST | `/api/cloud/invitations` | Bearer | Invite team member |
| POST | `/webhooks/stripe` | HMAC | Stripe billing events |
| GET  | `/health` | None | Liveness |
| GET  | `/ready` | None | Readiness |

## Install

This plugin is installed automatically as part of the ɳCloud infrastructure. It is not available via `nself plugin install` for end users.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3845` | HTTP listen port |
| `DATABASE_URL` | — | Postgres connection string (required) |
| `HETZNER_API_TOKEN` | — | Hetzner Cloud API token for provisioning (required) |
| `STRIPE_PLATFORM_SECRET_KEY` | — | Stripe platform key (required) |
| `STRIPE_PLATFORM_WEBHOOK_SECRET` | — | Stripe webhook HMAC secret (required) |
| `NSELF_CLOUD_BASE_URL` | — | Base URL for callback URLs (required) |
| `NSELF_DB_ENCRYPTION_KEY` | — | AES-256 key for BYOH token encryption (required) |
| `NCLOUD_PROVISIONING_ENABLED` | `false` | Kill-switch: set true to enable provisioning |
| `BILLING_PROVIDER` | `stripe` | `stripe` or `lemonsqueezy` |
| `NCLOUD_TRIAL_DAYS` | `14` | Free trial length in days |
| `NCLOUD_MANAGEMENT_FEE_CENTS` | `499` | Monthly management fee ($4.99) |
| `NCLOUD_BYOH_FEE_CENTS` | `199` | Monthly BYOH management fee ($1.99) |

## Status

Scaffold. Full provisioning saga (S6.T07), billing integration (S6.T08-T09), and domain management (S6.T10) pending.
