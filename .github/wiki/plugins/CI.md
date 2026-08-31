# CI Plugin

> Runs the local lint/test/build/gitleaks gate and posts the result as a GitHub commit status. **Free — MIT licensed.**

## Install

```bash
nself plugin install ci
```

No license key required.

## Description

Local CI gate runner: detects repo stack (Go/Node/Flutter/Dart), runs lint+test+build, scans secrets with gitleaks, then posts a GitHub commit status (nself-ci) via gh OAuth. Replaces billing-blocked GitHub Actions as the merge gate.

This is a CLI plugin: it installs the `nself-ci` binary into your plugin path and runs as a command, not a background service.

Category: `development`. Current version: `1.0.1`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_CI_REPO` | *(see plugin.json)* | Optional. |
| `NSELF_CI_SHA` | *(see plugin.json)* | Optional. |
| `NSELF_CI_SKIP_STATUS` | *(see plugin.json)* | Optional. |
| `NSELF_CI_TIMEOUT` | *(see plugin.json)* | Optional. |

## Examples

### Run

```bash
nself-ci
```

## Source

[`plugins/ci/`](https://github.com/nself-org/plugins/tree/main/ci)

Manifest: [`plugins/ci/plugin.json`](https://github.com/nself-org/plugins/tree/main/ci/plugin.json)

## See Also

- [[Forgejo]] — self-hosted git + Actions runner
- [[GitHub-Runner]] — self-hosted GitHub Actions runner

← [[Home]] →
