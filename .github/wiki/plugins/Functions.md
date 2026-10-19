# Functions Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.11 |
| **Install** | `nself plugin install functions` |
| **Commands** | `nself functions`: `delete`, `deploy`, `invoke`, `list`, `logs` |
| **Requirements** | nSelf >=1.4.12 |

<!-- END GENERATED:catalog -->

> Deploy and manage nself serverless functions.

## Install

```bash
nself plugin install functions
```

No license key required.

## Description

Deploy and manage nself serverless functions

Category: `utilities`. Current version: `1.2.7`.

## JSON output

Every `nself functions` subcommand takes `--json`.

- **v1.5 mode** (`NSELF_V15=1`): stdout carries exactly one v1 envelope (`schema_version`, `command`, then `data` or `error`). Progress and human text go to stderr. Errors use the class exit status: user 1, infra 2, auth 3, destructive_blocked 4.
- **v1.4 mode** (default): `list --json` keeps its pre-contract output (a bare JSON array of `name`, `status`, `url`, `dir`, or the plain "No functions" text when nothing is listed). Every other subcommand had no JSON before, so it prints the envelope in both modes.

| Command | Envelope `data` | Error codes |
|---|---|---|
| `functions list` | `functions` (list of name, status, url, dir) | E761 |
| `functions deploy <file\|dir>` | `name`, `path`, `url` | E401, E761, E765 |
| `functions delete <name> --confirm` | `name`, `deleted` | E401, E403 (no `--confirm`, exit 4), E760, E761 |
| `functions invoke <name>` | `name`, `method`, `status`, `body` (parsed JSON, or text) | E401, E762, E763 (HTTP 400 or above; the body is not echoed) |
| `functions logs <name>` | `name`, `count`, `lines` | E401 (`--json` with `--follow`), E764 |

Error codes E760 to E769 belong to this plugin. Envelope messages are fixed text: they never carry a token, a flag value, a path or an upstream error body.

## Examples

```bash
nself plugin install functions
```

## Source

[`plugins/functions/`](https://github.com/nself-org/plugins/tree/main/functions)

Manifest: [`plugins/functions/plugin.json`](https://github.com/nself-org/plugins/tree/main/functions/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
