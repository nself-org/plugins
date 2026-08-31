# Watchdog Plugin

> Self-healing container watchdog with a circuit breaker and Telegram/email escalation. **Free — MIT licensed.**

## Install

```bash
nself plugin install watchdog
```

No license key required.

## Description

Self-healing container watchdog with circuit breaker: status, resets, event history, and TG/email escalation alerts.

This is a CLI plugin: it installs the `nself-watchdog` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `WATCHDOG_PERMANENT_OPEN_THRESHOLD` | *(see plugin.json)* | Optional. |
| `WATCHDOG_TG_BOT_TOKEN` | — | Optional. |
| `WATCHDOG_TG_CHAT_ID` | *(see plugin.json)* | Optional. |
| `WATCHDOG_SMTP_HOST` | *(see plugin.json)* | Optional. |
| `WATCHDOG_SMTP_PORT` | *(see plugin.json)* | Optional. |
| `WATCHDOG_SMTP_FROM` | *(see plugin.json)* | Optional. |
| `WATCHDOG_SMTP_TO` | *(see plugin.json)* | Optional. |
| `WATCHDOG_SMTP_USER` | *(see plugin.json)* | Optional. |
| `WATCHDOG_SMTP_PASS` | *(see plugin.json)* | Optional. |
| `NSELF_ENV` | *(see plugin.json)* | Optional. |

## Commands

`nself-watchdog` subcommands (installed alongside the plugin):

- `nself-watchdog status`
- `nself-watchdog history`
- `nself-watchdog reset <service>`
- `nself-watchdog reset-breakers`
- `nself-watchdog test-alert`

## Examples

### Status

```bash
nself-watchdog status
```

### History

```bash
nself-watchdog history
```

## Source

[`plugins/watchdog/`](https://github.com/nself-org/plugins/tree/main/watchdog)

Manifest: [`plugins/watchdog/plugin.json`](https://github.com/nself-org/plugins/tree/main/watchdog/plugin.json)

## See Also

- [[Dogfood]] — production health checks
- [[Maintenance]] — disk cleanup and log rotation

← [[Home]] →
