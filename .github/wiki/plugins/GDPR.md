# GDPR Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install gdpr` |
| **Commands** | `nself gdpr` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Handles GDPR Art. 20 data-portability exports and Art. 17 erasure requests.

## Install

```bash
nself plugin install gdpr
```

No license key required.

## Description

GDPR data portability (Art. 20) and right-to-erasure (Art. 17) tools for self-hosted nSelf instances.

This is a CLI plugin: it installs the `nself-gdpr` binary into your plugin path and runs as a command, not a background service.

Category: `compliance`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | *(required)* | Required. |

## Database Schema

1 table(s) added to your Postgres database (prefix: `np_gdpr_`):

- `np_gdpr_requests`

## Commands

`nself-gdpr` subcommands (installed alongside the plugin):

- `nself-gdpr export`
- `nself-gdpr delete`
- `nself-gdpr forget`
- `nself-gdpr list-requests`
- `nself-gdpr status`

## Examples

### Export

```bash
nself-gdpr export
```

### Delete

```bash
nself-gdpr delete
```

## Source

[`plugins/gdpr/`](https://github.com/nself-org/plugins/tree/main/gdpr)

Manifest: [`plugins/gdpr/plugin.json`](https://github.com/nself-org/plugins/tree/main/gdpr/plugin.json)

## See Also

- [[Audit-Log]] — tamper-evident mutation log
- [[Tenant]] — per-tenant usage and billing

← [[Home]] →
