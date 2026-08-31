# K8s Plugin

> Installs and upgrades nSelf on any Kubernetes cluster through the official Helm chart. **Free — MIT licensed.**

## Install

```bash
nself plugin install k8s
```

No license key required.

## Description

Deploy and manage nSelf on any Kubernetes cluster via the official Helm chart: install, upgrade, and status commands wrapping helm.

This is a CLI plugin: it installs the `nself-k8s` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_PLUGIN_LICENSE_KEY` | — | Optional. |
| `KUBECONFIG` | *(see plugin.json)* | Optional. |

## Commands

`nself-k8s` subcommands (installed alongside the plugin):

- `nself-k8s install`
- `nself-k8s upgrade`
- `nself-k8s status`

## Examples

### Install

```bash
nself-k8s install
```

### Upgrade

```bash
nself-k8s upgrade
```

## Source

[`plugins/k8s/`](https://github.com/nself-org/plugins/tree/main/k8s)

Manifest: [`plugins/k8s/plugin.json`](https://github.com/nself-org/plugins/tree/main/k8s/plugin.json)

## See Also

- [[Infra]] — provision infrastructure via Terraform
- [[Watchdog]] — self-healing container watchdog

← [[Home]] →
