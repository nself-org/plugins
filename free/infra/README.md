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

### Hetzner Cloud servers: `nself infra server`

Provision, list, resize and destroy single Hetzner Cloud servers (the former
core `nself server`, same flags and messages).

```bash
nself infra server provision --name ci-runner-3 --type cx22 --location fsn1 --image ubuntu-24.04 --ssh-key deploy
nself infra server list [--label-selector managed-by=nself-cli] [--json]
nself infra server resize  --id 12345 --type cx41 [--upgrade-disk]
nself infra server destroy --id 12345 --snapshot [--snapshot-timeout 20m] [--release-ip]
nself infra server destroy --id 12345 --force-no-backup
```

`destroy` is safe by default and the order of its steps never changes:

1. It refuses to run at all (exit 1, nothing sent to the provider) unless you pass
   `--snapshot` or `--force-no-backup`. `--release-ip` does not waive this.
2. With `--snapshot` it takes a snapshot and waits for it to reach `available`;
   if that fails or times out, the server is NOT deleted.
3. It sets `auto_delete=false` on the server's primary IP(s) so they survive,
   unless `--release-ip` is passed. If that step fails, the server is NOT deleted.
4. Only then does it send the delete.

The same refusals apply with `--json` and in non-interactive runs: there is no
prompt to bypass and no flag that skips a step. `resize` refuses to shrink a disk.

The Hetzner token for `server` commands is read from `--token`, else the
variable named by `--token-env` (default `HETZNER_NSELF_TOKEN`), else
`HCLOUD_TOKEN`, in the process environment only, exactly as core did. It is
never read from a project `.env` file (`--token-env NSELF_INFRA_TERRAFORM_HCLOUD_TOKEN`, the one variable the CLI fills from `.env`, is refused with an error) and never logged. `--json` works on every
`server` subcommand (the manifest's `json: none` marks the machine-surface
envelope, which is not wired yet).

Parity with core is checked by `scripts/parity-server.sh` (builds core `nself`
from cli origin/main and diffs help, refusals and exit codes).

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
| `HETZNER_NSELF_TOKEN` | Process environment only. Copied to `HCLOUD_TOKEN` on apply if that is unset. Also the default token for `server` commands. |
| `HCLOUD_TOKEN` | Process environment only. Used directly by the Hetzner Terraform provider and as the fallback token for `server` commands. |
| `NSELF_INFRA_TERRAFORM_HCLOUD_TOKEN` | The only variable the manifest declares, so the CLI fills it from the project `.env` cascade. plan, apply and destroy copy it to `HCLOUD_TOKEN` when that is unset. `server` commands never read it, so a token kept in a project file can never provision, resize or destroy a server. |

## License

MIT.
