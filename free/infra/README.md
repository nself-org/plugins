# nself-infra

Provision nSelf infrastructure with Terraform.

This is the `nself infra` command family, moved out of the CLI core under
CLI-R11. The core covers the self-hosted backend lifecycle; provisioning cloud
servers is a separate job for the people who need it.

## Install

```bash
nself install infra
```

`nself infra ...` then works exactly as it did before, because the CLI proxies
the command to this plugin's binary.

## Commands

```bash
nself infra plan    --provider hetzner --domain myapp.com
nself infra apply   --provider hetzner --domain myapp.com --force
nself infra destroy --provider hetzner --auto-approve
```

Providers: `aws`, `gcp`, `azure`, `hetzner`, `do`, `linode`.

## Requirements

Terraform must be on your `PATH`. This plugin does not bundle it — see
https://developer.hashicorp.com/terraform for installation.

## Status

`planned`. `nself infra apply` is gated and refuses to run without `--force`,
which is how it shipped in the CLI. `plan` and `destroy` are unrestricted,
though `destroy` requires `--auto-approve`.

## Environment

| Variable | Purpose |
|---|---|
| `HETZNER_NSELF_TOKEN` | Copied to `HCLOUD_TOKEN` on apply if that is unset, so the Hetzner provider authenticates without exporting a second variable. |
| `HCLOUD_TOKEN` | Used directly by the Hetzner Terraform provider. |

## License

MIT.
