# Infra Plugin

> Provisions nSelf infrastructure on Hetzner Cloud via OpenTofu/Terraform. **Free — MIT licensed.**

## Install

```bash
nself plugin install infra
```

No license key required.

## Description

Provision nSelf infrastructure with OpenTofu/Terraform: plan, apply and destroy modules for hetzner (others are deferred, DEF-005).

This is a CLI plugin: it installs the `nself-infra` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.2.6`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `HETZNER_NSELF_TOKEN` | — | Optional. Process environment only: default token for `server` commands. |
| `HCLOUD_TOKEN` | — | Optional. Process environment only: OpenTofu/Terraform provider token and `server` fallback. |
| `NSELF_INFRA_TERRAFORM_HCLOUD_TOKEN` | — | Optional. The only variable filled from the project `.env`; plan, apply and destroy only, never `server`. |



### Maturity: Experimental

This plugin is currently experimental. Commands and behavior may change.

### OpenTofu First

Commands `validate`, `plan`, `apply`, and `destroy` prefer OpenTofu (`tofu`) if installed in PATH, and fall back to `terraform`.

### Spend Guard

The `apply` and `destroy` commands incur cloud spend or delete infrastructure. They require the `--i-accept-cloud-spend` flag and must be run in an interactive TTY to proceed.
## Commands

`nself-infra` subcommands (installed alongside the plugin):

- `nself infra validate`
- `nself infra plan`
- `nself infra apply`
- `nself infra destroy`
- `nself-infra server provision|list|resize|destroy` (`nself infra server ...`): Hetzner Cloud server lifecycle.

### Server destroy safety

`nself infra server destroy` refuses, sending nothing to Hetzner, unless `--snapshot` or `--force-no-backup` is given
(also with `--json`; `--release-ip` does not waive it). With `--snapshot` the server is only deleted after the snapshot
reaches `available`; its primary IP(s) are set to `auto_delete=false` first (unless `--release-ip`), and a failure in any
step stops before the delete. The token comes from `--token`, `--token-env` (default `HETZNER_NSELF_TOKEN`) or `HCLOUD_TOKEN` in the process environment only, never from a project `.env` file (`--token-env NSELF_INFRA_TERRAFORM_HCLOUD_TOKEN` is refused).

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
