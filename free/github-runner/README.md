# github-runner

GitHub Actions self-hosted runner for nSelf. Registers your server as a runner for a GitHub org, letting private repos run CI without GitHub-hosted runners.

## Why

GitHub-hosted runners require a paid org plan for private repos. This plugin runs the [official GitHub Actions runner](https://github.com/actions/runner) on your nSelf server — your private repo CI just works, at no extra cost.

The runner registers with the `ubuntu-latest` label by default, so existing workflows (`runs-on: ubuntu-latest`) work with zero changes.

## Requirements

- nSelf v0.9.0+
- GitHub org with at least one private repo
- A token that can register runners: a fine-grained token with the organisation **Self-hosted runners: write** permission (recommended), or a classic PAT with `admin:org`
- `curl`, `tar`, `jq` on the host

## Install

```bash
# Set required env vars first (in your .env file or shell):
export GITHUB_RUNNER_PAT=<your token>
export GITHUB_RUNNER_ORG=your-org

nself plugin install github-runner
```

The installer will:
1. Verify your PAT has the required scope
2. Check the runner group is trusted-only (see [Isolation and trust](#isolation-and-trust)); install stops with `runner_group_public` if it is open to public repositories
3. Download the latest GitHub Actions runner binary
4. Register with your GitHub org (uses a registration token fetched via your PAT)
5. Start the runner (systemd service if available, else nohup)

## Configuration

Copy `.env.example` to your nSelf `.env` file and set the values:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GITHUB_RUNNER_PAT` | Yes | — | Fine-grained token with org "Self-hosted runners: write" (or classic `admin:org`) |
| `GITHUB_RUNNER_ORG` | Yes | — | GitHub org name |
| `GITHUB_RUNNER_NAME` | No | `<hostname>-nself` | Runner display name |
| `GITHUB_RUNNER_LABELS` | No | `self-hosted,linux,x64,ubuntu-latest` (install.sh); `self-hosted,linux,x64` (compose service) | Comma-separated labels |
| `GITHUB_RUNNER_SCOPE` | No | `org` | `org` or `repo` |
| `GITHUB_RUNNER_REPO` | No | — | Repo name (scope=repo only) |
| `GITHUB_RUNNER_GROUP` | No | the org's default group | Runner group. One open to public repositories is refused |
| `GITHUB_RUNNER_ALLOW_PUBLIC_REPOS` | No | `false` | **Unsafe.** Allow a public runner group or public repo (warned on every start) |
| `ENV` | No | — | `prod`, `production` or `staging` adds `deploy_host_warning` to `/health` and status |
| `GITHUB_RUNNER_VERSION` | No | latest | Pin a specific version |

## Isolation and trust

This runner is **not isolated**. Read this before you point it at a repository.

- **Isolation level: `process`.** Jobs run as ordinary processes of the runner user. There is no container, VM or sandbox around them.
- **Persistent.** The runner keeps its workspace, caches and tool state between jobs. One job can leave files that the next job sees.
- **Same host as your stack.** The plugin runs next to your nSelf services, so a job can reach the host the stack runs on.
- **Not for fork pull requests.** A workflow triggered by a fork PR of a public repository runs attacker-chosen code. Never let this runner take those jobs.

So the plugin is trusted-only by default:

- Before registration, on every start and on every status call it reads the runner group (`GET /orgs/{org}/actions/runner-groups`, matching `GITHUB_RUNNER_GROUP`, or the group with `default: true` when it is empty) and refuses one with `allows_public_repositories: true`. For `GITHUB_RUNNER_SCOPE=repo` it reads `GET /repos/{owner}/{repo}` and refuses a public repository.
- The refusal has the error code `runner_group_public` (HTTP 409 from the plugin API, non-zero exit from `install.sh` and `actions/start.sh`). If the check cannot be completed (API error, missing permission) the runner does not start either (`runner_trust_unverifiable`).
- A group or repository that becomes public after registration stops the running runner on the next start or status call.
- `GITHUB_RUNNER_SCOPE` must be `org` or `repo` (case-insensitive); any other value is refused as `runner_trust_unverifiable`, so a typo never skips the repository check. Organisation and repository names are validated before use.
- A stop signals the runner and all of its child processes (`run.sh` does not forward signals to `Runner.Listener`). For a systemd runner the unit is stopped and also disabled, so a reboot does not bring it back into a public group; the next `start` re-checks the group and starts the runner again when it is private.
- Registered group: with `GITHUB_RUNNER_SCOPE=org` the guard also lists the runners of the configured group (`GET /orgs/{org}/actions/runner-groups/{id}/runners`). A runner that GitHub has registered in another group (for example after `GITHUB_RUNNER_GROUP` was changed without re-registering) is refused as `runner_group_mismatch` and a running one is stopped; a runner that is not registered yet passes. `install` skips this check so that a re-install can move the runner into the configured group.
- Known limits: A check that cannot complete (revoked token, rate limit, GitHub outage, deleted group) blocks a start and is reported by status, but does not stop a runner that is already running; monitor the `Check:` line of the status action. A status call makes one GitHub API request for `GITHUB_RUNNER_SCOPE=repo`, and two for `org` (the group list, then the registered-group runner list); each list is paged at 100 entries, so a large organisation costs one more request per extra page.
- To accept the risk for a public group or repository, set `GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true`. A warning is logged on every start.
- With `ENV=prod`, `production` or `staging`, `/health` and the status action report `deploy_host_warning`, because the runner then shares its host with a deployed stack.

Restrict the runner group to private repositories in the organisation settings (Settings, Actions, Runner groups). The default labels are unchanged, so existing `runs-on: ubuntu-latest` workflows of private repositories still run here instead of on paid GitHub minutes.

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

1. Go to [github.com/settings/personal-access-tokens](https://github.com/settings/personal-access-tokens)
2. Recommended: generate a **fine-grained** token for the organisation with **Self-hosted runners: Read and write**. It reads the runner groups and creates registration tokens, and nothing more. For `GITHUB_RUNNER_SCOPE=repo` the token instead needs **Administration: Read and write** on that repository (and read access to its metadata); the organisation permission alone cannot register or check a repository runner.
3. Or generate a token (classic) with scope `admin:org` (broader than the plugin needs)
4. Copy token → set as `GITHUB_RUNNER_PAT`

## Uninstall

```bash
nself plugin uninstall github-runner
```

This stops the runner, removes its GitHub registration, and deletes the binary. The runner disappears from your org's runners list.
