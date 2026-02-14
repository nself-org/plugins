# Observability Plugin

Unified observability platform for nself with Prometheus metrics, structured logging to Loki, and distributed tracing to Tempo.

## Features

- **Prometheus Metrics**: Expose custom metrics in Prometheus format
- **Structured Logging**: Send JSON logs to Loki for centralized log aggregation
- **Distributed Tracing**: OpenTelemetry traces exported to Tempo/Jaeger
- **Grafana Integration**: Pre-built dashboards for system and business metrics

## Installation

```bash
# Install the plugin
nself plugin install observability

# Start the observability server
nself plugin run observability server
```

## Configuration

### Environment Variables

```bash
# Server configuration
OBSERVABILITY_PLUGIN_PORT=3215
OBSERVABILITY_PLUGIN_HOST=0.0.0.0

# Prometheus metrics
OBSERVABILITY_PROMETHEUS_ENABLED=true
OBSERVABILITY_METRICS_PATH=/metrics

# Loki (log aggregation)
OBSERVABILITY_LOKI_ENABLED=true
OBSERVABILITY_LOKI_URL=http://loki:3100

# Tempo (distributed tracing)
OBSERVABILITY_TEMPO_ENABLED=true
OBSERVABILITY_TEMPO_URL=http://tempo:9411

# Grafana
OBSERVABILITY_GRAFANA_URL=http://grafana:3000
OBSERVABILITY_GRAFANA_API_KEY=your_grafana_api_key

# Logging
LOG_LEVEL=info
```

## API Endpoints

### Metrics

```bash
# Get Prometheus metrics
GET /metrics
```

Returns metrics in Prometheus exposition format:

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
```

### Logging

```bash
# Ingest log entries
POST /api/observability/logs
Content-Type: application/json

{
  "level": "info",
  "message": "User login successful",
  "timestamp": "2026-02-14T12:00:00Z",
  "trace_id": "abc123",
  "span_id": "xyz789",
  "user_id": "user_123",
  "metadata": {
    "ip_address": "192.168.1.1",
    "user_agent": "Mozilla/5.0..."
  }
}
```

### Tracing

```bash
# Ingest trace spans
POST /api/observability/traces
Content-Type: application/json

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
    "http.status_code": 200
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

### Dashboards

```bash
# List all Grafana dashboards
GET /api/observability/dashboards

# Get specific dashboard JSON
GET /api/observability/dashboards/:id
```

## Metric Types

### HTTP Metrics

- `http_requests_total` - Total HTTP requests by method, endpoint, status
- `http_request_duration_seconds` - Request duration histogram
- `http_request_size_bytes` - Request body size
- `http_response_size_bytes` - Response body size

### Database Metrics

- `db_queries_total` - Total database queries
- `db_query_duration_seconds` - Query duration histogram
- `db_connections_active` - Active database connections
- `db_connections_idle` - Idle database connections

### Queue Metrics

- `queue_size` - Current queue size
- `queue_jobs_processed_total` - Total jobs processed
- `queue_job_duration_seconds` - Job processing duration
- `queue_jobs_failed_total` - Total failed jobs

### Business Metrics

- `videos_uploaded_total` - Total videos uploaded
- `streams_started_total` - Total streams started
- `users_active` - Active users count
- `errors_total` - Total errors by type

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
  "source_account_id": "primary",
  "service": "api",
  "metadata": {
    "custom_field": "value"
  }
}
```

### Log Levels

- `debug` - Detailed debugging information
- `info` - General informational messages
- `warn` - Warning messages for potential issues
- `error` - Error messages for failures

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
    "key": "value"
  },
  "logs": [
    {
      "timestamp": "2026-02-14T12:00:00.5Z",
      "message": "log_message",
      "fields": {}
    }
  ]
}
```

## Pre-Built Dashboards

The plugin includes pre-built Grafana dashboards for:

1. **System Overview**
   - CPU usage
   - Memory usage
   - Disk I/O
   - Network traffic

2. **Database Performance**
   - Queries per second
   - Slow queries
   - Connection pool stats
   - Query duration percentiles

3. **Job Queue Health**
   - Queue size over time
   - Processing rate
   - Failed jobs
   - Job duration distribution

4. **Business Metrics**
   - Video uploads
   - Stream starts
   - Active users
   - Error rates

## Integration Example

### TypeScript/JavaScript

```typescript
import { PrometheusClient } from '@nself/plugin-observability';

const metrics = new PrometheusClient('http://localhost:3215');

// Increment counter
await metrics.incrementCounter('http_requests_total', {
  method: 'GET',
  endpoint: '/api/users',
  status: '200'
});

// Record histogram
await metrics.recordHistogram('http_request_duration_seconds', 0.123, {
  endpoint: '/api/users'
});

// Send log
await metrics.sendLog({
  level: 'info',
  message: 'User logged in',
  user_id: 'user_123',
  metadata: { ip_address: '192.168.1.1' }
});

// Send trace
await metrics.sendTrace({
  trace_id: 'abc123',
  span_id: 'span_001',
  operation_name: 'handle_login',
  start_time: new Date(),
  end_time: new Date(),
  tags: { 'http.method': 'POST' }
});
```

## Retention

- **Metrics**: Prometheus retention (default: 15 days)
- **Logs**: Loki retention (default: 30 days)
- **Traces**: Tempo retention (default: 7 days)

Configure retention in your Prometheus, Loki, and Tempo configurations.

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
```

## Performance

- `/metrics` endpoint responds in <100ms
- Log ingestion: 10,000+ logs/second
- Trace ingestion: 1,000+ spans/second

## Troubleshooting

### Metrics not appearing

Check that Prometheus is configured to scrape the observability plugin:

```yaml
scrape_configs:
  - job_name: 'observability'
    static_configs:
      - targets: ['observability:3215']
```

### Logs not in Loki

Verify Loki URL and ensure Loki is running:

```bash
curl http://loki:3100/ready
```

### Traces not in Tempo

Verify Tempo URL and ensure Tempo is running:

```bash
curl http://tempo:9411/api/traces
```

## License

Source-Available
