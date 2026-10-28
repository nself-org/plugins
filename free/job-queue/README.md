# ɳSelf Job Queue Plugin

**Port:** 8213
**Bundle:** Unbundled (standalone)
**Custom Service Slot:** CS_10
**License flag:** `NSELF_JOB_QUEUE=true` (env checked at deploy)

The job-queue plugin provides durable background job queuing with Redis as the execution store and Postgres as the visibility and durability layer. It supports multiple named queues, configurable concurrency, priority ordering, exponential backoff retry, and a Dead Letter Queue (DLQ) for jobs that exhaust all retry attempts.

## Features

### Job Queues

Enqueue jobs by name into any configured queue. Workers process jobs with at-least-once delivery using Redis `BRPOPLPUSH`:

```http
POST /jobs/enqueue   # enqueue a new job
GET  /jobs           # list queue depths per queue
```

**Enqueue request:**

```json
{
  "queue": "email",
  "job_type": "send_welcome_email",
  "payload": { "user_id": "abc123" }
}
```

Supported queues (configurable via `JOBQUEUE_QUEUES`): `default`, `email`, `ai`, `media`

### Priority Ordering

Jobs are enqueued with `LPush` (head) for high-priority or standard `LPush` at the queue head. The worker processes jobs in FIFO order per queue. Priority separation is achieved via dedicated queue names (`default`, `email`, `ai`, `media`): route time-sensitive work to a dedicated queue with higher concurrency.

### Retry with Exponential Backoff

Failed jobs are retried up to `JOBQUEUE_MAX_ATTEMPTS` times (default: 8) with exponential backoff: `2^attempt` seconds, capped at 1 hour. On exhaustion, the job moves to the Dead Letter Queue.

### Dead Letter Queue (DLQ)

Failed jobs that exhaust all retry attempts are moved to `np_job_dlq` and visible in the Admin UI DLQ panel:

```http
GET  /dlq              # list failed jobs (most recent first)
POST /dlq/requeue      # requeue a failed job (reset to pending, attempt_count=0)
POST /dlq/discard      # permanently delete a job from the DLQ
```

**Requeue request:**
```json
{ "job_id": "uuid-here" }
```

**Discard request:**
```json
{ "job_id": "uuid-here" }
```

### Circuit Breaker Persistence

The `np_circuit_breakers` table is reserved for a future circuit breaker controller; the Go service currently does not write it. Existing rows are preserved across upgrades. State values: `closed`, `open`, `half-open`.

### Metrics

Prometheus-compatible metrics exposed at `/metrics` (queue depth gauges per queue).

## Installation

```bash
nself plugin install job-queue
```

Verify:

```bash
nself plugin list | grep job-queue
```

## Custom Service Slot

This plugin uses **CS_10** (custom service slot 10). Defined in `F08-SERVICE-INVENTORY.md`. The slot reserves port 8213 and Redis list keys `queue:<name>` / `queue:<name>:processing`.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `REDIS_URL` | Yes | `redis://127.0.0.1:6379` | Redis connection URL |
| `DATABASE_URL` | Yes | — | Postgres URL; the plugin applies its own migrations at start and exits without it |
| `JOBQUEUE_PORT` | No | `8213` | HTTP listen port |
| `JOBQUEUE_CONCURRENCY` | No | `5` | Workers per queue |
| `JOBQUEUE_MAX_ATTEMPTS` | No | `8` | Max retries before DLQ |
| `JOBQUEUE_QUEUES` | No | `default,email,ai,media` | Comma-separated queue names |

## Storage and health

Postgres remains required: the documented job snapshots and DLQ are part of the
service, and ADR 0010 requires boot migrations for its SQL schema. The server
uses the same pool that applied migrations. Enqueue writes the job snapshot
before pushing to Redis; a failed snapshot insert rejects the request. Workers
update status and persist exhausted jobs in the DLQ. Redis remains the execution
store: snapshots are for visibility, not an automatic recovery or replay log.
The two stores do not share a transaction; a Redis failure after a snapshot
insert leaves a failed snapshot, and later worker snapshot failures are logged.

`/health` requires both Redis and migration health. There is no environment
switch to bypass Redis. Both boot test kits start a Redis sidecar when testing
job-queue, wait for it to answer `PING`, and remove it on exit.

## Schema

| Table | Purpose |
|---|---|
| `np_jobqueue_jobs` | Visibility snapshot of Redis job state; written by workers |
| `np_job_dlq` | Jobs that exhausted `MAX_ATTEMPTS`; shown in Admin UI DLQ panel |
| `np_circuit_breakers` | Reserved schema; existing rows retained, no Go controller yet |

The jobs and DLQ tables include `source_account_id TEXT NOT NULL DEFAULT 'primary'`.

## Migrations

`job-queue` applies its migration files itself when it starts (contract `plugin.boot-migrations` v1, `sdk/go/migrate`): file-name
order, one transaction per file, recorded in `np_job_queue.schema_migrations`. The manifest declares `migrations.apply: boot`
and `GET /health` reports `migrations: {applied, expected}` next to the Redis check. The tables live in `public`.

**Upgrading an existing install.** The file is idempotent (`IF NOT EXISTS`), so it applies over tables an earlier version
created: no data is touched or dropped. The first start of the new version creates the ledger.

The Go module root is the plugin root (`go.mod` next to `plugin.json`) so `migrations/` can be embedded; the entry point is `./go`.

| File | Description |
|---|---|
| `001_jobs.sql` | `np_jobqueue_jobs`, `np_job_dlq`, `np_circuit_breakers` |

## Multi-Tenant Isolation

The Go HTTP service currently serves the `primary` account only. Snapshot and DLQ queries restrict rows to `source_account_id='primary'`; the service does not accept a caller-selected account. Do not share its Redis queues between accounts.

## Job Handler Registration

Wire job handlers via the nSelf SDK:

```go
// In your application startup:
client.RegisterJobHandler("send_welcome_email", func(ctx context.Context, payload json.RawMessage) error {
    // decode payload and send email
    return nil
})
```

Unregistered job types fail fast and route through retry/DLQ rather than silently completing.
