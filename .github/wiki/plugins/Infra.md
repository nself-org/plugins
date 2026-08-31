# Infra Plugin

> Provisions nSelf infrastructure on AWS, GCP, Azure, Hetzner, DigitalOcean or Linode via Terraform. **Free — MIT licensed.**

## Install

```bash
nself plugin install infra
```

No license key required.

## Description

Provision nSelf infrastructure with Terraform: plan, apply and destroy modules for aws, gcp, azure, hetzner, do and linode.

This is a CLI plugin: it installs the `nself-infra` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `HETZNER_NSELF_TOKEN` | — | Optional. |
| `HCLOUD_TOKEN` | — | Optional. |

## Commands

`nself-infra` subcommands (installed alongside the plugin):

- `nself-infra plan`
- `nself-infra apply`
- `nself-infra destroy`

## Examples

### Plan

```bash
nself-infra plan
```

### Apply

```bash
nself-infra apply
```

## Source

[`plugins/infra/`](https://github.com/nself-org/plugins/tree/main/infra)

Manifest: [`plugins/infra/plugin.json`](https://github.com/nself-org/plugins/tree/main/infra/plugin.json)

## See Also

- [[K8s]] — deploy nSelf on Kubernetes via Helm
- [[Region]] — multi-region replica management

← [[Home]] →
