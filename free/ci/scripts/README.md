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
