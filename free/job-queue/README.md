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

The `np_circuit_breakers` table persists circuit breaker state across service restarts. State values: `closed`, `open`, `half-open`.

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
| `DATABASE_URL` | No | — | Postgres URL for visibility snapshots; jobs route to DLQ without it |
| `JOBQUEUE_PORT` | No | `8213` | HTTP listen port |
| `JOBQUEUE_CONCURRENCY` | No | `5` | Workers per queue |
| `JOBQUEUE_MAX_ATTEMPTS` | No | `8` | Max retries before DLQ |
| `JOBQUEUE_QUEUES` | No | `default,email,ai,media` | Comma-separated queue names |

## Schema

| Table | Purpose |
|---|---|
| `np_jobs` | Visibility snapshot of Redis job state; written by workers |
| `np_job_dlq` | Jobs that exhausted `MAX_ATTEMPTS`; shown in Admin UI DLQ panel |
| `np_circuit_breakers` | Persistent circuit breaker state |

All `np_*` tables include `source_account_id TEXT NOT NULL DEFAULT 'primary'` for multi-app isolation.

## Multi-Tenant Isolation

Jobs are isolated per `source_account_id`. The DLQ panel queries are scoped to the caller's account to prevent cross-tenant data leakage.

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
