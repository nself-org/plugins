# Costs Plugin

> Estimates the monthly bill for your install: VPS, Cloudflare, Vercel, Stripe fees, and licenses. **Free — MIT licensed.**

## Install

```bash
nself plugin install costs
```

No license key required.

## Description

Show estimated per-install operational costs: Hetzner VPS, Cloudflare, Vercel, Stripe fees, and installed paid plugin licenses.

This is a CLI plugin: it installs the `nself-costs` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `HETZNER_SERVER_TYPE` | *(see plugin.json)* | Optional. |
| `NSELF_PLUGIN_DIR` | *(see plugin.json)* | Optional. |

## Examples

### Run

```bash
nself-costs
```

## Source

[`plugins/costs/`](https://github.com/nself-org/plugins/tree/main/costs)

Manifest: [`plugins/costs/plugin.json`](https://github.com/nself-org/plugins/tree/main/costs/plugin.json)

## See Also

- [[Infra]] — provision infrastructure via Terraform
- [[Tenant]] — per-tenant billing reports

← [[Home]] →
