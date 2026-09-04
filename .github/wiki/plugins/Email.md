# Email Plugin

> Transactional email via Elastic Email. **Free — MIT licensed.**

## Install

```bash
nself plugin install email
```

No license key required.

## Description

Transactional email via Elastic Email. Send, template, and track emails.

Category: `communication`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `ELASTIC_EMAIL_API_KEY` | `(required)` | Elastic Email API key |
| `ELASTIC_EMAIL_FROM` | `(required)` | Default From address |
| `NSELF_DB_URL` | `(required)` | PostgreSQL connection string |
| `EMAIL_PLUGIN_PORT` | `9008` | HTTP server port |
| `NSELF_LICENSE_KEY` | `(required)` | License key |

## Ports

| Port | Purpose |
|------|---------|
| 9008 | Email service port |

## Database Schema

3 table(s) added to your Postgres database:

- `np_email_messages`
- `np_email_templates`
- `np_email_events`

## Examples

```bash
nself plugin install email
```

## Source

[`plugins/email/`](https://github.com/nself-org/plugins/tree/main/email)

Manifest: [`plugins/email/plugin.json`](https://github.com/nself-org/plugins/tree/main/email/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
