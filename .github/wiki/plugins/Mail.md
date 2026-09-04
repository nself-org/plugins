# Mail Plugin

> Send transactional and broadcast email through the nSelf stack: mux + Postmark pipeline via ping_api, template management, and DKIM verification. **Free — MIT licensed.**

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
