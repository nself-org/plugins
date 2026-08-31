# DLQ Plugin

> Re-enqueues dead-lettered rows from a plugin's failed-processing queue, with a dry-run preview. **Free — MIT licensed.**

## Install

```bash
nself plugin install dlq
```

No license key required.

## Description

Manage dead-letter queues for nSelf plugins: re-enqueue rows that failed processing back to the work queue, with safe row limits and dry-run preview.

This is a CLI plugin: it installs the `nself-dlq` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_API_TOKEN` | — | Optional. |
| `NSELF_API_URL` | *(see plugin.json)* | Optional. |

## Commands

`nself-dlq` subcommands (installed alongside the plugin):

- `nself-dlq replay <plugin>`

## Examples

### Replay

```bash
nself-dlq replay <plugin>
```

## Source

[`plugins/dlq/`](https://github.com/nself-org/plugins/tree/main/dlq)

Manifest: [`plugins/dlq/plugin.json`](https://github.com/nself-org/plugins/tree/main/dlq/plugin.json)

## See Also

- [[Queue]] — inspect background job queues
- [[Cron]] — scheduled job runner

← [[Home]] →
