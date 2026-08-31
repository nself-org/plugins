# Tenant Plugin

> Creates, suspends, upgrades and destroys tenants, with per-tenant usage and billing reports. **Free — MIT licensed.**

## Install

```bash
nself plugin install tenant
```

No license key required.

## Description

Multi-tenant operations: create, suspend, upgrade and destroy tenants, plus per-tenant usage metering and billing reports.

This is a CLI plugin: it installs the `nself-tenant` binary into your plugin path and runs as a command, not a background service.

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `PROJECT_NAME` | *(see plugin.json)* | Optional. |
| `POSTGRES_HOST` | *(see plugin.json)* | Optional. |
| `POSTGRES_PORT` | *(see plugin.json)* | Optional. |
| `POSTGRES_DB` | *(see plugin.json)* | Optional. |
| `POSTGRES_USER` | *(see plugin.json)* | Optional. |
| `POSTGRES_PASSWORD` | — | Optional. |
| `MINIO_ENABLED` | *(see plugin.json)* | Optional. |
| `STORAGE_ENABLED` | *(see plugin.json)* | Optional. |
| `MINIO_PORT` | *(see plugin.json)* | Optional. |
| `MINIO_ROOT_USER` | *(see plugin.json)* | Optional. |
| `MINIO_ROOT_PASSWORD` | — | Optional. |
| `PROMETHEUS_ENABLED` | *(see plugin.json)* | Optional. |
| `PROMETHEUS_PORT` | *(see plugin.json)* | Optional. |

## Commands

`nself-tenant` subcommands (installed alongside the plugin):

- `nself-tenant create <slug>`
- `nself-tenant suspend <slug>`
- `nself-tenant upgrade <slug>`
- `nself-tenant destroy <slug>`
- `nself-tenant usage <tenant-id>`
- `nself-tenant billing`
- `nself-tenant invoice-preview <tenant-id>`
- `nself-tenant audit <tenant-id>`
- `nself-tenant retry-event <id>`
- `nself-tenant report`

## Examples

### Create

```bash
nself-tenant create <slug>
```

### Suspend

```bash
nself-tenant suspend <slug>
```

## Source

[`plugins/tenant/`](https://github.com/nself-org/plugins/tree/main/tenant)

Manifest: [`plugins/tenant/plugin.json`](https://github.com/nself-org/plugins/tree/main/tenant/plugin.json)

## See Also

- [[GDPR]] — data portability and erasure
- [[Costs]] — per-install operational costs

← [[Home]] →
