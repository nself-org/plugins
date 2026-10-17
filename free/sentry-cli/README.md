# nself-sentry

ɳSentry operations, moved out of the CLI core under CLI-R11.

This plugin provides two commands: `nself sentry` and `nself sentry-server`.

## Install

```bash
nself install sentry-cli
```

Both commands then work exactly as before — the CLI proxies each to its own
binary.

## Commands

```bash
nself sentry login --api-url https://sentry.example.com
nself sentry monitors list
nself sentry incidents list
nself sentry pages list
nself sentry alerts list
nself sentry status
```

`nself sentry status` exits non-zero when the instance is unhealthy, which is
how it behaved in the CLI. Scripts checking that exit code keep working.

```bash
nself sentry-server provision
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

## Requirements

`nself sentry-server provision` chains into `nself build` and `nself start`, so
the nself CLI must be on your `PATH`. Provisioning against Hetzner reads
`HETZNER_NSELF_TOKEN` (copied to `HCLOUD_TOKEN` when that is unset).

## Note on `nself mcp`

The CLI keeps its ɳSentry MCP tools and its own copy of the API client. `mcp` is
a core command, and there is no mechanism for a plugin to contribute MCP tools,
so the two coexist rather than one importing the other.

## License

MIT.
