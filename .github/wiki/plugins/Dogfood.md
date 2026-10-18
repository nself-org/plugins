# Dogfood Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install dogfood` |
| **Commands** | `nself dogfood` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Runs 21 read-only production health checks across backups, DR, tenancy, and security.

## Install

```bash
nself plugin install dogfood
```

No license key required.

## Description

Production dogfood audit and reporting: 21 read-only checks covering backups, DR, tenancy, licensing, secrets, migrations, monitoring, security, watchdog, and queue health.

This is a CLI plugin: it installs the `nself-dogfood` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Commands

`nself-dogfood` subcommands (installed alongside the plugin):

- `nself-dogfood audit`
- `nself-dogfood report`

## Examples

### Audit

```bash
nself-dogfood audit
```

### Report

```bash
nself-dogfood report
```

## Source

[`plugins/dogfood/`](https://github.com/nself-org/plugins/tree/main/dogfood)

Manifest: [`plugins/dogfood/plugin.json`](https://github.com/nself-org/plugins/tree/main/dogfood/plugin.json)

## See Also

- [[Audit]] — docs and lint sweeps
- [[Watchdog]] — self-healing container watchdog

← [[Home]] →
