# API Plugin

> Inspect the installed plugin API surface, its deprecation calendar, and its changelog. **Free — MIT licensed.**

## Install

```bash
nself plugin install api
```

No license key required.

## Description

Inspect the nSelf plugin API surface: endpoint probes, deprecation calendar, and the API changelog.

This is a CLI plugin: it installs the `nself-api` binary into your plugin path and runs as a command, not a background service.

Category: `development`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_PLUGIN_DIR` | *(see plugin.json)* | Optional. |

## Commands

`nself-api` subcommands (installed alongside the plugin):

- `nself-api version`
- `nself-api changelog <plugin>`
- `nself-api deprecation-check`

## Examples

### Version

```bash
nself-api version
```

### Changelog

```bash
nself-api changelog <plugin>
```

## Source

[`plugins/api/`](https://github.com/nself-org/plugins/tree/main/api)

Manifest: [`plugins/api/plugin.json`](https://github.com/nself-org/plugins/tree/main/api/plugin.json)

## See Also

- [[Audit]] — docs and lint sweeps
- [[Release]] — the release cascade

← [[Home]] →
