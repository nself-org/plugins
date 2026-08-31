# Release Plugin

> Runs nSelf's own 12-step release cascade across cli, plugins-pro, admin, and Homebrew. **Free — MIT licensed.**

## Install

```bash
nself plugin install release
```

No license key required.

## Description

Orchestrate the nSelf project's own 12-step release cascade: tag and release cli and plugins-pro, build and push the admin image, and open the Homebrew formula PR.

This is a CLI plugin: it installs the `nself-release` binary into your plugin path and runs as a command, not a background service.

Category: `development`. Current version: `1.0.0`.

## Commands

`nself-release` subcommands (installed alongside the plugin):

- `nself-release status`
- `nself-release check <version>`
- `nself-release release <version>`
- `nself-release rollback <version> <prior-version>`

## Examples

### Status

```bash
nself-release status
```

### Check

```bash
nself-release check <version>
```

## Source

[`plugins/release/`](https://github.com/nself-org/plugins/tree/main/release)

Manifest: [`plugins/release/plugin.json`](https://github.com/nself-org/plugins/tree/main/release/plugin.json)

## See Also

- [[CI]] — local lint/test/build/gitleaks gate
- [[Soak]] — abort/rollback a soak test

← [[Home]] →
