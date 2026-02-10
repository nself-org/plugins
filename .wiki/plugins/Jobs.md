# Jobs Plugin

BullMQ-based background job queue with priorities, scheduling, retry logic, and BullBoard dashboard for nself.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Analytics Views](#analytics-views)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Jobs plugin provides a production-ready background job processing system built on BullMQ and Redis. It supports multiple queues, job priorities, cron scheduling, exponential backoff retries, and a BullBoard web dashboard for monitoring.

- **4 Database Tables** - Jobs, results, failures, schedules
- **6 Analytics Views** - Active jobs, failed details, queue stats, type stats, recent failures, scheduled overview
- **3 Queues** - default, high-priority, low-priority
- **4 Priority Levels** - critical, high, normal, low
- **5 Pre-built Job Types** - send-email, http-request, database-backup, file-cleanup, custom
- **BullBoard Dashboard** - Web UI for queue monitoring and management

### Pre-built Job Types

| Type | Description |
|------|-------------|
| `send-email` | Email sending with attachments and CC/BCC support |
| `http-request` | HTTP requests with configurable retry on specific status codes |
| `database-backup` | PostgreSQL backup with optional compression and encryption |
| `file-cleanup` | Clean up completed/failed jobs or old files |
| `custom` | Custom jobs via Hasura Actions for business logic |

---

## Quick Start

```bash
# Install the plugin
nself plugin install jobs

# Configure environment
cp .env.example .env
# Edit .env with Redis URL

# Initialize and verify
nself plugin jobs init

# Start the BullBoard dashboard
nself plugin jobs server

# Start the worker (in another terminal)
nself plugin jobs worker
```

Visit the dashboard at `http://localhost:3105/dashboard`.

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `JOBS_REDIS_URL` | Yes | - | Redis connection string |
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `JOBS_DASHBOARD_ENABLED` | No | `true` | Enable BullBoard web dashboard |
| `JOBS_DASHBOARD_PORT` | No | `3105` | Dashboard HTTP port |
| `JOBS_DASHBOARD_PATH` | No | `/dashboard` | Dashboard URL path |
| `JOBS_DEFAULT_CONCURRENCY` | No | `5` | Jobs processed simultaneously per worker |
| `JOBS_RETRY_ATTEMPTS` | No | `3` | Maximum retry attempts |
| `JOBS_RETRY_DELAY` | No | `5000` | Initial retry delay in milliseconds |
| `JOBS_JOB_TIMEOUT` | No | `60000` | Job timeout in milliseconds |
| `JOBS_CLEAN_COMPLETED_AFTER` | No | `86400000` | Remove completed jobs after (ms, default 24h) |
| `JOBS_CLEAN_FAILED_AFTER` | No | `604800000` | Remove failed jobs after (ms, default 7 days) |

### Example .env File

```bash
# Required
JOBS_REDIS_URL=redis://localhost:6379
DATABASE_URL=postgresql://nself:password@localhost:5432/nself

# Dashboard
JOBS_DASHBOARD_ENABLED=true
JOBS_DASHBOARD_PORT=3105

# Worker
JOBS_DEFAULT_CONCURRENCY=5
JOBS_RETRY_ATTEMPTS=3
JOBS_RETRY_DELAY=5000
JOBS_JOB_TIMEOUT=60000
```

---

## CLI Commands

### Plugin Management

```bash
# Initialize and verify Redis, database, and configuration
nself plugin jobs init

# View job statistics
nself plugin jobs stats

# Queue-specific stats
nself plugin jobs stats --queue default

# Last 48 hours of stats
nself plugin jobs stats --time 48

# Performance metrics
nself plugin jobs stats --performance

# Watch mode (auto-refresh)
nself plugin jobs stats --watch
```

### Server & Worker

```bash
# Start BullBoard dashboard server
nself plugin jobs server

# Start worker for default queue
nself plugin jobs worker

# Start worker for specific queue
nself plugin jobs worker high-priority

# Start with custom concurrency
JOBS_DEFAULT_CONCURRENCY=10 nself plugin jobs worker
```

### Retry Management

```bash
# Retry up to 10 failed jobs
nself plugin jobs retry

# Retry from specific queue
nself plugin jobs retry --queue default --limit 20

# Retry specific job type
nself plugin jobs retry --type send-email

# Retry specific job by ID
nself plugin jobs retry --id <uuid>

# Show retryable jobs without retrying
nself plugin jobs retry --show
```

### Scheduled Jobs

```bash
# List all schedules
nself plugin jobs schedule list

# Show schedule details
nself plugin jobs schedule show <name>

# Create a scheduled job
nself plugin jobs schedule create \
  --name daily-backup \
  --type database-backup \
  --cron "0 2 * * *" \
  --payload '{"database": "production", "destination": "/backups"}' \
  --desc "Daily production database backup"

# Enable/disable a schedule
nself plugin jobs schedule enable <name>
nself plugin jobs schedule disable <name>

# Delete a schedule
nself plugin jobs schedule delete <name>
```

---

## REST API

The plugin exposes a REST API alongside the BullBoard dashboard.

### Base URL

```
http://localhost:3105
```

### Endpoints

#### Health Check

```http
GET /health
```
Returns server health status.

#### Create Job

```http
POST /api/jobs
Content-Type: application/json

{
  "type": "send-email",
  "queue": "default",
  "payload": {
    "to": "user@example.com",
    "subject": "Test",
    "body": "Hello!"
  },
  "options": {
    "priority": "high",
    "maxRetries": 3,
    "delay": 5000
  }
}
```

Creates a new job in the specified queue.

#### Get Job

```http
GET /api/jobs/:id
```
Returns job details including status, progress, result, and failure history.

#### Statistics

```http
GET /api/stats
```
Returns queue-level statistics: waiting, active, completed, failed, and delayed counts.

#### Dashboard

```http
GET /dashboard
```
BullBoard web UI for monitoring queues, inspecting jobs, retrying failures, and pausing/resuming queues.

---

## Webhook Events

N/A - internal service. The Jobs plugin processes background tasks internally. Job completion callbacks are handled through the BullMQ event system rather than external webhooks.

---

## Database Schema

### jobs

Core job metadata and status tracking.

```sql
CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    type VARCHAR(100) NOT NULL,            -- send-email, http-request, etc.
    queue VARCHAR(100) DEFAULT 'default',
    status VARCHAR(50) NOT NULL,           -- pending, active, completed, failed, delayed
    priority INTEGER DEFAULT 0,
    payload JSONB NOT NULL,
    progress INTEGER DEFAULT 0,
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    delay_ms BIGINT DEFAULT 0,
    timeout_ms BIGINT DEFAULT 60000,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_type ON jobs(type);
CREATE INDEX idx_jobs_queue ON jobs(queue);
CREATE INDEX idx_jobs_created ON jobs(created_at DESC);
```

### job_results

Successful job outputs.

```sql
CREATE TABLE job_results (
    id UUID PRIMARY KEY,
    job_id UUID REFERENCES jobs(id),
    result JSONB,
    duration_ms INTEGER,
    completed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_job_results_job ON job_results(job_id);
```

### job_failures

Failed job attempts with stack traces.

```sql
CREATE TABLE job_failures (
    id UUID PRIMARY KEY,
    job_id UUID REFERENCES jobs(id),
    attempt_number INTEGER NOT NULL,
    error_message TEXT,
    error_stack TEXT,
    failed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_job_failures_job ON job_failures(job_id);
CREATE INDEX idx_job_failures_failed ON job_failures(failed_at DESC);
```

### job_schedules

Cron-based recurring job definitions.

```sql
CREATE TABLE job_schedules (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    type VARCHAR(100) NOT NULL,
    queue VARCHAR(100) DEFAULT 'default',
    cron_expression VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    last_run_at TIMESTAMP WITH TIME ZONE,
    next_run_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_job_schedules_enabled ON job_schedules(enabled);
CREATE INDEX idx_job_schedules_next ON job_schedules(next_run_at);
```

---

## Analytics Views

### jobs_active

Currently running jobs.

```sql
CREATE VIEW jobs_active AS
SELECT id, type, queue, priority, payload, started_at, attempts
FROM jobs
WHERE status = 'active'
ORDER BY started_at ASC;
```

### jobs_failed_details

Failed jobs with error details.

```sql
CREATE VIEW jobs_failed_details AS
SELECT
    j.id, j.type, j.queue, j.attempts, j.max_attempts,
    f.error_message, f.error_stack, f.failed_at
FROM jobs j
JOIN job_failures f ON j.id = f.job_id
WHERE j.status = 'failed'
ORDER BY f.failed_at DESC;
```

### queue_stats

Queue-level statistics.

```sql
CREATE VIEW queue_stats AS
SELECT
    queue,
    COUNT(*) FILTER (WHERE status = 'pending') AS pending,
    COUNT(*) FILTER (WHERE status = 'active') AS active,
    COUNT(*) FILTER (WHERE status = 'completed') AS completed,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed,
    COUNT(*) FILTER (WHERE status = 'delayed') AS delayed
FROM jobs
GROUP BY queue;
```

### job_type_stats

Job statistics grouped by type.

```sql
CREATE VIEW job_type_stats AS
SELECT
    type,
    COUNT(*) AS total,
    COUNT(*) FILTER (WHERE status = 'completed') AS completed,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed,
    AVG(r.duration_ms) AS avg_duration_ms
FROM jobs j
LEFT JOIN job_results r ON j.id = r.job_id
GROUP BY type
ORDER BY total DESC;
```

### recent_failures

Failures from the last 24 hours.

```sql
CREATE VIEW recent_failures AS
SELECT j.id, j.type, j.queue, f.error_message, f.failed_at
FROM jobs j
JOIN job_failures f ON j.id = f.job_id
WHERE f.failed_at > NOW() - INTERVAL '24 hours'
ORDER BY f.failed_at DESC;
```

### scheduled_jobs_overview

Overview of all scheduled recurring jobs.

```sql
CREATE VIEW scheduled_jobs_overview AS
SELECT name, type, queue, cron_expression, description, enabled, last_run_at, next_run_at
FROM job_schedules
ORDER BY enabled DESC, next_run_at ASC;
```

---

## Troubleshooting

### Common Issues

#### "Redis Connection Issues"

```
Error: Redis connection to localhost:6379 failed
```

**Solutions:**
1. Test Redis: `redis-cli -h localhost -p 6379 ping`
2. Verify `JOBS_REDIS_URL` in `.env`

#### "Database Issues"

```
Error: relation "jobs" does not exist
```

**Solution:** Initialize the database schema.

```bash
nself plugin jobs init
```

#### "Jobs Not Processing"

**Solutions:**
1. Check if worker is running: `ps aux | grep "nself plugin jobs worker"`
2. Check worker logs: `tail -f ~/.nself/logs/plugins/jobs/worker.log`
3. Verify queue has jobs: `redis-cli llen bull:default:waiting`

#### "Failed Jobs Not Retrying"

**Solutions:**
1. Check retry configuration: `nself plugin jobs stats`
2. View retryable jobs: `nself plugin jobs retry --show`
3. Manual retry: `nself plugin jobs retry --limit 50`

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug nself plugin jobs worker
```

### Health Checks

```bash
# Check server health
curl http://localhost:3105/health

# Check statistics
curl http://localhost:3105/api/stats

# View dashboard
open http://localhost:3105/dashboard
```

---

## Performance Considerations

### Queue Configuration

Optimize your queue setup based on workload patterns:

```typescript
// High-throughput configuration
const queueOptions = {
  defaultJobOptions: {
    removeOnComplete: 1000,  // Keep last 1000 completed jobs
    removeOnFail: 5000,       // Keep last 5000 failed jobs for debugging
    attempts: 3,
    backoff: {
      type: 'exponential',
      delay: 2000,
    },
  },
  settings: {
    lockDuration: 30000,      // Lock jobs for 30s
    stalledInterval: 30000,    // Check for stalled jobs every 30s
    maxStalledCount: 2,        // Retry stalled jobs twice
  },
};
```

### Worker Scaling

Scale workers based on job volume and complexity:

**Single Server - Multiple Workers:**
```bash
# Terminal 1: High-priority queue (2 concurrent jobs)
JOBS_DEFAULT_CONCURRENCY=2 nself plugin jobs worker high-priority

# Terminal 2: Default queue (10 concurrent jobs)
JOBS_DEFAULT_CONCURRENCY=10 nself plugin jobs worker default

# Terminal 3: Low-priority queue (20 concurrent jobs)
JOBS_DEFAULT_CONCURRENCY=20 nself plugin jobs worker low-priority
```

**Multi-Server Deployment:**
```bash
# Server 1: Dedicated to high-priority jobs
JOBS_REDIS_URL=redis://cluster:6379 \
JOBS_DEFAULT_CONCURRENCY=5 \
nself plugin jobs worker high-priority

# Server 2: Default queue processing
JOBS_REDIS_URL=redis://cluster:6379 \
JOBS_DEFAULT_CONCURRENCY=15 \
nself plugin jobs worker default

# Server 3: Batch processing (low-priority)
JOBS_REDIS_URL=redis://cluster:6379 \
JOBS_DEFAULT_CONCURRENCY=30 \
nself plugin jobs worker low-priority
```

### Redis Optimization

**Connection Pooling:**
```bash
# Redis connection with pooling
JOBS_REDIS_URL=redis://localhost:6379?maxRetriesPerRequest=3&enableReadyCheck=true
```

**Redis Configuration (`redis.conf`):**
```ini
# Memory optimization
maxmemory 4gb
maxmemory-policy allkeys-lru

# Persistence for job durability
save 900 1
save 300 10
save 60 10000
appendonly yes
appendfsync everysec

# Performance tuning
tcp-backlog 511
timeout 0
tcp-keepalive 300
```

**Redis Cluster for High Availability:**
```bash
# Clustered Redis setup
JOBS_REDIS_URL=redis://redis-cluster-1:6379,redis-cluster-2:6379,redis-cluster-3:6379
```

### Performance Benchmarks

Typical throughput on standard hardware:

| Job Type | Concurrency | Throughput | Latency (p95) |
|----------|-------------|------------|---------------|
| send-email | 10 | 1,200 jobs/min | 850ms |
| http-request | 20 | 3,500 jobs/min | 420ms |
| database-backup | 2 | 8 jobs/min | 45s |
| file-cleanup | 15 | 2,800 jobs/min | 320ms |
| custom (light) | 25 | 5,000 jobs/min | 180ms |

**Optimization Tips:**
- **CPU-bound jobs**: Match concurrency to CPU cores (e.g., 4 cores = 4-8 concurrent jobs)
- **I/O-bound jobs**: Higher concurrency (e.g., 2x-4x CPU cores)
- **Mixed workloads**: Use separate queues with different concurrency settings

### Job Payload Size

Keep payloads small for better performance:

```typescript
// BAD: Large payload stored in Redis
await jobs.create({
  type: 'process-data',
  payload: {
    largeDataset: [...10000Items], // 5MB payload
  },
});

// GOOD: Reference to external storage
await jobs.create({
  type: 'process-data',
  payload: {
    datasetUrl: 's3://bucket/dataset-12345.json', // Small reference
    recordCount: 10000,
  },
});
```

**Payload Size Guidelines:**
- **Ideal**: < 1 KB (metadata, IDs, references)
- **Acceptable**: 1 KB - 100 KB (small data structures)
- **Avoid**: > 100 KB (use external storage like S3, PostgreSQL)

### Cleanup and Maintenance

Automatic cleanup prevents Redis memory bloat:

```bash
# Aggressive cleanup for high-volume systems
JOBS_CLEAN_COMPLETED_AFTER=3600000    # 1 hour
JOBS_CLEAN_FAILED_AFTER=86400000       # 24 hours

# Conservative cleanup for debugging
JOBS_CLEAN_COMPLETED_AFTER=604800000   # 7 days
JOBS_CLEAN_FAILED_AFTER=2592000000     # 30 days
```

**Manual Cleanup:**
```sql
-- Clean old completed jobs
DELETE FROM jobs
WHERE status = 'completed'
  AND completed_at < NOW() - INTERVAL '7 days';

-- Clean old failed jobs (keep recent for debugging)
DELETE FROM job_failures
WHERE failed_at < NOW() - INTERVAL '30 days';
```

---

## Security Notes

### Dashboard Authentication

The BullBoard dashboard has NO built-in authentication. Implement security layers:

**Option 1: Reverse Proxy with Authentication (Recommended)**

```nginx
# nginx.conf
server {
  listen 443 ssl;
  server_name jobs.example.com;

  # Basic authentication
  auth_basic "BullBoard Access";
  auth_basic_user_file /etc/nginx/.htpasswd;

  location / {
    proxy_pass http://localhost:3105;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
  }
}
```

Create password file:
```bash
htpasswd -c /etc/nginx/.htpasswd admin
```

**Option 2: VPN/SSH Tunnel**

```bash
# SSH tunnel for remote access
ssh -L 3105:localhost:3105 user@production-server

# Access locally
open http://localhost:3105/dashboard
```

**Option 3: Firewall Rules**

```bash
# Allow only specific IPs
sudo ufw allow from 203.0.113.0/24 to any port 3105

# Block all other IPs
sudo ufw deny 3105
```

**Option 4: Custom Authentication Middleware**

```typescript
// In server.ts
import basicAuth from '@fastify/basic-auth';

const validateUser = async (username: string, password: string) => {
  if (username === process.env.DASHBOARD_USER &&
      password === process.env.DASHBOARD_PASS) {
    return;
  }
  throw new Error('Invalid credentials');
};

server.register(basicAuth, { validate: validateUser });

// Protect dashboard route
server.after(() => {
  server.addHook('onRequest', server.basicAuth);
});
```

### Job Data Security

**Encrypting Sensitive Payloads:**

```typescript
import crypto from 'crypto';

// Encryption helper
function encryptPayload(data: any, key: string): string {
  const iv = crypto.randomBytes(16);
  const cipher = crypto.createCipheriv('aes-256-gcm', Buffer.from(key, 'hex'), iv);

  let encrypted = cipher.update(JSON.stringify(data), 'utf8', 'hex');
  encrypted += cipher.final('hex');

  const authTag = cipher.getAuthTag();

  return JSON.stringify({
    iv: iv.toString('hex'),
    authTag: authTag.toString('hex'),
    data: encrypted,
  });
}

// Create job with encrypted payload
await jobs.create({
  type: 'send-email',
  payload: {
    encrypted: true,
    data: encryptPayload({
      to: 'user@example.com',
      apiKey: 'sk_live_...',
    }, process.env.ENCRYPTION_KEY!),
  },
});
```

**Decryption in Worker:**

```typescript
function decryptPayload(encrypted: string, key: string): any {
  const { iv, authTag, data } = JSON.parse(encrypted);

  const decipher = crypto.createDecipheriv(
    'aes-256-gcm',
    Buffer.from(key, 'hex'),
    Buffer.from(iv, 'hex')
  );

  decipher.setAuthTag(Buffer.from(authTag, 'hex'));

  let decrypted = decipher.update(data, 'hex', 'utf8');
  decrypted += decipher.final('utf8');

  return JSON.parse(decrypted);
}
```

### Redis Security

**Authentication:**
```bash
# redis.conf
requirepass your-strong-redis-password

# Connection string
JOBS_REDIS_URL=redis://:your-strong-redis-password@localhost:6379
```

**Network Security:**
```bash
# redis.conf - Bind to specific interface
bind 127.0.0.1

# Or specific private IP
bind 10.0.1.50

# Disable dangerous commands
rename-command FLUSHDB ""
rename-command FLUSHALL ""
rename-command CONFIG "CONFIG_a8d2f9b3"
```

**TLS Encryption:**
```bash
# Redis with TLS
JOBS_REDIS_URL=rediss://username:password@redis-host:6380?tls=true

# With custom CA certificate
JOBS_REDIS_URL=rediss://redis-host:6380?tls=true&ca=/path/to/ca.crt
```

### Database Security

**Row-Level Security (RLS) for Multi-Tenant:**

```sql
-- Enable RLS on jobs table
ALTER TABLE jobs ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only see their own jobs
CREATE POLICY user_jobs_policy ON jobs
  FOR ALL
  USING (payload->>'tenant_id' = current_setting('app.current_tenant_id'));

-- Set tenant ID in session
SET app.current_tenant_id = 'tenant_123';
```

**Audit Logging:**

```sql
-- Create audit table
CREATE TABLE job_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID REFERENCES jobs(id),
    action VARCHAR(50) NOT NULL,  -- created, started, completed, failed, retried
    user_id VARCHAR(255),
    ip_address INET,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Audit trigger function
CREATE OR REPLACE FUNCTION log_job_changes()
RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO job_audit_log (job_id, action)
  VALUES (NEW.id, TG_OP);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Attach trigger
CREATE TRIGGER job_audit_trigger
  AFTER INSERT OR UPDATE ON jobs
  FOR EACH ROW EXECUTE FUNCTION log_job_changes();
```

---

## Advanced Code Examples

### Cron Scheduling

**Complex Cron Patterns:**

```typescript
// Every weekday at 9 AM
await scheduleJob({
  name: 'weekday-report',
  type: 'generate-report',
  cron: '0 9 * * 1-5',
  payload: { reportType: 'daily' },
});

// Every 15 minutes during business hours
await scheduleJob({
  name: 'frequent-sync',
  type: 'sync-data',
  cron: '*/15 9-17 * * *',
  payload: { source: 'crm' },
});

// First day of every month at midnight
await scheduleJob({
  name: 'monthly-billing',
  type: 'process-billing',
  cron: '0 0 1 * *',
  payload: { cycle: 'monthly' },
});

// Every 6 hours
await scheduleJob({
  name: 'cache-refresh',
  type: 'refresh-cache',
  cron: '0 */6 * * *',
  payload: { cacheType: 'product-catalog' },
});

// Specific date/time (New Year's midnight)
await scheduleJob({
  name: 'new-year-greeting',
  type: 'send-email',
  cron: '0 0 1 1 *',
  payload: { template: 'new-year' },
});
```

**Dynamic Cron Schedules:**

```typescript
// Update schedule based on user preferences
async function updateUserSchedule(userId: string, timezone: string, hour: number) {
  const cronExpression = `0 ${hour} * * *`;

  await db.execute(
    `UPDATE job_schedules
     SET cron_expression = $1,
         payload = jsonb_set(payload, '{timezone}', $2)
     WHERE name = $3`,
    [cronExpression, JSON.stringify(timezone), `user-digest-${userId}`]
  );
}

// Different schedules per environment
const cronExpression = process.env.NODE_ENV === 'production'
  ? '0 2 * * *'      // 2 AM in production
  : '*/5 * * * *';   // Every 5 minutes in dev

await scheduleJob({
  name: 'backup',
  type: 'database-backup',
  cron: cronExpression,
  payload: { env: process.env.NODE_ENV },
});
```

### Job Retry Strategies

**Exponential Backoff:**

```typescript
await jobs.create({
  type: 'http-request',
  payload: { url: 'https://api.example.com/data' },
  options: {
    attempts: 5,
    backoff: {
      type: 'exponential',
      delay: 1000,  // Start with 1s, then 2s, 4s, 8s, 16s
    },
  },
});
```

**Fixed Delay:**

```typescript
await jobs.create({
  type: 'send-email',
  payload: { to: 'user@example.com' },
  options: {
    attempts: 3,
    backoff: {
      type: 'fixed',
      delay: 5000,  // Always wait 5s between retries
    },
  },
});
```

**Custom Retry Logic:**

```typescript
await jobs.create({
  type: 'custom',
  payload: { action: 'api-call' },
  options: {
    attempts: 5,
    backoff: {
      type: 'custom',
      delay: (attemptsMade: number) => {
        // Custom strategy: 1s, 5s, 30s, 2m, 10m
        const delays = [1000, 5000, 30000, 120000, 600000];
        return delays[attemptsMade] || 600000;
      },
    },
  },
});
```

**Conditional Retry (in Worker):**

```typescript
// In worker processor
async function processHttpRequest(job: Job) {
  try {
    const response = await fetch(job.data.url);

    // Don't retry client errors (4xx)
    if (response.status >= 400 && response.status < 500) {
      throw new Error(`Client error ${response.status}: Do not retry`);
    }

    // Retry server errors (5xx)
    if (response.status >= 500) {
      throw new Error(`Server error ${response.status}: Retry`);
    }

    return await response.json();
  } catch (error) {
    // Check if error is retryable
    if (error.message.includes('Do not retry')) {
      job.moveToFailed({ message: error.message }, true);  // Skip retries
      return;
    }
    throw error;  // Normal retry flow
  }
}
```

### Queue Patterns

**Priority-Based Job Distribution:**

```typescript
// Critical: Process immediately
await jobs.create({
  type: 'alert',
  queue: 'high-priority',
  payload: { type: 'security-breach' },
  options: { priority: 1 },  // Highest priority
});

// High: Process soon
await jobs.create({
  type: 'send-email',
  queue: 'default',
  payload: { to: 'vip@example.com' },
  options: { priority: 5 },
});

// Normal: Standard processing
await jobs.create({
  type: 'sync-data',
  queue: 'default',
  payload: { source: 'crm' },
  options: { priority: 10 },
});

// Low: Background batch work
await jobs.create({
  type: 'file-cleanup',
  queue: 'low-priority',
  payload: { olderThan: '30d' },
  options: { priority: 20 },
});
```

**Rate-Limited Queue:**

```typescript
// Process max 10 jobs per minute
const rateLimitedQueue = new Queue('rate-limited', {
  limiter: {
    max: 10,          // 10 jobs
    duration: 60000,  // per minute
  },
});

// Jobs will be throttled automatically
for (let i = 0; i < 100; i++) {
  await rateLimitedQueue.add('api-call', { index: i });
}
```

**Delayed Job Execution:**

```typescript
// Process in 1 hour
await jobs.create({
  type: 'send-email',
  payload: { template: 'welcome' },
  options: {
    delay: 3600000,  // 1 hour in ms
  },
});

// Process at specific time
const targetTime = new Date('2026-02-01T10:00:00Z');
const delayMs = targetTime.getTime() - Date.now();

await jobs.create({
  type: 'send-reminder',
  payload: { eventId: '12345' },
  options: { delay: delayMs },
});
```

### Job Chaining

**Sequential Job Chain:**

```typescript
// Job 1: Fetch data
const fetchJob = await jobs.create({
  type: 'http-request',
  payload: { url: 'https://api.example.com/users' },
});

// Job 2: Process data (waits for Job 1)
const processJob = await jobs.create({
  type: 'custom',
  payload: {
    action: 'process-users',
    dependsOn: fetchJob.id,
  },
  options: {
    delay: 5000,  // Check every 5s if parent is done
  },
});

// Job 3: Send notification (waits for Job 2)
await jobs.create({
  type: 'send-email',
  payload: {
    template: 'processing-complete',
    dependsOn: processJob.id,
  },
  options: { delay: 5000 },
});
```

**Worker Implementation for Chaining:**

```typescript
async function processJob(job: Job) {
  const { dependsOn } = job.data;

  if (dependsOn) {
    // Check if parent job completed
    const parentJob = await jobs.get(dependsOn);

    if (!parentJob || parentJob.status !== 'completed') {
      // Re-queue this job to check again later
      throw new Error('Parent job not ready');
    }

    // Use parent job result
    const parentResult = await parentJob.result();
    job.data.parentData = parentResult;
  }

  // Process current job
  return performWork(job.data);
}
```

**Parallel + Merge Pattern:**

```typescript
// Create multiple parallel jobs
const parallelJobs = await Promise.all([
  jobs.create({ type: 'fetch-service-a', payload: { source: 'a' } }),
  jobs.create({ type: 'fetch-service-b', payload: { source: 'b' } }),
  jobs.create({ type: 'fetch-service-c', payload: { source: 'c' } }),
]);

// Create merge job that waits for all
await jobs.create({
  type: 'merge-results',
  payload: {
    dependsOnAll: parallelJobs.map(j => j.id),
  },
});
```

### Dashboard Customization

**Custom Queue View:**

```typescript
// server.ts
import { createBullBoard } from '@bull-board/api';
import { BullMQAdapter } from '@bull-board/api/bullMQAdapter';
import { FastifyAdapter } from '@bull-board/fastify';

const serverAdapter = new FastifyAdapter();

createBullBoard({
  queues: [
    new BullMQAdapter(defaultQueue),
    new BullMQAdapter(highPriorityQueue),
    new BullMQAdapter(lowPriorityQueue),
  ],
  serverAdapter,
  options: {
    uiConfig: {
      boardTitle: 'Jobs Dashboard - Production',
      boardLogo: {
        path: '/logo.png',
        width: 100,
        height: 50,
      },
      miscLinks: [
        { text: 'Grafana', url: 'https://grafana.example.com' },
        { text: 'Docs', url: 'https://docs.example.com' },
      ],
      favIcon: {
        default: '/favicon.ico',
        alternative: '/favicon-32x32.png',
      },
    },
  },
});

serverAdapter.setBasePath('/dashboard');
server.register(serverAdapter.registerPlugin(), {
  prefix: '/dashboard',
  basePath: '/',
});
```

**Custom Job Data Formatting:**

```typescript
// Add custom formatter for job display
class CustomBullMQAdapter extends BullMQAdapter {
  formatJob(job: Job): any {
    const formatted = super.formatJob(job);

    // Add custom fields
    return {
      ...formatted,
      displayName: this.getDisplayName(job),
      priority: this.getPriorityLabel(job.opts.priority),
      estimatedDuration: this.estimateDuration(job.name),
    };
  }

  getDisplayName(job: Job): string {
    const { type } = job.data;
    const prefixes = {
      'send-email': '📧',
      'http-request': '🌐',
      'database-backup': '💾',
      'file-cleanup': '🗑️',
    };
    return `${prefixes[type] || '⚙️'} ${type}`;
  }

  getPriorityLabel(priority?: number): string {
    if (!priority) return 'normal';
    if (priority <= 2) return 'critical';
    if (priority <= 5) return 'high';
    if (priority <= 15) return 'normal';
    return 'low';
  }

  estimateDuration(jobType: string): number {
    const estimates = {
      'send-email': 2000,
      'http-request': 1000,
      'database-backup': 60000,
      'file-cleanup': 5000,
    };
    return estimates[jobType] || 3000;
  }
}
```

---

## Monitoring & Alerting

### Queue Health Metrics

**Key Metrics to Track:**

```typescript
// Collect queue statistics
async function getQueueHealth() {
  const queues = ['default', 'high-priority', 'low-priority'];
  const health = {};

  for (const queueName of queues) {
    const queue = getQueue(queueName);

    const [waiting, active, completed, failed, delayed] = await Promise.all([
      queue.getWaitingCount(),
      queue.getActiveCount(),
      queue.getCompletedCount(),
      queue.getFailedCount(),
      queue.getDelayedCount(),
    ]);

    health[queueName] = {
      waiting,
      active,
      completed,
      failed,
      delayed,
      // Calculate health score
      healthScore: calculateHealthScore({
        waiting,
        active,
        failed,
        capacity: 1000,
      }),
    };
  }

  return health;
}

function calculateHealthScore(metrics: {
  waiting: number;
  active: number;
  failed: number;
  capacity: number;
}): number {
  const { waiting, active, failed, capacity } = metrics;

  // 100 = healthy, 0 = critical
  let score = 100;

  // Penalize high waiting queue
  if (waiting > capacity * 0.8) score -= 30;
  else if (waiting > capacity * 0.5) score -= 15;

  // Penalize stalled processing
  if (active === 0 && waiting > 0) score -= 40;

  // Penalize high failure rate
  const total = waiting + active + failed;
  if (total > 0) {
    const failureRate = failed / total;
    if (failureRate > 0.1) score -= 20;
    if (failureRate > 0.25) score -= 30;
  }

  return Math.max(0, score);
}
```

**Prometheus Metrics Export:**

```typescript
import promClient from 'prom-client';

// Create metrics
const jobsProcessed = new promClient.Counter({
  name: 'jobs_processed_total',
  help: 'Total number of jobs processed',
  labelNames: ['queue', 'type', 'status'],
});

const jobDuration = new promClient.Histogram({
  name: 'job_duration_seconds',
  help: 'Job processing duration',
  labelNames: ['queue', 'type'],
  buckets: [0.1, 0.5, 1, 2, 5, 10, 30, 60],
});

const queueSize = new promClient.Gauge({
  name: 'queue_size',
  help: 'Number of jobs in queue',
  labelNames: ['queue', 'status'],
});

// Update metrics in worker
worker.on('completed', (job, result) => {
  jobsProcessed.inc({
    queue: job.queueName,
    type: job.data.type,
    status: 'completed'
  });

  const duration = (Date.now() - job.timestamp) / 1000;
  jobDuration.observe({ queue: job.queueName, type: job.data.type }, duration);
});

worker.on('failed', (job, error) => {
  jobsProcessed.inc({
    queue: job.queueName,
    type: job.data.type,
    status: 'failed'
  });
});

// Expose metrics endpoint
server.get('/metrics', async (req, reply) => {
  reply.type('text/plain');
  return promClient.register.metrics();
});
```

### Job Failure Rate Tracking

**Real-time Failure Monitoring:**

```sql
-- Create failure rate view
CREATE VIEW job_failure_rates AS
SELECT
  type,
  queue,
  COUNT(*) AS total_jobs,
  COUNT(*) FILTER (WHERE status = 'failed') AS failed_jobs,
  ROUND(
    COUNT(*) FILTER (WHERE status = 'failed')::NUMERIC /
    NULLIF(COUNT(*), 0) * 100,
    2
  ) AS failure_rate_percent,
  AVG(attempts) AS avg_attempts
FROM jobs
WHERE created_at > NOW() - INTERVAL '1 hour'
GROUP BY type, queue
HAVING COUNT(*) > 10  -- Only show types with significant volume
ORDER BY failure_rate_percent DESC;
```

**Failure Rate Alerts:**

```typescript
async function checkFailureRates() {
  const rates = await db.query(`
    SELECT type, queue, failure_rate_percent
    FROM job_failure_rates
    WHERE failure_rate_percent > 10  -- Alert threshold: 10%
  `);

  for (const row of rates.rows) {
    await sendAlert({
      severity: row.failure_rate_percent > 25 ? 'critical' : 'warning',
      message: `High failure rate for ${row.type} in ${row.queue}: ${row.failure_rate_percent}%`,
      queue: row.queue,
      jobType: row.type,
      failureRate: row.failure_rate_percent,
    });
  }
}

// Run every 5 minutes
setInterval(checkFailureRates, 300000);
```

### Processing Time Analysis

**P95/P99 Latency Tracking:**

```sql
-- Processing time percentiles
CREATE VIEW job_processing_percentiles AS
SELECT
  type,
  queue,
  COUNT(*) AS total_jobs,
  PERCENTILE_CONT(0.50) WITHIN GROUP (ORDER BY r.duration_ms) AS p50_ms,
  PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY r.duration_ms) AS p95_ms,
  PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY r.duration_ms) AS p99_ms,
  AVG(r.duration_ms) AS avg_ms,
  MAX(r.duration_ms) AS max_ms
FROM jobs j
JOIN job_results r ON j.id = r.job_id
WHERE j.created_at > NOW() - INTERVAL '24 hours'
  AND j.status = 'completed'
GROUP BY type, queue
ORDER BY p95_ms DESC;
```

**Slow Job Detection:**

```typescript
// Alert on slow jobs
async function detectSlowJobs() {
  const slowJobs = await db.query(`
    SELECT j.id, j.type, j.queue, r.duration_ms,
           p.p95_ms
    FROM jobs j
    JOIN job_results r ON j.id = r.job_id
    JOIN job_processing_percentiles p ON j.type = p.type AND j.queue = p.queue
    WHERE r.duration_ms > p.p95_ms * 2  -- 2x slower than p95
      AND r.completed_at > NOW() - INTERVAL '1 hour'
  `);

  for (const job of slowJobs.rows) {
    logger.warn('Slow job detected', {
      jobId: job.id,
      type: job.type,
      duration: job.duration_ms,
      expectedP95: job.p95_ms,
    });
  }
}
```

### Alerting Integrations

**Slack Alerts:**

```typescript
async function sendSlackAlert(message: string, severity: 'info' | 'warning' | 'critical') {
  const colors = {
    info: '#36a64f',
    warning: '#ff9800',
    critical: '#ff0000',
  };

  await fetch(process.env.SLACK_WEBHOOK_URL!, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      attachments: [{
        color: colors[severity],
        title: `Jobs Plugin Alert - ${severity.toUpperCase()}`,
        text: message,
        footer: 'Jobs Plugin Monitoring',
        ts: Math.floor(Date.now() / 1000),
      }],
    }),
  });
}

// Monitor queue depth
setInterval(async () => {
  const health = await getQueueHealth();

  for (const [queue, metrics] of Object.entries(health)) {
    if (metrics.waiting > 1000) {
      await sendSlackAlert(
        `Queue "${queue}" has ${metrics.waiting} waiting jobs (threshold: 1000)`,
        'warning'
      );
    }

    if (metrics.healthScore < 50) {
      await sendSlackAlert(
        `Queue "${queue}" health score is ${metrics.healthScore}/100`,
        'critical'
      );
    }
  }
}, 60000);  // Every minute
```

**PagerDuty Integration:**

```typescript
async function sendPagerDutyAlert(event: {
  severity: 'info' | 'warning' | 'error' | 'critical';
  summary: string;
  source: string;
  component?: string;
  details?: any;
}) {
  await fetch('https://events.pagerduty.com/v2/enqueue', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Token token=${process.env.PAGERDUTY_TOKEN}`,
    },
    body: JSON.stringify({
      routing_key: process.env.PAGERDUTY_ROUTING_KEY,
      event_action: 'trigger',
      payload: {
        summary: event.summary,
        severity: event.severity,
        source: event.source,
        component: event.component,
        custom_details: event.details,
      },
    }),
  });
}

// Critical alert: Worker stopped
worker.on('error', async (error) => {
  await sendPagerDutyAlert({
    severity: 'critical',
    summary: 'Job worker encountered fatal error',
    source: 'jobs-plugin-worker',
    component: 'worker-process',
    details: { error: error.message, stack: error.stack },
  });
});
```

**Email Alerts:**

```typescript
async function sendEmailAlert(subject: string, body: string, recipients: string[]) {
  await jobs.create({
    type: 'send-email',
    queue: 'high-priority',
    payload: {
      to: recipients,
      subject: `[ALERT] ${subject}`,
      body: `
        <h2>Jobs Plugin Alert</h2>
        <p>${body}</p>
        <hr>
        <p><small>Timestamp: ${new Date().toISOString()}</small></p>
      `,
      isHtml: true,
    },
    options: { priority: 1 },  // Highest priority
  });
}

// Alert on repeated failures
async function checkRepeatedFailures() {
  const repeatedFailures = await db.query(`
    SELECT j.id, j.type, j.queue, COUNT(f.id) AS failure_count
    FROM jobs j
    JOIN job_failures f ON j.id = f.job_id
    WHERE j.status = 'failed'
      AND j.attempts >= j.max_attempts
    GROUP BY j.id, j.type, j.queue
    HAVING COUNT(f.id) >= 3
  `);

  if (repeatedFailures.rows.length > 0) {
    const body = repeatedFailures.rows
      .map(r => `- ${r.type} (${r.queue}): ${r.failure_count} failures`)
      .join('\n');

    await sendEmailAlert(
      'Jobs failing repeatedly',
      `The following jobs have failed all retry attempts:\n\n${body}`,
      ['ops-team@example.com']
    );
  }
}
```

### Grafana Dashboard

**Sample dashboard JSON:**

```json
{
  "dashboard": {
    "title": "Jobs Plugin Monitoring",
    "panels": [
      {
        "title": "Jobs Processed (per minute)",
        "targets": [{
          "expr": "rate(jobs_processed_total[1m])"
        }]
      },
      {
        "title": "Queue Depth",
        "targets": [{
          "expr": "queue_size{status='waiting'}"
        }]
      },
      {
        "title": "P95 Processing Time",
        "targets": [{
          "expr": "histogram_quantile(0.95, rate(job_duration_seconds_bucket[5m]))"
        }]
      },
      {
        "title": "Failure Rate (%)",
        "targets": [{
          "expr": "rate(jobs_processed_total{status='failed'}[5m]) / rate(jobs_processed_total[5m]) * 100"
        }]
      }
    ]
  }
}
```

---

## Use Cases

### 1. Transactional Email Delivery

Send emails asynchronously without blocking API responses:

```typescript
// API endpoint
app.post('/signup', async (req, res) => {
  const user = await createUser(req.body);

  // Queue welcome email (non-blocking)
  await jobs.create({
    type: 'send-email',
    payload: {
      to: user.email,
      subject: 'Welcome to Our Platform!',
      template: 'welcome',
      data: { name: user.name, userId: user.id },
    },
    options: {
      priority: 5,  // High priority
      attempts: 3,
    },
  });

  res.json({ success: true, userId: user.id });
});
```

### 2. Webhook Processing

Process incoming webhooks asynchronously with retry logic:

```typescript
// Webhook endpoint
app.post('/webhooks/stripe', async (req, res) => {
  const event = req.body;

  // Respond immediately (Stripe requires < 5s response)
  res.json({ received: true });

  // Queue webhook processing
  await jobs.create({
    type: 'process-webhook',
    payload: {
      source: 'stripe',
      eventType: event.type,
      eventData: event.data,
      eventId: event.id,
    },
    options: {
      attempts: 5,  // Retry up to 5 times
      backoff: {
        type: 'exponential',
        delay: 2000,
      },
    },
  });
});
```

### 3. Batch Data Processing

Process large datasets in chunks:

```typescript
// Split large dataset into batches
async function processBulkImport(records: any[]) {
  const batchSize = 100;
  const batches = [];

  for (let i = 0; i < records.length; i += batchSize) {
    const batch = records.slice(i, i + batchSize);

    batches.push(
      jobs.create({
        type: 'import-batch',
        payload: {
          batchNumber: Math.floor(i / batchSize) + 1,
          totalBatches: Math.ceil(records.length / batchSize),
          records: batch,
        },
        options: {
          priority: 10,  // Normal priority
        },
      })
    );
  }

  await Promise.all(batches);
  logger.info(`Queued ${batches.length} batches for processing`);
}
```

### 4. Scheduled Reports

Generate and email daily/weekly reports:

```typescript
// Daily sales report at 8 AM
await scheduleJob({
  name: 'daily-sales-report',
  type: 'generate-report',
  cron: '0 8 * * *',
  payload: {
    reportType: 'sales-summary',
    period: 'daily',
    recipients: ['management@example.com'],
  },
  description: 'Daily sales summary report',
});

// Weekly analytics report (Monday 9 AM)
await scheduleJob({
  name: 'weekly-analytics',
  type: 'generate-report',
  cron: '0 9 * * 1',
  payload: {
    reportType: 'analytics',
    period: 'weekly',
    recipients: ['analytics-team@example.com'],
  },
});
```

### 5. Image/Video Processing

Process media uploads asynchronously:

```typescript
// Upload endpoint
app.post('/upload/image', upload.single('image'), async (req, res) => {
  const file = req.file;

  // Queue image processing
  await jobs.create({
    type: 'process-image',
    payload: {
      filePath: file.path,
      userId: req.user.id,
      operations: [
        { type: 'resize', width: 800, height: 600 },
        { type: 'thumbnail', width: 200, height: 200 },
        { type: 'compress', quality: 80 },
        { type: 'upload-s3', bucket: 'user-images' },
      ],
    },
    options: {
      timeout: 300000,  // 5 minutes timeout
    },
  });

  res.json({
    success: true,
    message: 'Upload queued for processing',
    fileId: file.filename,
  });
});
```

### 6. API Rate Limit Management

Respect third-party API rate limits:

```typescript
// Queue API calls to respect rate limits
const apiQueue = new Queue('third-party-api', {
  limiter: {
    max: 100,         // 100 requests
    duration: 60000,  // per minute
  },
});

// Make API calls through queue
async function fetchUserData(userId: string) {
  const job = await apiQueue.add('fetch-user', {
    endpoint: `/users/${userId}`,
    method: 'GET',
  });

  const result = await job.waitUntilFinished();
  return result.data;
}
```

### 7. Database Maintenance

Automate database cleanup and optimization:

```typescript
// Nightly database cleanup (2 AM)
await scheduleJob({
  name: 'nightly-cleanup',
  type: 'database-maintenance',
  cron: '0 2 * * *',
  payload: {
    tasks: [
      { action: 'vacuum', tables: ['jobs', 'job_results'] },
      { action: 'delete-old-records', table: 'job_failures', olderThan: '30d' },
      { action: 'reindex', tables: ['jobs'] },
      { action: 'analyze' },
    ],
  },
});

// Weekly database backup (Sunday 3 AM)
await scheduleJob({
  name: 'weekly-backup',
  type: 'database-backup',
  cron: '0 3 * * 0',
  payload: {
    database: process.env.DATABASE_URL,
    destination: 's3://backups/weekly',
    compress: true,
    encrypt: true,
  },
});
```

### 8. Notification Campaigns

Send targeted notifications to user segments:

```typescript
// Send campaign to user segment
async function sendCampaign(segmentId: string, template: string) {
  // Fetch user segment
  const users = await db.query(
    'SELECT id, email, name FROM users WHERE segment_id = $1',
    [segmentId]
  );

  // Queue individual emails
  for (const user of users.rows) {
    await jobs.create({
      type: 'send-email',
      queue: 'low-priority',  // Spread out over time
      payload: {
        to: user.email,
        template,
        data: { name: user.name, userId: user.id },
      },
      options: {
        delay: Math.random() * 3600000,  // Random delay up to 1 hour
        priority: 15,  // Low priority
      },
    });
  }

  logger.info(`Queued ${users.rows.length} campaign emails`);
}
```

### 9. Cache Warming

Pre-populate caches before traffic spikes:

```typescript
// Warm cache before Black Friday (Nov 25, midnight)
await scheduleJob({
  name: 'black-friday-cache-warm',
  type: 'warm-cache',
  cron: '0 0 25 11 *',
  payload: {
    caches: [
      { name: 'product-catalog', ttl: 86400 },
      { name: 'homepage-data', ttl: 3600 },
      { name: 'featured-products', ttl: 7200 },
    ],
  },
});

// Hourly cache refresh
await scheduleJob({
  name: 'hourly-cache-refresh',
  type: 'warm-cache',
  cron: '0 * * * *',
  payload: {
    caches: ['trending-products', 'popular-categories'],
  },
});
```

### 10. Data Synchronization

Sync data between systems:

```typescript
// Sync CRM data every 15 minutes
await scheduleJob({
  name: 'crm-sync',
  type: 'sync-data',
  cron: '*/15 * * * *',
  payload: {
    source: 'salesforce',
    destination: 'local-db',
    entities: ['accounts', 'contacts', 'opportunities'],
    incrementalSync: true,
  },
});

// Full sync daily at 1 AM
await scheduleJob({
  name: 'crm-full-sync',
  type: 'sync-data',
  cron: '0 1 * * *',
  payload: {
    source: 'salesforce',
    destination: 'local-db',
    entities: ['accounts', 'contacts', 'opportunities'],
    incrementalSync: false,
  },
});
```

### 11. File Archival

Archive old files to cold storage:

```typescript
// Monthly archival (1st of month, 4 AM)
await scheduleJob({
  name: 'monthly-archival',
  type: 'archive-files',
  cron: '0 4 1 * *',
  payload: {
    source: 's3://active-storage',
    destination: 's3://glacier-archive',
    criteria: {
      olderThan: '90d',
      fileTypes: ['*.log', '*.csv', '*.backup'],
    },
    compress: true,
  },
});
```

### 12. Real-time Analytics Aggregation

Aggregate analytics data in the background:

```typescript
// Process analytics events
app.post('/analytics/track', async (req, res) => {
  const event = req.body;

  // Respond immediately
  res.json({ tracked: true });

  // Queue aggregation
  await jobs.create({
    type: 'aggregate-analytics',
    queue: 'low-priority',
    payload: {
      event: event.type,
      userId: event.userId,
      properties: event.properties,
      timestamp: event.timestamp,
    },
    options: {
      delay: 60000,  // Batch events for 1 minute
    },
  });
});
```

### 13. Fraud Detection

Run fraud detection models asynchronously:

```typescript
// Check transaction for fraud
app.post('/transactions', async (req, res) => {
  const transaction = await createTransaction(req.body);

  // Queue fraud check
  await jobs.create({
    type: 'fraud-detection',
    queue: 'high-priority',
    payload: {
      transactionId: transaction.id,
      amount: transaction.amount,
      userId: transaction.userId,
      ipAddress: req.ip,
    },
    options: {
      priority: 3,  // High priority
      timeout: 10000,  // 10 second timeout
    },
  });

  res.json({ transactionId: transaction.id, status: 'pending' });
});
```

### 14. Content Moderation

Queue user-generated content for moderation:

```typescript
// Submit content for review
app.post('/content', async (req, res) => {
  const content = await saveContent(req.body);

  // Queue moderation
  await jobs.create({
    type: 'moderate-content',
    payload: {
      contentId: content.id,
      contentType: content.type,
      text: content.text,
      userId: content.userId,
      checks: ['profanity', 'spam', 'ai-detection'],
    },
    options: {
      priority: 7,
    },
  });

  res.json({ contentId: content.id, status: 'pending_review' });
});
```

### 15. Invoice Generation

Generate invoices at month-end:

```typescript
// Generate invoices for all active subscriptions
await scheduleJob({
  name: 'monthly-invoices',
  type: 'generate-invoices',
  cron: '0 0 1 * *',  // 1st of every month at midnight
  payload: {
    subscriptionStatus: 'active',
    sendEmail: true,
    emailTemplate: 'invoice',
  },
  description: 'Generate monthly invoices for active subscriptions',
});
```

---

## Support

- **GitHub Issues:** [nself-plugins/issues](https://github.com/acamarata/nself-plugins/issues)

---

*Last Updated: January 2026*
*Plugin Version: 1.0.0*
