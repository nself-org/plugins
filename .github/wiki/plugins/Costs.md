# Costs Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install costs` |
| **Commands** | `nself costs` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

<!-- BEGIN GENERATED:machine-surface -->
<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |
|---|---|---|---|---|---|---|
| `nself costs` | - | - | read | document | CLI only (json None) | none |

<!-- END GENERATED:machine-surface -->

> Estimates the monthly bill for your install: VPS, Cloudflare, Vercel, Stripe fees, and licenses.

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
