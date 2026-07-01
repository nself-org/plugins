# nSentry Local Bug-Report Sync

Turns nSentry server-side issues (CI failures, errors, diagnostics, SSL expiry,
non-operational components) into **timestamped Markdown reports** that each dev
machine pulls into a local inbox (e.g. `.claude/inbox`) — **exactly once per dev**.

## Pieces

### `nsentry-report-writer.sh` (runs on the nSentry/ops server)
Snapshots open issues into `/opt/nself-ops/errors/*.md`. Idempotent per issue-key
(won't rewrite the same open issue). Sources:
- components not `operational`
- SSL certs expiring `< 30d`
- recently ingested errors
- pushed CI failures: `nsentry-report-writer.sh --ci <payload-file>`

Run on a schedule (cron every minute is installed on nself-sentry):
```
* * * * * /opt/nself-ops/bin/nsentry-report-writer.sh
```
CI integration: `nself ci serve` / Forgejo failure hooks call it with `--ci`.

### `nself-sentry-sync` (runs on each dev machine, or via Cascade)
Pulls reports from the server (rsync) into a local inbox, copying each report
**at most once per dev** via a per-dev `consumed.list` manifest.

```bash
nself-sentry-sync --inbox .claude/inbox          # default server + dir
NSENTRY_SERVER=root@<ops-ip> nself-sentry-sync    # override server
```
Env: `NSENTRY_SERVER`, `NSENTRY_REMOTE_DIR`, `NSENTRY_INBOX`, `NSENTRY_DEV_ID`,
`NSENTRY_STATE`. Idempotent — safe to run on a timer (cron/launchd) for
"automatic and instant" delivery.

## Per-dev dedup guarantee
Each dev has its own `consumed.list` (keyed by `NSENTRY_DEV_ID`, default a hash of
host+user). A report is copied into a dev's inbox at most once. With N devs, every
dev receives every report exactly once — verified: dev A and dev B independently
received the same report set; re-runs sync nothing new; new reports sync incrementally.

## Cascade integration
When a project is marked CI=nsentry, Cascade runs `nself-sentry-sync` into the
project's `.claude/inbox` on a short interval, so CI/error reports appear in the
local AI tools automatically. A launchd agent (macOS) or cron entry drives it.

## Runner on a sentry box — ephemeral workspace teardown

### Problem
Self-hosted runners accumulate `node_modules`, Docker layer caches, and build artifacts in `_work/`. On a 4 GB sentry box this fills the disk — kills ops-postgres — status page 500s. This recurred on 160 GB and 80 GB boxes; hourly prune cannot reclaim fast enough once CI backlog is large.

### Canonical fix: `--ephemeral` runners + per-job cleanup

Configure the runner with `--ephemeral` so each job gets a fresh registration and the `_work` directory is wiped after each run. Combine with a post-job cleanup hook.

**Docker Compose snippet (recommended — matches unity-sentry reference impl):**

```yaml
# docker-compose.runner.yml
# Add to your sentry-box compose stack.
services:
  actions-runner:
    image: myoung34/github-runner:latest
    restart: unless-stopped
    environment:
      RUNNER_SCOPE: org                        # or 'repo' for single-repo
      ORG_NAME: nself-org                      # change per project
      LABELS: self-hosted,linux,x64
      EPHEMERAL: "true"                        # wipes _work after each job
      RUNNER_WORKDIR: /tmp/runner-work         # use /tmp, not persistent storage
      ACCESS_TOKEN: ${GITHUB_RUNNER_TOKEN}     # short-lived token via GitHub App
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - runner-config:/home/runner             # persists registration config only
    mem_limit: 2g                              # never starve the nSentry stack
    cpus: "1.5"
    tmpfs:
      - /tmp/runner-work:size=4g               # ephemeral build space in RAM/tmpfs
volumes:
  runner-config:
```

**systemd unit snippet (alternative to compose):**

```ini
# /etc/systemd/system/github-runner.service
[Unit]
Description=GitHub Actions Runner (ephemeral)
After=docker.service
Requires=docker.service

[Service]
Type=simple
User=runner
ExecStartPre=/opt/actions-runner/config.sh \
  --url https://github.com/nself-org \
  --token $(cat /opt/actions-runner/.runner-token) \
  --ephemeral --unattended --replace
ExecStart=/opt/actions-runner/run.sh
ExecStopPost=/bin/rm -rf /opt/actions-runner/_work/*
Restart=on-success
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Disk safeguards (both required alongside ephemeral)

| Script | Cron | What it does |
|---|---|---|
| `disk-guard.sh` | `*/5 * * * *` | Alert at >80%; aggressive prune + pause runner at >90% |
| `disk-prune.sh` | `0 * * * *` | Hourly Docker prune + `_work` cleanup |
| `db-watchdog.sh` | `*/2 * * * *` | Restart postgres/redis on failure; emits alert |

All three MUST be installed on any sentry box running CI. Install to `/opt/nself-ops/bin/` (symlink from the checked-out `free/ci/scripts/`).

### Status page — disk health visibility

`disk-guard.sh` writes `/opt/nself-ops/status/disk.json` on every run:
```json
{"disk_pct": 73, "updated_at": "2026-07-01T12:00:00Z", "threshold_warn": 80, "threshold_crit": 90}
```
Mount this file into the nself-status container (or serve it via a tiny nginx `location /internal/disk` block) so the "Infrastructure" group component can poll it. Add a `disk` component to the `STATUS_PAGE_LAYOUT` Infrastructure group with `check_url: http://localhost/internal/disk` and a custom health-check that returns degraded when `disk_pct >= 80` and down when `disk_pct >= 90`.

### Hard rule: monitoring-only vs CI boxes

A minimal 4 GB sentry box running heavy CI (Rust, Tauri, macOS signing, Windows cross-compile) WILL fill its disk. See `~/Sites/.claude/nsentry-server-standard.md` §11 for the canonical ruling on CI box sizing and when to use GitHub-hosted runners instead.
