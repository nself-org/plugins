# Jobs Plugin for nself

BullMQ-based background job queue with priorities, scheduling, retry logic, and BullBoard dashboard.

## Features

- **Multiple Queues**: `default`, `high-priority`, `low-priority`
- **Job Priorities**: `critical`, `high`, `normal`, `low`
- **Retry Logic**: Configurable exponential backoff
- **Cron Scheduling**: Recurring jobs with cron expressions
- **BullBoard Dashboard**: Web UI for monitoring jobs
- **Pre-built Job Types**:
  - `send-email` - Email sending
  - `http-request` - HTTP requests with retry
  - `database-backup` - PostgreSQL backups
  - `file-cleanup` - Clean old jobs
  - `custom` - Custom jobs via Hasura Actions
- **Full Persistence**: All jobs tracked in PostgreSQL
- **Telemetry**: Job statistics and performance metrics

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

### 1. Send Email

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

**Note**: Integrate with your email service (SendGrid, AWS SES, etc.) in `src/processors.ts`

### 2. HTTP Request

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

### 3. Database Backup

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

### 4. File Cleanup

```typescript
type: 'file-cleanup'
payload: {
  target: 'completed_jobs' | 'failed_jobs' | 'old_files',
  older_than_hours?: number,
  older_than_days?: number,
  path?: string,
  pattern?: string
}
```

### 5. Custom Jobs

```typescript
type: 'custom'
payload: {
  action: string,
  data: Record<string, unknown>
}
```

Integrate with Hasura Actions for custom business logic.

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

## License

Source-Available (see LICENSE)

## Support

- Documentation: https://github.com/acamarata/nself-plugins
- Issues: https://github.com/acamarata/nself-plugins/issues
- nself CLI: https://github.com/acamarata/nself
