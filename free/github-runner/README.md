# github-runner

GitHub Actions self-hosted runner for nSelf. Registers your server as a runner for a GitHub org, letting private repos run CI without GitHub-hosted runners.

## Why

GitHub-hosted runners require a paid org plan for private repos. This plugin runs the [official GitHub Actions runner](https://github.com/actions/runner) on your nSelf server — your private repo CI just works, at no extra cost.

The runner registers with the `ubuntu-latest` label by default, so existing workflows (`runs-on: ubuntu-latest`) work with zero changes.

## Requirements

- nSelf v0.9.0+
- GitHub org with at least one private repo
- GitHub PAT with `admin:org` scope
- `curl`, `tar`, `jq` on the host

## Install

```bash
# Set required env vars first (in your .env file or shell):
export GITHUB_RUNNER_PAT=ghp_xxxxxxxxxxxx
export GITHUB_RUNNER_ORG=your-org

nself plugin install github-runner
```

The installer will:
1. Verify your PAT has the required scope
2. Download the latest GitHub Actions runner binary
3. Register with your GitHub org (uses a registration token fetched via your PAT)
4. Start the runner (systemd service if available, else nohup)

## Configuration

Copy `.env.example` to your nSelf `.env` file and set the values:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GITHUB_RUNNER_PAT` | Yes | — | PAT with `admin:org` scope |
| `GITHUB_RUNNER_ORG` | Yes | — | GitHub org name |
| `GITHUB_RUNNER_NAME` | No | `<hostname>-nself` | Runner display name |
| `GITHUB_RUNNER_LABELS` | No | `self-hosted,linux,x64,ubuntu-latest` | Comma-separated labels |
| `GITHUB_RUNNER_SCOPE` | No | `org` | `org` or `repo` |
| `GITHUB_RUNNER_REPO` | No | — | Repo name (scope=repo only) |
| `GITHUB_RUNNER_GROUP` | No | Default | Runner group |
| `GITHUB_RUNNER_VERSION` | No | latest | Pin a specific version |

## Usage

```bash
# Check if runner is online
nself plugin github-runner status

# View logs (live)
nself plugin github-runner logs

# Stop the runner
nself plugin github-runner stop

# Start the runner
nself plugin github-runner start

# Update to latest runner version
nself plugin github-runner update
```

## Verify in GitHub

After install, your runner appears at:

```
https://github.com/organizations/<your-org>/settings/actions/runners
```

It shows as **Idle** when waiting for jobs and **Active** when running a job.

## Scaling

By default, one runner instance handles one job at a time. For parallel jobs, install on multiple servers — each gets a unique runner name based on hostname.

To run multiple runners on one server, install the plugin multiple times with different names:

```bash
GITHUB_RUNNER_NAME=nself-runner-1 nself plugin install github-runner
# Then manually run a second instance in a different RUNNER_DIR
```

## PAT Creation

1. Go to [github.com/settings/tokens](https://github.com/settings/tokens)
2. Generate new token (classic)
3. Select scope: `admin:org`
4. Copy token → set as `GITHUB_RUNNER_PAT`

For fine-grained tokens, the permission is: **Organization self-hosted runners → Read and Write**.

## Uninstall

```bash
nself plugin uninstall github-runner
```

This stops the runner, removes its GitHub registration, and deletes the binary. The runner disappears from your org's runners list.
