# Jobs Plugin for nself

BullMQ-based background job queue with priorities, scheduling, retry logic, and BullBoard dashboard.

## Features

- **Multiple Queues**: `default`, `high-priority`, `low-priority`
- **Job Priorities**: `critical`, `high`, `normal`, `low`
- **Retry Logic**: Configurable exponential backoff
- **Cron Scheduling**: Recurring jobs with cron expressions
- **BullBoard Dashboard**: Web UI for monitoring jobs
- **Full Persistence**: All jobs tracked in PostgreSQL
- **Telemetry**: Job statistics and performance metrics

## Job Processors

### ✅ Fully Implemented
- `http-request` - HTTP requests with retry logic (uses native fetch)
- `file-cleanup` - Clean old completed/failed jobs from database
- `send-email` - Email sending via SMTP (uses Nodemailer)

### 🔄 Stubs (Require External Integration)
- `database-backup` - PostgreSQL backups (requires pg_dump binary and credentials - use backup plugin instead)
- `custom` - Hasura Actions integration (requires GraphQL endpoint configuration)

See "Job Types" section below for integration instructions.

## Current Features

### ✅ Job Queue Infrastructure
- BullMQ-based job queue with Redis
- Multiple priority queues (default, high-priority, low-priority)
- Configurable retry logic with exponential backoff
- Cron-based scheduled/recurring jobs
- BullBoard dashboard for monitoring
- Full persistence in PostgreSQL

### ✅ Implemented Job Processors
- **HTTP Request** - Make HTTP/REST API calls with retry logic
- **File Cleanup** - Clean old completed/failed jobs from database
- **Send Email** - Send emails via SMTP server (Nodemailer)

### ✅ Database Schema
- All tables created and ready (jobs, job_results, job_failures, job_schedules)
- Database views for monitoring (queue_stats, job_type_stats, recent_failures)
- Cleanup functions for maintenance

## Planned Job Processors

The following job processors have placeholder implementations but require external service integration:

### ✅ Email Sending (Implemented)
**Status:** Fully implemented using Nodemailer with SMTP

Emails are sent via configured SMTP server. Supports HTML content, attachments, CC/BCC.

**Required Configuration:**
```bash
SMTP_HOST=smtp.gmail.com           # SMTP server hostname
SMTP_PORT=587                       # SMTP port (587 for TLS, 465 for SSL)
SMTP_SECURE=false                   # true for port 465, false for other ports
SMTP_USER=your-email@gmail.com      # SMTP username
SMTP_PASSWORD=your-app-password     # SMTP password or app password
SMTP_FROM=noreply@yourdomain.com    # Default from address
```

**Endpoints:** POST /api/jobs with type `send-email`

### 🔄 Database Backup (Stub)
**Status:** Requires `pg_dump` binary and PostgreSQL credentials

Jobs are queued and tracked, but backups are not created. Integration point is in `ts/src/processors.ts` (lines 101-130).

**Note:** For production database backups, use the dedicated **backup plugin** instead.

### 🔄 Hasura Actions (Stub)
**Status:** Requires Hasura GraphQL endpoint configuration

Custom jobs can be queued, but Hasura actions are not invoked. Integration point is in `ts/src/processors.ts` (lines 173-192).

**Requires:**
- `HASURA_GRAPHQL_ENDPOINT` environment variable
- `HASURA_ADMIN_SECRET` environment variable

## Installation

```bash
# 1. Install the plugin
nself plugin install jobs

# 2. Configure environment
cp .env.example .env
# Edit .env with your Redis URL

# 3. Verify installation
nself plugin jobs init
```

## Configuration

### Required Environment Variables

```bash
JOBS_REDIS_URL=redis://localhost:6379
```

### Optional Environment Variables

```bash
# Dashboard
JOBS_DASHBOARD_ENABLED=true          # Enable BullBoard dashboard
JOBS_DASHBOARD_PORT=3105             # Dashboard port
JOBS_DASHBOARD_PATH=/dashboard       # Dashboard path

# Worker
JOBS_DEFAULT_CONCURRENCY=5           # Jobs processed simultaneously
JOBS_RETRY_ATTEMPTS=3                # Max retry attempts
JOBS_RETRY_DELAY=5000                # Initial retry delay (ms)
JOBS_JOB_TIMEOUT=60000               # Job timeout (ms)

# Cleanup
JOBS_CLEAN_COMPLETED_AFTER=86400000  # 24 hours in ms
JOBS_CLEAN_FAILED_AFTER=604800000    # 7 days in ms
```

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `JOBS_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `JOBS_PLUGIN_PORT` | `PORT` or `JOBS_DASHBOARD_PORT` | Dashboard port | `3105` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `JOBS_REDIS_URL` | `JOBS_REDIS_URL` | Redis connection URL | `redis://localhost:6379` |
| `JOBS_MAX_CONCURRENT` | `JOBS_DEFAULT_CONCURRENCY` | Max concurrent jobs | `5` |
| `JOBS_DEFAULT_ATTEMPTS` | `JOBS_RETRY_ATTEMPTS` | Default retry attempts | `3` |
| `JOBS_DEFAULT_BACKOFF_DELAY` | `JOBS_RETRY_DELAY` | Retry backoff delay (ms) | `5000` |

### Configuration Helper Script

```bash
#!/bin/bash
# generate-jobs-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/jobs/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
JOBS_REDIS_URL=$JOBS_REDIS_URL

# Dashboard
JOBS_DASHBOARD_ENABLED=true
JOBS_DASHBOARD_PORT=$JOBS_PLUGIN_PORT

# Worker
JOBS_DEFAULT_CONCURRENCY=${JOBS_MAX_CONCURRENT:-5}
JOBS_RETRY_ATTEMPTS=${JOBS_DEFAULT_ATTEMPTS:-3}
JOBS_RETRY_DELAY=${JOBS_DEFAULT_BACKOFF_DELAY:-5000}
JOBS_JOB_TIMEOUT=60000

# Cleanup
JOBS_CLEAN_COMPLETED_AFTER=86400000
JOBS_CLEAN_FAILED_AFTER=604800000

# Logging
LOG_LEVEL=info
EOF

echo "Created $PLUGIN_ENV"
```

See [CONFIGURATION.md](../../CONFIGURATION.md) for detailed mapping patterns and troubleshooting.

## Quick Start

### 1. Start the Dashboard

```bash
nself plugin jobs server
```

Visit: `http://localhost:3105/dashboard`

### 2. Start Workers

```bash
# Start worker for default queue
nself plugin jobs worker

# Start worker for specific queue
nself plugin jobs worker high-priority

# Start with custom concurrency
JOBS_DEFAULT_CONCURRENCY=10 nself plugin jobs worker
```

### 3. Add Jobs Programmatically

```typescript
import { Queue } from 'bullmq';
import IORedis from 'ioredis';

const connection = new IORedis('redis://localhost:6379');
const queue = new Queue('default', { connection });

// Send email job
await queue.add('send-email', {
  to: 'user@example.com',
  subject: 'Welcome!',
  body: 'Thanks for signing up',
}, {
  priority: 5,  // High priority
  attempts: 3,
  backoff: {
    type: 'exponential',
    delay: 5000,
  },
});

// HTTP request job
await queue.add('http-request', {
  url: 'https://api.example.com/webhook',
  method: 'POST',
  body: { event: 'user.created', userId: '123' },
}, {
  priority: 0,  // Normal priority
  attempts: 5,
});

// Delayed job (schedule for future)
await queue.add('database-backup', {
  database: 'production',
  destination: '/backups',
  compression: true,
}, {
  delay: 60000,  // Run in 1 minute
});
```

### 4. Create Scheduled Jobs

```bash
# Create a daily backup job
nself plugin jobs schedule create \
  --name daily-backup \
  --type database-backup \
  --cron "0 2 * * *" \
  --payload '{"database": "production", "destination": "/backups"}' \
  --desc "Daily production database backup"

# List all schedules
nself plugin jobs schedule list

# Show schedule details
nself plugin jobs schedule show daily-backup

# Enable/disable schedule
nself plugin jobs schedule enable daily-backup
nself plugin jobs schedule disable daily-backup
```

## CLI Commands

### Initialize

```bash
nself plugin jobs init
```

Verify Redis connection, database schema, and configuration.

### Dashboard Server

```bash
nself plugin jobs server
```

Start BullBoard dashboard at `http://localhost:3105/dashboard`

### Workers

```bash
# Start default worker
nself plugin jobs worker

# Start worker for specific queue
nself plugin jobs worker high-priority

# Custom concurrency
JOBS_DEFAULT_CONCURRENCY=10 nself plugin jobs worker
```

### Statistics

```bash
# Overall stats
nself plugin jobs stats

# Queue-specific stats
nself plugin jobs stats --queue default

# Last 48 hours
nself plugin jobs stats --time 48

# Performance metrics
nself plugin jobs stats --performance

# Watch mode (auto-refresh)
nself plugin jobs stats --watch
```

### Retry Failed Jobs

```bash
# Retry up to 10 failed jobs
nself plugin jobs retry

# Retry from specific queue
nself plugin jobs retry --queue default --limit 20

# Retry specific job type
nself plugin jobs retry --type send-email

# Retry specific job by ID
nself plugin jobs retry --id <uuid>

# Show retryable jobs
nself plugin jobs retry --show
```

### Scheduled Jobs

```bash
# List all schedules
nself plugin jobs schedule list

# Show schedule details
nself plugin jobs schedule show <name>

# Create schedule
nself plugin jobs schedule create \
  --name <name> \
  --type <job-type> \
  --cron "<cron-expression>" \
  --payload '<json>' \
  --queue <queue> \
  --desc "<description>"

# Enable/disable
nself plugin jobs schedule enable <name>
nself plugin jobs schedule disable <name>

# Delete
nself plugin jobs schedule delete <name>
```

## Job Types

### 1. Send Email (⚠️ Stub - Requires Integration)

```typescript
type: 'send-email'
payload: {
  to: string | string[],
  from?: string,
  subject: string,
  body: string,
  html?: string,
  attachments?: Array<{
    filename: string,
    content: string | Buffer,
    contentType?: string
  }>,
  cc?: string[],
  bcc?: string[]
}
```

**Status**: Stub implementation that logs the job but does not send emails.

**Integration Required**: Replace stub in `ts/src/processors.ts` (lines 27-49) with your email service:
- **SendGrid**: `npm install @sendgrid/mail`
- **AWS SES**: `npm install @aws-sdk/client-ses`
- **Mailgun**: `npm install mailgun-js`
- **Nodemailer SMTP**: `npm install nodemailer`

See code comments in `processSendEmail()` for integration point.

### 2. HTTP Request (✅ Implemented)

```typescript
type: 'http-request'
payload: {
  url: string,
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH',
  headers?: Record<string, string>,
  body?: unknown,
  timeout?: number,
  retryOn?: number[]  // HTTP status codes to retry
}
```

**Status**: Fully implemented using native `fetch()` API with timeout support and automatic retry logic.

### 3. Database Backup (⚠️ Stub - Requires Integration)

```typescript
type: 'database-backup'
payload: {
  database: string,
  tables?: string[],
  destination: string,
  compression?: boolean,
  encryption?: boolean
}
```

**Status**: Stub implementation that simulates backup but does not execute `pg_dump`.

**Integration Required**: Replace stub in `ts/src/processors.ts` (lines 101-130) with actual backup logic:
- Requires `pg_dump` binary in PATH
- Requires database credentials (host, user, password)
- Use `child_process.spawn()` to execute pg_dump
- For production use, consider using the dedicated **backup plugin** instead

See code comments in `processDatabaseBackup()` for integration point.

### 4. File Cleanup (✅ Implemented)

```typescript
type: 'file-cleanup'
payload: {
  target: 'completed_jobs' | 'failed_jobs',
  older_than_hours?: number,  // For completed_jobs
  older_than_days?: number,   // For failed_jobs
}
```

**Status**: Fully implemented. Calls database cleanup functions:
- `cleanup_old_jobs(hours)` - Removes completed jobs older than N hours
- `cleanup_old_failed_jobs(days)` - Removes failed jobs older than N days

**Note**: File system cleanup (old_files target) is not implemented.

### 5. Custom Jobs (⚠️ Stub - Requires Integration)

```typescript
type: 'custom'
payload: {
  action: string,
  data: Record<string, unknown>
}
```

**Status**: Stub implementation that logs the action but does not call Hasura.

**Integration Required**: Replace stub in `ts/src/processors.ts` (lines 173-192) with Hasura Actions integration:
- Requires `HASURA_GRAPHQL_ENDPOINT` environment variable
- Requires `HASURA_ADMIN_SECRET` environment variable
- Make POST request to Hasura endpoint with action name and data
- Use `graphql-request` or `fetch()` to call Hasura Actions

See code comments in `processCustomJob()` for integration point.

## API Endpoints

### Create Job

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

### Get Job

```http
GET /api/jobs/:id
```

### Statistics

```http
GET /api/stats
```

## Database Schema

### Tables

- **jobs** - Core job metadata and status
- **job_results** - Successful job outputs
- **job_failures** - Failed job attempts with stack traces
- **job_schedules** - Cron-based recurring jobs

### Views

- **jobs_active** - Currently running jobs
- **jobs_failed_details** - Failed jobs with error details
- **queue_stats** - Queue statistics
- **job_type_stats** - Job type statistics
- **recent_failures** - Recent failures (last 24 hours)
- **scheduled_jobs_overview** - Scheduled jobs overview

### Functions

- **get_job_stats(queue, hours)** - Get job statistics
- **cleanup_old_jobs(hours)** - Clean old completed jobs
- **cleanup_old_failed_jobs(days)** - Clean old failed jobs

## Monitoring

### BullBoard Dashboard

Access at `http://localhost:3105/dashboard` to:

- View all queues
- Monitor job progress
- Inspect job data
- Retry failed jobs
- Pause/resume queues
- View job logs

### Statistics

```bash
# View comprehensive stats
nself plugin jobs stats

# Performance metrics
nself plugin jobs stats --performance

# Watch mode (live updates)
nself plugin jobs stats --watch
```

## Production Deployment

### 1. Multiple Workers

Run multiple worker processes for high throughput:

```bash
# Worker 1 - default queue
JOBS_DEFAULT_CONCURRENCY=10 nself plugin jobs worker &

# Worker 2 - high-priority queue
JOBS_DEFAULT_CONCURRENCY=5 nself plugin jobs worker high-priority &

# Worker 3 - low-priority queue
JOBS_DEFAULT_CONCURRENCY=20 nself plugin jobs worker low-priority &
```

### 2. Process Manager

Use PM2 or systemd for production:

```bash
# PM2
pm2 start "nself plugin jobs worker" --name jobs-worker-default
pm2 start "nself plugin jobs worker high-priority" --name jobs-worker-high
pm2 start "nself plugin jobs server" --name jobs-dashboard

# systemd (create service files)
systemctl start jobs-worker@default
systemctl start jobs-worker@high-priority
systemctl start jobs-dashboard
```

### 3. Auto-cleanup

Schedule automatic cleanup via cron:

```cron
# Cleanup completed jobs daily at 2 AM
0 2 * * * cd /path/to/plugin && nself plugin jobs schedule create --name auto-cleanup --type file-cleanup --cron "0 2 * * *" --payload '{"target": "completed_jobs", "older_than_hours": 24}'
```

## Integration with Hasura

### Register Custom Job via Action

```graphql
mutation CreateJob {
  createJob(
    type: "send-email"
    payload: {
      to: "user@example.com"
      subject: "Welcome"
      body: "Thanks for signing up!"
    }
    options: {
      priority: HIGH
      maxRetries: 3
    }
  ) {
    jobId
    queue
    status
  }
}
```

### Query Job Status

```graphql
query GetJob($id: uuid!) {
  jobs_by_pk(id: $id) {
    id
    status
    progress
    created_at
    started_at
    completed_at
    retry_count
    result {
      result
      duration_ms
    }
    failures {
      error_message
      attempt_number
      failed_at
    }
  }
}
```

## Architecture

```
┌─────────────────┐
│   Application   │
│   (Hasura)      │
└────────┬────────┘
         │
         │ Add Jobs
         ▼
┌─────────────────┐         ┌──────────────┐
│   BullMQ        │◄────────┤    Redis     │
│   Queue         │         │   (Queue)    │
└────────┬────────┘         └──────────────┘
         │
         │ Process Jobs
         ▼
┌─────────────────┐         ┌──────────────┐
│   Workers       │────────►│  PostgreSQL  │
│   (Multiple)    │  Track  │  (Metadata)  │
└─────────────────┘         └──────────────┘
         │
         │ Monitor
         ▼
┌─────────────────┐
│   BullBoard     │
│   Dashboard     │
└─────────────────┘
```

## Troubleshooting

### Redis Connection Issues

```bash
# Test Redis connection
redis-cli -h localhost -p 6379 ping

# Check Redis URL in .env
echo $JOBS_REDIS_URL
```

### Database Issues

```bash
# Verify tables exist
nself plugin jobs init

# Check database connection
psql -h localhost -U postgres -d nself -c "SELECT COUNT(*) FROM jobs;"
```

### Jobs Not Processing

```bash
# Check if worker is running
ps aux | grep "nself plugin jobs worker"

# Check worker logs
tail -f ~/.nself/logs/plugins/jobs/worker.log

# Verify queue has jobs
redis-cli llen bull:default:waiting
```

### Failed Jobs Not Retrying

```bash
# Check retry configuration
nself plugin jobs stats

# Manual retry
nself plugin jobs retry --show
nself plugin jobs retry --limit 50
```

## Performance Tuning

### Concurrency

Increase worker concurrency based on CPU cores:

```bash
# 4 cores = 8-16 concurrent jobs
JOBS_DEFAULT_CONCURRENCY=12 nself plugin jobs worker
```

### Queue Priorities

Distribute load across queues:

```typescript
// Critical operations
queue.add('payment-processing', data, { priority: 10 });

// Normal operations
queue.add('email-notification', data, { priority: 0 });

// Background tasks
queue.add('analytics-update', data, { priority: -5 });
```

### Memory Management

Configure Redis maxmemory:

```conf
# redis.conf
maxmemory 2gb
maxmemory-policy allkeys-lru
```

## Implementation Roadmap

### Phase 1: Current State ✅
- Job queue infrastructure (BullMQ + Redis)
- HTTP request processor
- File cleanup processor
- Database schema and views
- BullBoard dashboard
- CLI commands and API endpoints

### Phase 2: Email Integration (Planned)
**Estimated Effort:** 2-3 hours

1. Choose email provider (SendGrid, AWS SES, Mailgun, or Nodemailer)
2. Install provider SDK (`npm install @sendgrid/mail` or similar)
3. Add environment variables (API keys, SMTP credentials)
4. Replace stub in `ts/src/processors.ts` (lines 27-49)
5. Test with real email sending

### Phase 3: Database Backup Integration (Planned)
**Estimated Effort:** 3-4 hours

1. Verify `pg_dump` binary is installed and in PATH
2. Add database connection credentials to environment
3. Replace stub in `ts/src/processors.ts` (lines 101-130)
4. Implement using `child_process.spawn('pg_dump', [...])`
5. Add compression and encryption support
6. Test with real database backup

**Note:** For production use, the dedicated **backup plugin** is recommended instead.

### Phase 4: Hasura Actions Integration (Planned)
**Estimated Effort:** 2-3 hours

1. Configure Hasura GraphQL endpoint and admin secret
2. Add environment variables (`HASURA_GRAPHQL_ENDPOINT`, `HASURA_ADMIN_SECRET`)
3. Replace stub in `ts/src/processors.ts` (lines 173-192)
4. Implement using `graphql-request` or `fetch()`
5. Test with real Hasura actions

**Total Estimated Effort for Full Implementation:** 7-10 hours

## Migration Guide

When email, backup, or Hasura integrations are implemented:

1. **No database migrations needed** - Schema is already complete
2. **Add environment variables** - Provider API keys and endpoints
3. **Install provider SDKs** - `npm install` in `ts/` directory
4. **Update processor code** - Replace stubs in `ts/src/processors.ts`
5. **Restart workers** - `nself plugin jobs worker` picks up new code

No breaking changes expected - jobs will transparently upgrade from stub to working implementation.

## License

Source-Available (see LICENSE)

## Support

- Documentation: https://github.com/nself-org/plugins
- Issues: https://github.com/nself-org/plugins/issues
- nself CLI: https://github.com/nself-org/cli
