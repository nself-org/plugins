# Flags Plugin

> Command-line control surface for the feature-flags plugin: canary rollouts and kill switches. **Free — MIT licensed.**

## Install

```bash
nself plugin install flags
```

No license key required.

## Description

Manage feature flags served by the feature-flags plugin: list, get, set, history, canary rollouts and kill switches.

This is a CLI plugin: it installs the `nself-flags` binary into your plugin path and runs as a command, not a background service.

Category: `development`. Current version: `1.0.0`.

## Commands

`nself-flags` subcommands (installed alongside the plugin):

- `nself-flags list`
- `nself-flags get <key>`
- `nself-flags set <key>`
- `nself-flags enable <key>`
- `nself-flags disable <key>`
- `nself-flags kill <key>`
- `nself-flags history <key>`
- `nself-flags prune`

## Examples

### List

```bash
nself-flags list
```

### Get

```bash
nself-flags get <key>
```

## Source

[`plugins/flags/`](https://github.com/nself-org/plugins/tree/main/flags)

Manifest: [`plugins/flags/plugin.json`](https://github.com/nself-org/plugins/tree/main/flags/plugin.json)

## See Also

- [[Feature-Flags]] — the plugin flags controls

← [[Home]] →
