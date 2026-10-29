# Mail Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install mail` |
| **Commands** | `nself mail` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

<!-- BEGIN GENERATED:machine-surface -->
<!-- Generated from plugin.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Command | MCP tool | HTTP route | Side effect | Output | Exposure | Confirmation |
|---|---|---|---|---|---|---|
| `nself mail` | - | - | read | document | CLI only (json None) | none |

<!-- END GENERATED:machine-surface -->

> Send transactional and broadcast email through the nSelf stack: mux + Postmark pipeline via ping_api, template management, and DKIM verification.

## Install

```bash
nself plugin install mail
```

No license key required.

## Description

Send transactional and broadcast email through the nSelf stack: mux + Postmark pipeline via ping_api, template management, and DKIM verification.

Category: `communication`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_PING_API_URL` | `-` | - |
| `NSELF_PLUGIN_LICENSE_KEY` | `-` | nSelf plugin license key |
| `NSELF_LICENSE_KEY_1` | `-` | - |

## Examples

### Send

```bash
nself mail send
```

### Broadcast

```bash
nself mail broadcast
```

### Status

```bash
nself mail status
```

### Templates

```bash
nself mail templates
```

## Source

[`plugins/mail/`](https://github.com/nself-org/plugins/tree/main/mail)

Manifest: [`plugins/mail/plugin.json`](https://github.com/nself-org/plugins/tree/main/mail/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
