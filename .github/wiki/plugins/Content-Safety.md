# Content Safety Plugin

> Trust-safety evidence, legal holds, spam detection, raid protection, and abuse scoring. **Free — MIT licensed.**

## Install

```bash
nself plugin install content-safety
```

No license key required.

## Description

Trust-safety evidence, legal holds, spam detection, raid protection, and abuse scoring

Category: `compliance`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `CONTENT_SAFETY_PLUGIN_PORT` | `3213` | - |
| `CONTENT_SAFETY_PLUGIN_HOST` | `0.0.0.0` | - |
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `CONTENT_SAFETY_API_KEY` | `(required)` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3213 | Content Safety service port |

## Database Schema

9 table(s) added to your Postgres database:

- `np_cs_evidence`
- `np_cs_legal_holds`
- `np_cs_evidence_exports`
- `np_cs_spam_rules`
- `np_cs_spam_configs`
- `np_cs_rate_limit_violations`
- `np_cs_raid_events`
- `np_cs_lockdowns`
- `np_cs_abuse_scores`

## Examples

```bash
nself plugin install content-safety
```

## Source

[`plugins/content-safety/`](https://github.com/nself-org/plugins/tree/main/content-safety)

Manifest: [`plugins/content-safety/plugin.json`](https://github.com/nself-org/plugins/tree/main/content-safety/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
