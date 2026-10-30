# Plugin ClawDE Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install plugin-clawde` |
| **Requirements** | nSelf >=1.1.1 |

<!-- END GENERATED:catalog -->

> ClawDE daemon integration backend.

## Install

```bash
nself plugin install plugin-clawde
```

No license key required.

## Description

ClawDE daemon integration backend. Manages session lifecycle, tracks daemon health, and streams events via SSE for the ClawDE AI development environment.

Category: `development`. Current version: `0.1.0`.

## Ports

| Port | Purpose |
|------|---------|
| 3847 | Plugin ClawDE service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_clawde_sessions`
- `np_clawde_events`

## Examples

```bash
nself plugin install plugin-clawde
```

## Source

[`plugins/plugin-clawde/`](https://github.com/nself-org/plugins/tree/main/plugin-clawde)

Manifest: [`plugins/plugin-clawde/plugin.json`](https://github.com/nself-org/plugins/tree/main/plugin-clawde/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
