# transactional-email

Provider-agnostic transactional email: template rendering, per-tenant domain management, SPF/DKIM reporting, delivery webhook relay.

> **Status: planned.** This plugin is specced (see `plugin.json`) but not yet implemented.

## Details

- **Category:** communication
- **Tier:** pro
- **Language:** go
- **Port:** 3822
- **License:** MIT

## Configuration

| Env var | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | — |
| `EMAIL_DEFAULT_PROVIDER` | No | — |
| `EMAIL_ELASTIC_API_KEY` | No | — |
| `EMAIL_SMTP_HOST` | No | — |
| `EMAIL_SMTP_PORT` | No | — |
| `EMAIL_SMTP_USER` | No | — |
| `EMAIL_SMTP_PASS` | No | — |
| `EMAIL_POSTMARK_SERVER_TOKEN` | No | — |
| `EMAIL_RATE_LIMIT_RPM` | No | — |
| `EMAIL_WEBHOOK_SECRET` | No | — |
| `TRANSACTIONAL_EMAIL_PROVIDER` | No | — |

## Dependencies

Optional: `mux`, `notify`

## Install

```bash
nself plugin install transactional-email
```
