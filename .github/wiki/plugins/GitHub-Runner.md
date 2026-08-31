# GitHub-Runner Plugin

> Self-hosted GitHub Actions runner so private repos can run CI without metered minutes. **Free — MIT licensed.**

## Install

```bash
nself plugin install github-runner
```

No license key required.

## Description

GitHub Actions self-hosted runner. Registers with your GitHub org and picks up CI jobs tagged `runs-on: ubuntu-latest`, enabling private repos to run CI without GitHub-hosted runners.

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `development`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `GITHUB_RUNNER_PAT` | *(required)* | Required. |
| `GITHUB_RUNNER_ORG` | *(required)* | Required. |
| `GITHUB_RUNNER_NAME` | *(see plugin.json)* | Optional. |
| `GITHUB_RUNNER_LABELS` | *(see plugin.json)* | Optional. |
| `GITHUB_RUNNER_SCOPE` | *(see plugin.json)* | Optional. |
| `GITHUB_RUNNER_REPO` | *(see plugin.json)* | Optional. |
| `GITHUB_RUNNER_GROUP` | *(see plugin.json)* | Optional. |
| `GITHUB_RUNNER_VERSION` | *(see plugin.json)* | Optional. |

## Ports

| Port | Purpose |
|------|---------|
| 3054 | GitHub-Runner service |

## REST API

```
No REST API — registers as a GitHub Actions self-hosted runner and long-polls for jobs
```

## Examples

### Check health

```bash
curl http://localhost:3054/health
```

## Source

[`plugins/github-runner/`](https://github.com/nself-org/plugins/tree/main/github-runner)

Manifest: [`plugins/github-runner/plugin.json`](https://github.com/nself-org/plugins/tree/main/github-runner/plugin.json)

## See Also

- [[Forgejo]] — self-hosted git + Actions runner
- [[CI]] — local lint/test/build/gitleaks gate

← [[Home]] →
