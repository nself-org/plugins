# Queue Plugin

> Inspects background job queues for depth, stuck jobs, and lets you retry or purge them. **Free — MIT licensed.**

## Install

```bash
nself plugin install queue
```

No license key required.

## Description

Inspect and manage nSelf background job queues: depth, stuck jobs, retries and purges.

This is a CLI plugin: it installs the `nself-queue` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `POSTGRES_HOST` | *(see plugin.json)* | Optional. |
| `POSTGRES_PORT` | *(see plugin.json)* | Optional. |
| `POSTGRES_DB` | *(see plugin.json)* | Optional. |
| `POSTGRES_USER` | *(see plugin.json)* | Optional. |
| `POSTGRES_PASSWORD` | — | Optional. |

## Commands

`nself-queue` subcommands (installed alongside the plugin):

- `nself-queue list`
- `nself-queue jobs <queue>`
- `nself-queue retry <job-id>`
- `nself-queue purge <queue>`
- `nself-queue cron`

## Examples

### List

```bash
nself-queue list
```

### Jobs

```bash
nself-queue jobs <queue>
```

## Source

[`plugins/queue/`](https://github.com/nself-org/plugins/tree/main/queue)

Manifest: [`plugins/queue/plugin.json`](https://github.com/nself-org/plugins/tree/main/queue/plugin.json)

## See Also

- [[DLQ]] — replay dead-lettered rows
- [[Cron]] — scheduled job runner

← [[Home]] →
