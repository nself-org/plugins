# Forgejo Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install forgejo` |
| **Requirements** | nSelf >=1.1.1 |

<!-- END GENERATED:catalog -->

> Self-hosted Forgejo git forge with Forgejo Actions enabled for external runners.

## Install

```bash
nself plugin install forgejo
```

No license key required.

## Description

Self-hosted Forgejo git forge with Forgejo Actions enabled for external runners. There is no in-stack runner and no Docker socket mount: register runners on a CI node that is not a deploy host. Designed for the ops profile (ops server on staging/prod). See the [plugin README](https://github.com/nself-org/plugins/blob/main/free/forgejo/README.md) for isolation and trust, runner registration and migration from the removed runner.

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `development`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_FORGEJO_ADMIN_USER` | *(required)* | Admin account name, created on first boot. |
| `NSELF_FORGEJO_ADMIN_PASSWORD` | *(required)* | Admin account password. |
| `NSELF_FORGEJO_ADMIN_EMAIL` | *(required)* | Admin account email. |
| `FORGEJO_HTTP_PORT` | `3844` | Optional. HTTP port, published on 127.0.0.1 only. |
| `FORGEJO_SSH_PORT` | `3222` | Optional. SSH port for git over SSH. |
| `FORGEJO_DOMAIN` | `localhost` | Optional. Public hostname used in clone URLs and webhook callbacks. |
| `FORGEJO_DISABLE_REGISTRATION` | `true` | Optional. Self-service sign-up is closed by default; set `false` to open it. |

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
