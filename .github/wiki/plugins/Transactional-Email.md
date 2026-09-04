# Transactional Email Plugin

> Provider-agnostic transactional email: template rendering, per-tenant domain management, SPF/DKIM reporting, delivery webhook relay. **Free — MIT licensed.**

## Install

```bash
nself plugin install transactional-email
```

No license key required.

## Description

Provider-agnostic transactional email: template rendering, per-tenant domain management, SPF/DKIM reporting, delivery webhook relay.

Category: `communication`. Current version: `1.1.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `EMAIL_DEFAULT_PROVIDER` | `-` | - |
| `EMAIL_ELASTIC_API_KEY` | `-` | - |
| `EMAIL_SMTP_HOST` | `-` | - |
| `EMAIL_SMTP_PORT` | `3822` | - |
| `EMAIL_SMTP_USER` | `-` | - |
| `EMAIL_SMTP_PASS` | `-` | - |
| `EMAIL_POSTMARK_SERVER_TOKEN` | `-` | - |
| `EMAIL_RATE_LIMIT_RPM` | `-` | - |
| `EMAIL_WEBHOOK_SECRET` | `-` | - |
| `TRANSACTIONAL_EMAIL_PROVIDER` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3822 | Transactional Email service port |

## Database Schema

3 table(s) added to your Postgres database:

- `np_email_domains`
- `np_email_messages`
- `np_email_templates`

## Examples

```bash
nself plugin install transactional-email
```

## Source

[`plugins/transactional-email/`](https://github.com/nself-org/plugins/tree/main/transactional-email)

Manifest: [`plugins/transactional-email/plugin.json`](https://github.com/nself-org/plugins/tree/main/transactional-email/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
