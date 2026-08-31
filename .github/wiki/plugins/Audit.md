# Audit Plugin

> Sweeps READMEs, wiki, docs, SPORT, PPI and PRI for banned words, dead links, and missing anchors. **Free — MIT licensed.**

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
