# Observability Plugin

Unified observability platform for nself with Prometheus metrics, structured logging to Loki, and distributed tracing to Tempo.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [API Endpoints](#api-endpoints)
- [Metric Types](#metric-types)
- [Log Structure](#log-structure)
- [Trace Structure](#trace-structure)
- [Pre-Built Dashboards](#pre-built-dashboards)
- [Integration Example](#integration-example)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Observability plugin provides a complete observability stack for nself applications with metrics, logs, and traces. It integrates with the Grafana stack for comprehensive monitoring and debugging.

### Key Features

- **Prometheus Metrics** - Expose custom metrics in Prometheus format
- **Structured Logging** - Send JSON logs to Loki for centralized log aggregation
- **Distributed Tracing** - OpenTelemetry traces exported to Tempo/Jaeger
- **Grafana Integration** - Pre-built dashboards for system and business metrics
- **Multi-App Support** - Full tenant isolation with `source_account_id` in logs
- **High Performance** - 10,000+ logs/second, 1,000+ traces/second

### Architecture

```
┌─────────────┐
│   Your App  │
└─────┬───────┘
      │ Metrics, Logs, Traces
      ▼
┌─────────────────┐
│  Observability  │  (Port 3215)
│     Plugin      │
└─────┬───────────┘
      │
      ├──────────▶ Prometheus (metrics)
      ├──────────▶ Loki (logs)
      ├──────────▶ Tempo (traces)
      └──────────▶ Grafana (visualization)
```

---

## Quick Start

```bash
# Install the plugin
nself plugin install observability

# Configure environment
echo "OBSERVABILITY_LOKI_URL=http://loki:3100" >> .env
echo "OBSERVABILITY_TEMPO_URL=http://tempo:9411" >> .env
echo "OBSERVABILITY_GRAFANA_URL=http://grafana:3000" >> .env

# Start the observability server
nself plugin observability server
```

Server will be available at `http://localhost:3215`

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OBSERVABILITY_PLUGIN_PORT` | `3215` | HTTP server port |
| `OBSERVABILITY_PLUGIN_HOST` | `0.0.0.0` | HTTP server host |
| `OBSERVABILITY_PROMETHEUS_ENABLED` | `true` | Enable Prometheus metrics endpoint |
| `OBSERVABILITY_METRICS_PATH` | `/metrics` | Prometheus metrics path |
| `OBSERVABILITY_LOKI_ENABLED` | `true` | Enable Loki log aggregation |
| `OBSERVABILITY_LOKI_URL` | `http://loki:3100` | Loki server URL |
| `OBSERVABILITY_TEMPO_ENABLED` | `true` | Enable Tempo distributed tracing |
| `OBSERVABILITY_TEMPO_URL` | `http://tempo:9411` | Tempo server URL |
| `OBSERVABILITY_GRAFANA_URL` | `http://grafana:3000` | Grafana server URL |
| `OBSERVABILITY_GRAFANA_API_KEY` | - | Grafana API key for dashboard management |
| `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

---

## API Endpoints

### Metrics Endpoint

```
GET /metrics
```

Returns metrics in Prometheus exposition format.

**Example Response:**
```
# HELP http_requests_total Total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",endpoint="/api/users",status="200"} 1234

# HELP http_request_duration_seconds HTTP request duration in seconds
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 500
http_request_duration_seconds_bucket{le="0.5"} 800
http_request_duration_seconds_bucket{le="1"} 950
http_request_duration_seconds_sum 456.78
http_request_duration_seconds_count 1000

# HELP db_query_duration_seconds Database query duration
# TYPE db_query_duration_seconds histogram
db_query_duration_seconds_bucket{le="0.01"} 250
db_query_duration_seconds_bucket{le="0.05"} 450
db_query_duration_seconds_sum 15.67
db_query_duration_seconds_count 500
```

### Ingest Logs

```
POST /api/observability/logs
Content-Type: application/json
```

**Request Body:**
```json
{
  "level": "info",
  "message": "User login successful",
  "timestamp": "2026-02-14T12:00:00Z",
  "trace_id": "abc123",
  "span_id": "xyz789",
  "user_id": "user_123",
  "source_account_id": "tenant_456",
  "metadata": {
    "ip_address": "192.168.1.1",
    "user_agent": "Mozilla/5.0..."
  }
}
```

**Response:**
```json
{
  "status": "ok",
  "ingested": true
}
```

### Ingest Traces

```
POST /api/observability/traces
Content-Type: application/json
```

**Request Body:**
```json
{
  "trace_id": "abc123",
  "span_id": "span_001",
  "parent_span_id": null,
  "operation_name": "handle_request",
  "start_time": "2026-02-14T12:00:00Z",
  "end_time": "2026-02-14T12:00:01Z",
  "duration_ms": 1000,
  "tags": {
    "http.method": "GET",
    "http.url": "/api/users",
    "http.status_code": 200,
    "source_account_id": "tenant_456"
  },
  "logs": [
    {
      "timestamp": "2026-02-14T12:00:00.5Z",
      "message": "Querying database",
      "fields": {
        "query": "SELECT * FROM users WHERE id = $1"
      }
    }
  ]
}
```

**Response:**
```json
{
  "status": "ok",
  "trace_id": "abc123",
  "span_id": "span_001"
}
```

### List Dashboards

```
GET /api/observability/dashboards
```

Returns list of available Grafana dashboards.

**Response:**
```json
{
  "dashboards": [
    {
      "id": "system-overview",
      "title": "System Overview",
      "url": "http://grafana:3000/d/system-overview"
    },
    {
      "id": "database-performance",
      "title": "Database Performance",
      "url": "http://grafana:3000/d/database-performance"
    }
  ]
}
```

### Get Dashboard

```
GET /api/observability/dashboards/:id
```

Returns Grafana dashboard JSON.

---

## Metric Types

### HTTP Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `http_requests_total` | Counter | `method`, `endpoint`, `status` | Total HTTP requests |
| `http_request_duration_seconds` | Histogram | `method`, `endpoint` | Request duration |
| `http_request_size_bytes` | Histogram | `method`, `endpoint` | Request body size |
| `http_response_size_bytes` | Histogram | `method`, `endpoint` | Response body size |
| `http_requests_in_flight` | Gauge | `method` | Current in-flight requests |

### Database Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `db_queries_total` | Counter | `operation`, `table` | Total database queries |
| `db_query_duration_seconds` | Histogram | `operation`, `table` | Query duration |
| `db_connections_active` | Gauge | - | Active database connections |
| `db_connections_idle` | Gauge | - | Idle database connections |
| `db_errors_total` | Counter | `type` | Database errors |

### Queue Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `queue_size` | Gauge | `queue_name` | Current queue size |
| `queue_jobs_processed_total` | Counter | `queue_name`, `status` | Total jobs processed |
| `queue_job_duration_seconds` | Histogram | `queue_name` | Job processing duration |
| `queue_jobs_failed_total` | Counter | `queue_name`, `failure_reason` | Total failed jobs |
| `queue_jobs_retried_total` | Counter | `queue_name` | Total job retries |

### Business Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `videos_uploaded_total` | Counter | `source_account_id` | Total videos uploaded |
| `streams_started_total` | Counter | `source_account_id` | Total streams started |
| `users_active` | Gauge | `source_account_id` | Active users count |
| `errors_total` | Counter | `type`, `severity` | Total application errors |
| `cache_hits_total` | Counter | `cache_name` | Cache hits |
| `cache_misses_total` | Counter | `cache_name` | Cache misses |

---

## Log Structure

All logs follow this JSON structure:

```json
{
  "timestamp": "2026-02-14T12:00:00Z",
  "level": "info",
  "message": "Operation completed",
  "trace_id": "abc123",
  "span_id": "xyz789",
  "user_id": "user_123",
  "source_account_id": "tenant_456",
  "service": "api",
  "metadata": {
    "custom_field": "value"
  }
}
```

### Required Fields

- `timestamp` - ISO 8601 timestamp
- `level` - Log level: `debug`, `info`, `warn`, `error`
- `message` - Human-readable log message

### Optional Fields

- `trace_id` - Distributed trace ID
- `span_id` - Current span ID
- `user_id` - User identifier
- `source_account_id` - Multi-app tenant identifier
- `service` - Service name
- `metadata` - Additional key-value pairs

### Log Levels

| Level | Usage |
|-------|-------|
| `debug` | Detailed debugging information (verbose) |
| `info` | General informational messages |
| `warn` | Warning messages for potential issues |
| `error` | Error messages for failures |

---

## Trace Structure

Traces follow the OpenTelemetry format:

```json
{
  "trace_id": "unique_trace_id",
  "span_id": "unique_span_id",
  "parent_span_id": "parent_span_id or null",
  "operation_name": "operation_description",
  "start_time": "2026-02-14T12:00:00Z",
  "end_time": "2026-02-14T12:00:01Z",
  "duration_ms": 1000,
  "tags": {
    "http.method": "GET",
    "http.url": "/api/users",
    "http.status_code": 200,
    "source_account_id": "tenant_456",
    "user_id": "user_123"
  },
  "logs": [
    {
      "timestamp": "2026-02-14T12:00:00.5Z",
      "message": "log_message",
      "fields": {
        "query": "SELECT ...",
        "duration_ms": 45
      }
    }
  ]
}
```

### Required Fields

- `trace_id` - Unique trace identifier (shared across all spans in trace)
- `span_id` - Unique span identifier
- `operation_name` - Name of the operation
- `start_time` - Span start time (ISO 8601)
- `end_time` - Span end time (ISO 8601)

### Optional Fields

- `parent_span_id` - Parent span ID (null for root span)
- `duration_ms` - Duration in milliseconds
- `tags` - Key-value metadata
- `logs` - Structured log events within the span

### Standard Tags

- `http.method` - HTTP method
- `http.url` - Request URL
- `http.status_code` - HTTP status code
- `db.statement` - SQL query
- `db.type` - Database type
- `error` - Boolean indicating error
- `error.message` - Error message
- `source_account_id` - Tenant identifier

---

## Pre-Built Dashboards

The plugin includes pre-built Grafana dashboards for:

### 1. System Overview

Metrics:
- CPU usage
- Memory usage
- Disk I/O
- Network traffic
- System load

### 2. Database Performance

Metrics:
- Queries per second
- Slow queries (p95, p99)
- Connection pool stats
- Query duration percentiles
- Error rates

### 3. Job Queue Health

Metrics:
- Queue size over time
- Processing rate
- Failed jobs
- Job duration distribution
- Retry rates

### 4. Business Metrics

Metrics:
- Video uploads
- Stream starts
- Active users
- Content views
- Error rates

### 5. HTTP Traffic

Metrics:
- Requests per second
- Response times (p50, p95, p99)
- Error rates by endpoint
- Request/response sizes
- Status code distribution

---

## Integration Example

### TypeScript/JavaScript

```typescript
import fetch from 'node-fetch';

// Increment counter metric
async function incrementCounter(name: string, labels: Record<string, string>, value = 1) {
  await fetch('http://localhost:3215/api/observability/metrics', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type: 'counter',
      name,
      value,
      labels
    })
  });
}

// Send log
async function sendLog(level: string, message: string, metadata?: any) {
  await fetch('http://localhost:3215/api/observability/logs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      level,
      message,
      timestamp: new Date().toISOString(),
      metadata
    })
  });
}

// Send trace
async function sendTrace(trace: any) {
  await fetch('http://localhost:3215/api/observability/traces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(trace)
  });
}

// Example usage
await incrementCounter('http_requests_total', {
  method: 'GET',
  endpoint: '/api/users',
  status: '200'
});

await sendLog('info', 'User logged in', {
  user_id: 'user_123',
  ip_address: '192.168.1.1'
});

await sendTrace({
  trace_id: 'abc123',
  span_id: 'span_001',
  operation_name: 'handle_login',
  start_time: new Date().toISOString(),
  end_time: new Date().toISOString(),
  tags: { 'http.method': 'POST' }
});
```

### Python

```python
import requests
from datetime import datetime

def increment_counter(name, labels, value=1):
    requests.post('http://localhost:3215/api/observability/metrics', json={
        'type': 'counter',
        'name': name,
        'value': value,
        'labels': labels
    })

def send_log(level, message, metadata=None):
    requests.post('http://localhost:3215/api/observability/logs', json={
        'level': level,
        'message': message,
        'timestamp': datetime.utcnow().isoformat() + 'Z',
        'metadata': metadata or {}
    })

def send_trace(trace):
    requests.post('http://localhost:3215/api/observability/traces', json=trace)

# Example usage
increment_counter('http_requests_total', {
    'method': 'GET',
    'endpoint': '/api/users',
    'status': '200'
})

send_log('info', 'User logged in', {
    'user_id': 'user_123',
    'ip_address': '192.168.1.1'
})
```

---

## Retention

Configure retention in your stack:

| Component | Default Retention | Configuration |
|-----------|-------------------|---------------|
| Prometheus | 15 days | `--storage.tsdb.retention.time=15d` |
| Loki | 30 days | `retention_period: 720h` in config |
| Tempo | 7 days | `block_retention: 168h` in config |

---

## Alerting

Configure alerts in Prometheus Alertmanager:

```yaml
groups:
  - name: http
    interval: 30s
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High HTTP error rate detected"
          description: "Error rate is {{ $value }} (threshold: 0.05)"

      - alert: SlowResponseTime
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 1.0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Slow HTTP response times"
          description: "P95 response time is {{ $value }}s (threshold: 1s)"

  - name: database
    interval: 30s
    rules:
      - alert: HighDatabaseLoad
        expr: rate(db_queries_total[5m]) > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High database query rate"
          description: "Query rate is {{ $value }}/sec (threshold: 1000/sec)"
```

---

## Performance

- `/metrics` endpoint responds in <100ms
- Log ingestion: **10,000+ logs/second**
- Trace ingestion: **1,000+ spans/second**
- Memory usage: ~50MB idle, ~200MB under load
- CPU usage: <5% idle, ~20% under load

---

## Troubleshooting

### Metrics not appearing in Prometheus

Check that Prometheus is configured to scrape the observability plugin:

```yaml
scrape_configs:
  - job_name: 'observability'
    static_configs:
      - targets: ['observability:3215']
    scrape_interval: 15s
```

Verify Prometheus targets:
```bash
curl http://prometheus:9090/api/v1/targets
```

### Logs not appearing in Loki

Verify Loki URL and ensure Loki is running:

```bash
curl http://loki:3100/ready
```

Check Loki logs for errors:
```bash
docker logs loki
```

Test log ingestion:
```bash
curl -X POST http://localhost:3215/api/observability/logs \
  -H "Content-Type: application/json" \
  -d '{"level":"info","message":"test"}'
```

### Traces not appearing in Tempo

Verify Tempo URL and ensure Tempo is running:

```bash
curl http://tempo:9411/api/traces
```

Check Tempo configuration:
```bash
docker logs tempo
```

### High Memory Usage

- Reduce metrics cardinality (fewer label combinations)
- Lower retention periods
- Increase Prometheus/Loki/Tempo scrape intervals

---

## License

Source-Available
