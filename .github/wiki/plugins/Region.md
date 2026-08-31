# Region Plugin

> Adds, inspects, and promotes replica regions for multi-region deployments. **Free — MIT licensed.**

## Install

```bash
nself plugin install region
```

No license key required.

## Description

Multi-region management: add replica regions, list and inspect their status, and promote a region to primary.

This is a CLI plugin: it installs the `nself-region` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Commands

`nself-region` subcommands (installed alongside the plugin):

- `nself-region add`
- `nself-region list`
- `nself-region status`
- `nself-region promote`

## Examples

### Add

```bash
nself-region add
```

### List

```bash
nself-region list
```

## Source

[`plugins/region/`](https://github.com/nself-org/plugins/tree/main/region)

Manifest: [`plugins/region/plugin.json`](https://github.com/nself-org/plugins/tree/main/region/plugin.json)

## See Also

- [[DR]] — disaster recovery: promote, fence, drill
- [[Infra]] — provision infrastructure via Terraform

← [[Home]] →
