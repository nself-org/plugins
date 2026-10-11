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
- `nself infra server provision --user-data-file <path>`: send a cloud-init user-data file to the new server (see below).

### Providers

Hetzner is the only shipped provider. `aws`, `gcp`, `azure`, `do` and `linode` are recognised and exit 1 with
"not shipped (DEF-005)" (an E401 envelope under `--json`); any other value is a usage error. The Hetzner module is
embedded in the plugin and extracted to the user cache directory on first use. `--state-bucket` is passed to the tool as
`-backend-config=bucket=<name>`; the embedded module keeps local state.

### Approval prompt

`apply` and `destroy` need `--i-accept-cloud-spend` and an interactive terminal on stdin, in both human and `--json` mode;
otherwise they exit 4 before OpenTofu or Terraform runs. Past that gate the child tool gets your stdin. `apply` without
`--auto-approve` therefore shows the tool's own approval prompt and waits for you to type `yes` (under `--json` the prompt
is on stderr and stdout stays the envelope). `plan`, `validate` and `init` get no stdin. `destroy` also requires
`--auto-approve`: that flag is the confirmation, and without it the command refuses (exit 1, or 4 with `--json`).

### Server provision: `--user-data-file`

`nself infra server provision --user-data-file <path>` sends the file to Hetzner as the new server's `user_data`
(cloud-init). The file is opened without following a symlink (a symlink as the final path component is refused), and the
checks run on the open file, not on the path. It must be a regular file, owned by the current user, with no group or other
permission bits (mode 0600 or stricter), non-empty, valid UTF-8 and at most 32768 bytes. Any other file is refused with
E401 before the provider is called. The content can hold a pairing code, so it is never printed or logged. When user data
was sent, the provider error message is withheld and only the HTTP status is shown. Linux and macOS only: on any other OS
the flag is refused.

### Server destroy safety

`nself infra server destroy` refuses, sending nothing to Hetzner, unless `--snapshot` or `--force-no-backup` is given
(also with `--json`; `--release-ip` does not waive it). With `--snapshot` the server is only deleted after the snapshot
reaches `available`; its primary IP(s) are set to `auto_delete=false` first (unless `--release-ip`), and a failure in any
step stops before the delete. The token comes from `--token`, `--token-env` (default `HETZNER_NSELF_TOKEN`) or `HCLOUD_TOKEN` in the process environment only, never from a project `.env` file (`--token-env NSELF_INFRA_TERRAFORM_HCLOUD_TOKEN` is refused).

## JSON output and exit classes

`--json` on any subcommand (`validate`, `plan`, `apply`, `destroy`, `server list`, `server provision`, `server resize`,
`server destroy`) makes stdout carry exactly one v1 envelope (`schema_version`, `command`, then `data` or `error`), written
through `sdk/go/output`. `command` is `infra <subcommand>`, for example `infra plan` or `infra server list`. OpenTofu/Terraform
output and the progress lines go to stderr, so a script can pipe stdout into `jq`. Without `--json` the output is unchanged.
`apply --json` still needs a terminal and `--i-accept-cloud-spend`. Without `--auto-approve` the tool's approval prompt
appears on stderr and you answer it on that terminal; stdout stays the envelope. The manifest declares these subcommands `json: legacy` because the pinned manifest schema accepts only
`none` and `legacy` today (the k8s plugin has the same limit); each prints the v1 envelope.

| Subcommand | `data` |
|------------|--------|
| `validate` | `provider`, `status` (`valid`) |
| `plan` | `provider`, `domain`, `status` (`planned`); the plan itself is on stderr |
| `apply` | `provider`, `domain`, `status` (`applied`), `auto_approve` |
| `destroy` | `provider`, `domain`, `status` (`destroyed`) |
| `server list` | `servers[]` (`id`, `name`, `status`, `server_type`, `location`, `created`, `ipv4`, `ipv6`, `labels`), `count` |
| `server provision` | `server` (same fields as a list entry), `user_data` (boolean: whether a user-data file was sent; the content is never reported) |
| `server resize` | `server_id`, `target_type`, `action` (`id`, `status`, `command`, `progress`) |
| `server destroy` | `server_id`, `snapshot_id` (0 when none), `retained_ips[]`, `released_ips[]` (`id`, `ip`, `type`, `assignee_id`, `auto_delete`) |

`--help --json` (or `-h`, any position, any subcommand, for example `nself infra plan --help --json`) prints one ok
envelope instead of help text: `command` is the command path (`infra`, `infra plan`, `infra server list`), `data` is
`{"help": "<the text plain --help prints>"}`, and the exit status is 0. Plain `--help` is unchanged. The `k8s` plugin
behaves the same way (`command` is `k8s` or `k8s <subcommand>`).

An error envelope carries `error.code`, `message`, optional `cause` and `remediation`, `exit_code` and `class`; the process exit
status equals `exit_code`. Messages are fixed text: they never carry a token, an environment value or a raw provider or
terraform error. A usage error names the flag only: a mistyped `--tokn=VALUE`, `-xVALUE` or `--id VALUE` never puts the value in the envelope. For Hetzner and local failures a redacted one-line diagnostic is printed on stderr, never in the envelope (secret values and everything after "token", "Bearer" or "Authorization" are dropped). `cause` is limited to the terraform exit status or the HTTP status of a Hetzner reply.

Every refusal exits 4 with an E403 envelope and reaches neither terraform nor Hetzner: `apply` and `destroy` without
`--i-accept-cloud-spend` or without a terminal, `destroy` without `--auto-approve`, and `server destroy` without `--snapshot`
or `--force-no-backup`. Without `--json` the exit status is what it always was: 4 for the spend and terminal refusals of
`apply` and `destroy`, 1 for the rest.

| Code | Class (exit) | Meaning |
|------|--------------|---------|
| E401 | user (1) | Usage: missing or unknown flag, an unknown provider, a provider that is not shipped (DEF-005), a bad `--label`. Core code |
| E403 | destructive_blocked (4) | A consent gate refused; nothing was changed or deleted. Core code |
| E740 | infra (2) | Neither `tofu` nor `terraform` is on `PATH` |
| E741 | infra (2) | A terraform command failed (`cause` is the exit status; terraform's output is on stderr) |
| E742 | auth (3) | No Hetzner API token: set `HETZNER_NSELF_TOKEN` (or `HCLOUD_TOKEN`) or pass `--token` |
| E743 | user (1) | `--token-env` names `NSELF_INFRA_TERRAFORM_HCLOUD_TOKEN`, which a `server` command never reads |
| E744 | infra (2) | A Hetzner Cloud operation failed (`cause` is `HTTP <status>` or `network error`; a 401 or 403 is E747). `server destroy` states when the server was not deleted |
| E745 | user (1) | Server not found (Hetzner answered 404 for the id) |
| E746 | user (1) | `server resize` would shrink the disk, which Hetzner cannot do: snapshot, provision a smaller server, destroy the original |
| E747 | auth (3) | Hetzner rejected the token (HTTP 401 or 403): check that it is valid, not revoked, in the right project, and not read-only for a change |
| E749 | infra (2) | Unexpected failure: any error that is not an HTTP reply, a network error or a known refusal (a `server destroy` message says when the server was not deleted) |

The plugin owns E740 to E759; E748 and E750 to E759 are free. E401 and E403 are the core registry's codes, reused
because a plugin cannot register new ones.

## Examples

### Plan

```bash
nself-infra plan
```

### Apply

```bash
nself-infra apply
```

## Changelog

- The pre-contract `--json` of `server list`, `server provision`, `server resize` and `server destroy` (a raw JSON array or
  object on stdout) is replaced by the v1 envelope: the same data now sits under `data` (`servers`, `server`, `action`,
  `server_id`/`snapshot_id`/`retained_ips`/`released_ips`, with snake_case keys), and failures are error envelopes with an
  exit class instead of plain text. The plugin is `planned` and ungated, so there is no compatibility window.
- `validate`, `plan`, `apply` and `destroy` gain `--json` with the same envelope.
- The isatty check is covered by unit tests for every non-terminal input; the positive control (a real terminal passes) is manual, under a pty, for example with `script`.
- `apply` and `destroy` now pass your terminal to `tofu`/`terraform`, so `apply` without `--auto-approve` shows the tool's
  approval prompt instead of failing. `--help --json` prints a v1 envelope. Help text and README name Hetzner as the only
  shipped provider and OpenTofu first. The "neither tofu nor terraform" message names OpenTofu first.
- `apply --help` no longer carries the stale "gated as PLANNED" paragraph; `--force` is accepted and ignored. Redacted stderr diagnostics also cut at "password", "secret", "api_key", "apikey" and "x-auth" and are capped at 300 characters.
- `--json` takes the last value given, like every other flag (`--json=false --json` is on). Failures without terraform output of their own print a redacted diagnostic on stderr.
- The terminal check of `apply` and `destroy` is now a real isatty test. A stdin of `/dev/null` (what a program gives a child by
  default), `/dev/zero`, `/dev/urandom`, a pipe or a file no longer counts as a terminal: those runs exit 4 at the check.
- Hetzner HTTP 401 and 403 are now E747 (auth, exit 3) instead of E744; usage envelopes no longer echo flag values; the manifest
  marks `apply` and `destroy` `surface: cli-only`, declares `--label` and `--ssh-key` as `stringSlice`, and `--force` is described
  as ignored.
- `apply` and `destroy` refusals (spend flag, terminal) exit 4 as before; with `--json` they also print an E403 envelope.

## Source

[`plugins/infra/`](https://github.com/nself-org/plugins/tree/main/infra)

Manifest: [`plugins/infra/plugin.json`](https://github.com/nself-org/plugins/tree/main/infra/plugin.json)

## See Also

- [[K8s]] — deploy nSelf on Kubernetes via Helm
- [[Region]] — multi-region replica management

← [[Home]] →
