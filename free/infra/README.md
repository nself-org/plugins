# nself-infra

Provision nSelf infrastructure with OpenTofu or Terraform.

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
nself infra validate --provider hetzner
nself infra plan     --provider hetzner --domain myapp.com
nself infra apply    --provider hetzner --domain myapp.com --i-accept-cloud-spend
nself infra destroy  --provider hetzner --auto-approve --i-accept-cloud-spend
```

Hetzner is the only shipped provider. `aws`, `gcp`, `azure`, `do` and `linode` are
recognised and exit 1 with "not shipped (DEF-005)"; any other value is a usage error.

`apply` and `destroy` spend money or delete servers, so they need
`--i-accept-cloud-spend` and an interactive terminal on stdin; without either they
refuse and exit 4 before OpenTofu or Terraform is touched. Once past that gate,
`apply` without `--auto-approve` hands your terminal to `tofu` (or `terraform`), so
you answer its own approval prompt. `destroy` also requires `--auto-approve`; the
flag is the confirmation, there is no second prompt. With `--json` the prompt goes to
stderr and stdout carries only the envelope.

`--help --json` (`--help` or `-h` as a flag, not as another flag's value and not after `--`, with `--json`, on the root or
any subcommand) prints one ok v1 envelope with
`command` (for example `infra plan`) and `data.help`, the same text `--help` prints,
and exits 0. Plain `--help` is unchanged. The hidden `help` subcommand (`nself infra help plan --json`) is not covered:
it gives the usual E401 unknown-flag envelope.

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

`provision --user-data-file <path>` sends a cloud-init user-data file to the new
server as `user_data`. The file is opened without following a symlink (a symlink
as the final path component is refused), and the checks run on the open file,
not on the path. It must be a regular file, mode 0600 or stricter (no execute, group, other or special bits), owned by you,
not empty, valid UTF-8 and at most 32768 bytes (the Hetzner limit). Any other
file is refused with E401 before the provider is called. The content can hold a
pairing code, so it is never printed or logged, and `--json` reports only
`user_data: true` or `false`. When user data was sent, a provider error is shown
with its HTTP status only. Linux and macOS only.

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
from cli origin/main and diffs help, refusals, exit codes and the full request
sequence). `--json` cases are not byte-compared: the plugin prints a v1 envelope and
core its legacy JSON, so `scripts/parity-envelope.py` checks the envelope contract,
the exit class and the data mapping, while the request sequence stays byte-exact.
The header of the script lists every documented difference.

## Requirements

OpenTofu or Terraform must be on your `PATH`. The plugin runs `tofu` when it finds
it and falls back to `terraform`. It bundles neither: see
https://opentofu.org/docs/intro/install/ or https://developer.hashicorp.com/terraform.
The Hetzner module is embedded in the plugin and extracted to your user cache
directory on first use.

## Status

Hetzner is shipped. `apply` and `destroy` are behind the spend gate above.
`--force` on `apply` is accepted and ignored (old scripts still parse).

## Environment

| Variable | Purpose |
|---|---|
| `HETZNER_NSELF_TOKEN` | Process environment only. Copied to `HCLOUD_TOKEN` on apply if that is unset. Also the default token for `server` commands. |
| `HCLOUD_TOKEN` | Process environment only. Used directly by the Hetzner Terraform provider and as the fallback token for `server` commands. |
| `NSELF_INFRA_TERRAFORM_HCLOUD_TOKEN` | The only variable the manifest declares, so the CLI fills it from the project `.env` cascade. plan, apply and destroy copy it to `HCLOUD_TOKEN` when that is unset. `server` commands never read it, so a token kept in a project file can never provision, resize or destroy a server. |

## License

MIT.
