# SMS Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install sms` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> SMS messaging via Twilio.

## Install

```bash
nself plugin install sms
```

No license key required.

## Description

SMS messaging via Twilio. Send, track, and manage opt-outs.

Category: `communication`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `TWILIO_ACCOUNT_SID` | `(required)` | Twilio Account SID |
| `TWILIO_AUTH_TOKEN` | `(required)` | Twilio Auth Token |
| `TWILIO_FROM_NUMBER` | `(required)` | Twilio sending number (E.164) |
| `NSELF_DB_URL` | `(required)` | PostgreSQL connection string |
| `SMS_PLUGIN_PORT` | `9009` | HTTP server port |
| `SMS_RATE_LIMIT_PER_MIN` | `10` | Max SMS per source_account per minute |
| `NSELF_LICENSE_KEY` | `(required)` | License key |

## Ports

| Port | Purpose |
|------|---------|
| 9009 | SMS service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_sms_messages`
- `np_sms_opt_outs`

## Examples

```bash
nself plugin install sms
```

## Source

[`plugins/sms/`](https://github.com/nself-org/plugins/tree/main/sms)

Manifest: [`plugins/sms/plugin.json`](https://github.com/nself-org/plugins/tree/main/sms/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
