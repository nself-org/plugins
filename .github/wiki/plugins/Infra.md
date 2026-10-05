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
- `nself-infra server provision|list|resize|destroy` (`nself infra server ...`): Hetzner Cloud server lifecycle.

### Server destroy safety

`nself infra server destroy` refuses, sending nothing to Hetzner, unless `--snapshot` or `--force-no-backup` is given
(also with `--json`; `--release-ip` does not waive it). With `--snapshot` the server is only deleted after the snapshot
reaches `available`; its primary IP(s) are set to `auto_delete=false` first (unless `--release-ip`), and a failure in any
step stops before the delete. The token comes from `--token`, `--token-env` (default `HETZNER_NSELF_TOKEN`) or `HCLOUD_TOKEN`.

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
