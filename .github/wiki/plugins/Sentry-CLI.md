# Sentry-CLI Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install sentry-cli` |
| **Commands** | `nself sentry`: `alerts channels`, `alerts test`, `incidents ack`, `incidents list`, `incidents resolve`, `login`, `logout`, `monitors add`, `monitors list`, `monitors pause`, `monitors resume`, `monitors rm`, `status`, `status-pages create`, `status-pages list`, `whoami` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

<!-- BEGIN GENERATED:machine-surface -->
<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |
|---|---|---|---|---|---|---|
| `nself sentry alerts channels` | `nself_sentry_alerts_channels` | `/v1/commands/sentry/alerts/channels` | read | document | MCP and HTTP | none |
| `nself sentry alerts test` | - | - | remote | document | CLI only (remote without confirm) | none |
| `nself sentry incidents ack` | `nself_sentry_incidents_ack` | `/v1/commands/sentry/incidents/ack` | write | document | MCP and HTTP | none |
| `nself sentry incidents list` | `nself_sentry_incidents_list` | `/v1/commands/sentry/incidents/list` | read | document | MCP and HTTP | none |
| `nself sentry incidents resolve` | `nself_sentry_incidents_resolve` | `/v1/commands/sentry/incidents/resolve` | remote | document | MCP and HTTP | nonce |
| `nself sentry login` | - | - | write | document | CLI only (surface cli-only) | none |
| `nself sentry logout` | - | - | write | document | CLI only (surface cli-only) | none |
| `nself sentry monitors add` | `nself_sentry_monitors_add` | `/v1/commands/sentry/monitors/add` | write | document | MCP and HTTP | none |
| `nself sentry monitors list` | `nself_sentry_monitors_list` | `/v1/commands/sentry/monitors/list` | read | document | MCP and HTTP | none |
| `nself sentry monitors pause` | `nself_sentry_monitors_pause` | `/v1/commands/sentry/monitors/pause` | remote | document | MCP and HTTP | nonce |
| `nself sentry monitors resume` | `nself_sentry_monitors_resume` | `/v1/commands/sentry/monitors/resume` | remote | document | MCP and HTTP | nonce |
| `nself sentry monitors rm` | `nself_sentry_monitors_rm` | `/v1/commands/sentry/monitors/rm` | destructive | document | MCP and HTTP | nonce |
| `nself sentry status` | `nself_sentry_status` | `/v1/commands/sentry/status` | read | document | MCP and HTTP | none |
| `nself sentry status-pages create` | `nself_sentry_status_pages_create` | `/v1/commands/sentry/status-pages/create` | remote | document | MCP and HTTP | nonce |
| `nself sentry status-pages list` | `nself_sentry_status_pages_list` | `/v1/commands/sentry/status-pages/list` | read | document | MCP and HTTP | none |
| `nself sentry whoami` | `nself_sentry_whoami` | `/v1/commands/sentry/whoami` | read | document | MCP and HTTP | none |

<!-- END GENERATED:machine-surface -->

> Operates a self-hosted ɳSentry instance: monitors, incidents, status pages, and alerts.

## Install

```bash
nself plugin install sentry-cli
```

No license key required.

## Description

ɳSentry operations: monitors, incidents, status pages, alerts, cloud login, and provisioning a self-hosted sentry server.

This is a CLI plugin: it installs the `nself-sentry-cli` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `HETZNER_NSELF_TOKEN` | — | Optional. |
| `HCLOUD_TOKEN` | — | Optional. |

## Commands

`nself-sentry-cli` subcommands (installed alongside the plugin):

- `nself-sentry-cli login`
- `nself-sentry-cli logout`
- `nself-sentry-cli whoami`
- `nself-sentry-cli monitors`
- `nself-sentry-cli incidents`
- `nself-sentry-cli alerts`
- `nself-sentry-cli channels`
- `nself-sentry-cli status-pages`
- `nself-sentry-cli status`
- `nself-sentry-cli provision <project>`
- `nself-sentry-cli sentry-server`

## Examples

### Login

```bash
nself-sentry-cli login
```

### Logout

```bash
nself-sentry-cli logout
```

## JSON output

Every `nself sentry` subcommand accepts `--json`. The shape depends on the
compat mode (`NSELF_V15=1` selects v1.5):

- **v1.5**: one v1 envelope on stdout (`schema_version`, `command`, then `data` or `error`) and nothing else on stdout. Progress and human text go to stderr. A failure prints the error envelope and exits with the status of its class: user 1, infra 2, auth 3.
- **v1.4 (default)**: the commands that printed JSON before keep those bytes exactly (the bare payload, written by cobra to stderr as before; `status` writes its summary to stdout). `login`, `logout` and `sentry-server provision` printed no JSON before, so they print the envelope in both modes.

| Command | v1.5 `data` | v1.4 `--json` |
|---|---|---|
| `monitors list` | `{"monitors": [...]}` | bare array |
| `monitors add`, `pause`, `resume` | `{"monitor": {...}}` | bare monitor |
| `monitors rm` | `{"deleted": "<id>"}` | `{"deleted": "<id>"}` |
| `incidents list` | `{"incidents": [...]}` | bare array |
| `incidents ack`, `resolve` | `{"incident": {...}}` | bare incident |
| `status-pages list` | `{"status_pages": [...]}` | bare array |
| `status-pages create` | `{"status_page": {...}}` | bare page |
| `alerts channels` | `{"channels": [...]}` | bare array |
| `alerts test` | `{"delivered", "detail"}`; a failed delivery is an error envelope (E784) | same object, exit 0 |
| `whoami` | the account object | the account object |
| `status` | `{"overall", "total", "operational", "degraded", "down", "components"}`; exit 1 when not operational, as before | same object |
| `login` | `{"logged_in", "email", "tenant_id", "tier"}` | same (envelope) |
| `logout` | `{"logged_out": true}` | same (envelope) |
| `sentry-server provision` | `{"project", "dry_run", "terraform", "deployed"}` | same (envelope) |

Error codes: `E401` usage, `E780` API key missing or rejected (auth), `E781`
plan quota exceeded, `E782` API request failed, `E783` status page fetch failed,
`E784` test notification not delivered, `E785` credentials file error, `E786`
provision step failed, `E789` unexpected. Error text is fixed: it never carries
an API key, a URL or the upstream error body.

## Source

[`plugins/sentry-cli/`](https://github.com/nself-org/plugins/tree/main/sentry-cli)

Manifest: [`plugins/sentry-cli/plugin.json`](https://github.com/nself-org/plugins/tree/main/sentry-cli/plugin.json)

## See Also

- [[Watchdog]] — self-healing container watchdog
- [[Alerts]] — Prometheus alert rules and silences

← [[Home]] →
