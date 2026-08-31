# Forgejo Plugin

> Self-hosted Forgejo git forge plus a Forgejo Actions runner for zero-quota offline CI. **Free — MIT licensed.**

## Install

```bash
nself plugin install forgejo
```

No license key required.

## Description

Self-hosted Forgejo git forge + Forgejo Actions runner. Provides offline CI that executes .github/workflows/*.yml YAML on self-hosted compute, consuming zero GitHub Actions quota. Designed for the ops profile (ops server on staging/prod).

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `development`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_FORGEJO_ADMIN_USER` | *(required)* | Required. |
| `NSELF_FORGEJO_ADMIN_PASSWORD` | — | Required. |
| `NSELF_FORGEJO_ADMIN_EMAIL` | *(required)* | Required. |
| `FORGEJO_RUNNER_REGISTRATION_TOKEN` | — | Required. |
| `FORGEJO_HTTP_PORT` | *(see plugin.json)* | Optional. |
| `FORGEJO_SSH_PORT` | *(see plugin.json)* | Optional. |
| `FORGEJO_DOMAIN` | *(see plugin.json)* | Optional. |
| `FORGEJO_RUNNER_NAME` | *(see plugin.json)* | Optional. |
| `FORGEJO_RUNNER_LABELS` | *(see plugin.json)* | Optional. |
| `FORGEJO_RUNNER_CAPACITY` | *(see plugin.json)* | Optional. |
| `FORGEJO_RUNNER_MEMORY` | *(see plugin.json)* | Optional. |
| `FORGEJO_RUNNER_CPU` | *(see plugin.json)* | Optional. |

## Ports

| Port | Purpose |
|------|---------|
| 3844 | Forgejo service |

## REST API

```
Forgejo's own web UI + REST API at the configured domain
Forgejo Actions runner registers against Forgejo's runner-registration endpoint
```

## Nginx Routes

| Route | Target |
|-------|--------|
| `/` | Forgejo web UI, on its own configured domain |

## Examples

### Check health

```bash
curl http://localhost:3844/health
```

## Source

[`plugins/forgejo/`](https://github.com/nself-org/plugins/tree/main/forgejo)

Manifest: [`plugins/forgejo/plugin.json`](https://github.com/nself-org/plugins/tree/main/forgejo/plugin.json)

## See Also

- [[CI]] — local lint/test/build/gitleaks gate
- [[GitHub-Runner]] — self-hosted GitHub Actions runner

← [[Home]] →
