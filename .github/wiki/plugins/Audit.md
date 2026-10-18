# Audit Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install audit` |
| **Commands** | `nself audit` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Sweeps READMEs, wiki, docs, SPORT, PPI and PRI for banned words, dead links, and missing anchors.

## Install

```bash
nself plugin install audit
```

No license key required.

## Description

Ecosystem documentation audit: banned words, dead links, and missing anchors across READMEs, wiki, docs, SPORT, PPI, and PRI.

This is a CLI plugin: it installs the `nself-audit` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Commands

`nself-audit` subcommands (installed alongside the plugin):

- `nself-audit docs`

## Examples

### Docs

```bash
nself-audit docs
```

## Source

[`plugins/audit/`](https://github.com/nself-org/plugins/tree/main/audit)

Manifest: [`plugins/audit/plugin.json`](https://github.com/nself-org/plugins/tree/main/audit/plugin.json)

## See Also

- [[API]] — plugin API surface + changelog
- [[Dogfood]] — production health checks

← [[Home]] →
