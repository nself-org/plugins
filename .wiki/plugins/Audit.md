# Audit Plugin

**Port:** 3303
**Status:** Production Ready
**Category:** Security
**Dependencies:** PostgreSQL

Immutable audit logging with SIEM integration, compliance frameworks (SOC2, HIPAA, GDPR, PCI), and retention policies. Provides tamper-proof event tracking with cryptographic checksums.

---

## Overview

The Audit plugin provides a complete audit logging solution with:

- **Immutable append-only log** - Events cannot be modified or deleted (enforced by database triggers)
- **Cryptographic integrity** - SHA-256 checksums verify event integrity
- **SIEM integration** - Export to Splunk, ELK, Datadog in CEF, LEEF, Syslog formats
- **Compliance reports** - Generate SOC2, HIPAA, GDPR, PCI compliance reports
- **Retention policies** - Automated event lifecycle management (up to 7 years)
- **Alert rules** - Real-time alerting on security events
- **Fallback logging** - Never lose events even if database fails

---

## Quick Start

### 1. Install Dependencies

```bash
cd plugins/audit/ts
npm install
npm run build
```

### 2. Configure Environment

```bash
cp .env.example .env
# Edit .env with your configuration
```

Required:
- PostgreSQL connection

Optional:
- SIEM integration credentials
- Alert webhook URLs
- Custom retention periods

### 3. Initialize Database

```bash
npm run cli -- init
```

This creates:
- `audit_events` table with immutability triggers
- `audit_retention_policies` table
- `audit_alert_rules` table
- `audit_webhook_events` table

### 4. Start Server

```bash
npm run dev
```

Server starts on port 3303.

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `AUDIT_PLUGIN_PORT` | No | 3303 | HTTP server port |
| `AUDIT_PLUGIN_HOST` | No | 0.0.0.0 | HTTP server host |
| `AUDIT_LOG_LEVEL` | No | info | Log level (debug/info/warn/error) |
| `POSTGRES_HOST` | Yes | localhost | PostgreSQL host |
| `POSTGRES_PORT` | Yes | 5432 | PostgreSQL port |
| `POSTGRES_DB` | Yes | nself | PostgreSQL database |
| `POSTGRES_USER` | Yes | postgres | PostgreSQL user |
| `POSTGRES_PASSWORD` | Yes | - | PostgreSQL password |
| `POSTGRES_SSL` | No | false | Enable SSL |
| `AUDIT_APP_IDS` | No | primary | Comma-separated app IDs |
| `AUDIT_FALLBACK_LOG_PATH` | No | /var/log/nself/audit-fallback.jsonl | Fallback log file |
| `AUDIT_SIEM_SPLUNK_HEC_URL` | No | - | Splunk HEC URL |
| `AUDIT_SIEM_SPLUNK_HEC_TOKEN` | No | - | Splunk HEC token |
| `AUDIT_SIEM_ELK_URL` | No | - | Elasticsearch URL |
| `AUDIT_SIEM_ELK_INDEX` | No | audit-logs | Elasticsearch index |
| `AUDIT_SIEM_ELK_API_KEY` | No | - | Elasticsearch API key |
| `AUDIT_SIEM_DATADOG_API_KEY` | No | - | Datadog API key |
| `AUDIT_SIEM_DATADOG_SITE` | No | datadoghq.com | Datadog site |
| `AUDIT_DEFAULT_RETENTION_DAYS` | No | 2555 | Default retention (7 years) |
| `AUDIT_COMPLIANCE_FRAMEWORKS` | No | SOC2,HIPAA,GDPR,PCI | Supported frameworks |
| `AUDIT_ALERT_WEBHOOK_URL` | No | - | Global alert webhook URL |
| `AUDIT_EXPORT_MAX_ROWS` | No | 100000 | Max rows per export |

---

## CLI Commands

### Initialize Schema

```bash
npm run cli -- init
```

### Log an Event

```bash
npm run cli -- log \
  --plugin auth \
  --event-type auth.login.success \
  --action login \
  --actor-id user123 \
  --actor-type user \
  --resource-type session \
  --resource-id sess_abc123 \
  --outcome success \
  --severity low \
  --ip 192.168.1.100 \
  --user-agent "Mozilla/5.0..." \
  --location "San Francisco, CA"
```

### Query Events

```bash
# All events
npm run cli -- query

# Filter by plugin
npm run cli -- query --plugin auth

# Filter by event type
npm run cli -- query --event-type "auth.login.*"

# Filter by date range
npm run cli -- query \
  --start-date 2026-01-01T00:00:00Z \
  --end-date 2026-01-31T23:59:59Z

# Filter by severity
npm run cli -- query --severity critical --limit 50
```

### Export Events

```bash
# Export to JSON
npm run cli -- export --format json --output audit.json

# Export to CSV
npm run cli -- export --format csv --output audit.csv

# Export to CEF (Splunk/ArcSight)
npm run cli -- export --format cef --output audit.cef

# Export to LEEF (IBM QRadar)
npm run cli -- export --format leef --output audit.leef

# Export to Syslog
npm run cli -- export --format syslog --output audit.log

# Export with filters
npm run cli -- export \
  --format json \
  --plugin auth \
  --start-date 2026-01-01T00:00:00Z \
  --limit 10000 \
  --output audit.json
```

### Verify Event Integrity

```bash
npm run cli -- verify --event-id <event-uuid>
```

### Generate Compliance Report

```bash
# SOC2 report for last 30 days
npm run cli -- compliance --framework SOC2

# HIPAA report with date range
npm run cli -- compliance \
  --framework HIPAA \
  --start-date 2026-01-01T00:00:00Z \
  --end-date 2026-03-31T23:59:59Z \
  --output hipaa-q1-2026.json

# GDPR report
npm run cli -- compliance --framework GDPR

# PCI DSS report
npm run cli -- compliance --framework PCI
```

### Statistics

```bash
npm run cli -- stats
```

---

## REST API

### Health Checks

#### GET /health
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "audit",
  "timestamp": "2026-02-10T12:00:00Z",
  "version": "1.0.0"
}
```

#### GET /ready
Readiness check (database + triggers).

**Response:**
```json
{
  "ready": true,
  "database": "ok",
  "immutabilityTriggers": "ok",
  "timestamp": "2026-02-10T12:00:00Z"
}
```

#### GET /live
Liveness check with stats.

**Response:**
```json
{
  "alive": true,
  "uptime": 3600,
  "memory": {
    "used": 50000000,
    "total": 100000000
  },
  "stats": {
    "totalEvents": 1000000,
    "last24Hours": 5000,
    "last7Days": 35000,
    "retentionPolicies": 3,
    "alertRules": 5,
    "oldestEvent": "2024-01-01T00:00:00Z",
    "newestEvent": "2026-02-10T12:00:00Z",
    "diskUsageMB": null
  }
}
```

### Event Logging

#### POST /v1/events
Log an audit event (append-only).

**Headers:**
- `X-App-Id`: Application ID (optional, default: "primary")

**Request:**
```json
{
  "sourcePlugin": "auth",
  "eventType": "auth.login.success",
  "actorId": "user123",
  "actorType": "user",
  "resourceType": "session",
  "resourceId": "sess_abc123",
  "action": "login",
  "outcome": "success",
  "severity": "low",
  "ipAddress": "192.168.1.100",
  "userAgent": "Mozilla/5.0...",
  "location": "San Francisco, CA",
  "details": {
    "method": "password",
    "mfaUsed": true
  },
  "metadata": {
    "browser": "Chrome",
    "os": "macOS"
  }
}
```

**Response:**
```json
{
  "eventId": "evt_abc123",
  "checksum": "a1b2c3d4e5f6...",
  "createdAt": "2026-02-10T12:00:00Z"
}
```

### Query Events

#### GET /v1/events
Query audit events with filters.

**Query Parameters:**
- `sourcePlugin`: Filter by source plugin
- `eventType`: Filter by event type
- `actorId`: Filter by actor ID
- `resourceType`: Filter by resource type
- `resourceId`: Filter by resource ID
- `action`: Filter by action
- `outcome`: Filter by outcome (success/failure/unknown)
- `severity`: Filter by severity (low/medium/high/critical)
- `startDate`: Filter by start date (ISO 8601)
- `endDate`: Filter by end date (ISO 8601)
- `limit`: Max results (default: 100, max: 1000)
- `offset`: Offset for pagination (default: 0)

**Response:**
```json
{
  "events": [
    {
      "id": "evt_abc123",
      "sourcePlugin": "auth",
      "eventType": "auth.login.success",
      "actorId": "user123",
      "actorType": "user",
      "resourceType": "session",
      "resourceId": "sess_abc123",
      "action": "login",
      "outcome": "success",
      "severity": "low",
      "ipAddress": "192.168.1.100",
      "userAgent": "Mozilla/5.0...",
      "location": "San Francisco, CA",
      "details": {},
      "metadata": {},
      "checksum": "a1b2c3d4e5f6...",
      "createdAt": "2026-02-10T12:00:00Z"
    }
  ],
  "total": 1000,
  "limit": 100,
  "offset": 0
}
```

#### GET /v1/events/:id
Get a single event by ID.

**Response:**
```json
{
  "id": "evt_abc123",
  "sourcePlugin": "auth",
  "eventType": "auth.login.success",
  ...
}
```

### Export

#### POST /v1/export
Export events in various formats.

**Request:**
```json
{
  "format": "csv",
  "sourcePlugin": "auth",
  "eventType": "auth.login.*",
  "startDate": "2026-01-01T00:00:00Z",
  "endDate": "2026-01-31T23:59:59Z",
  "limit": 10000
}
```

**Formats:**
- `csv` - Comma-separated values
- `json` - JSON array
- `jsonl` - JSON Lines (one event per line)
- `cef` - Common Event Format (Splunk, ArcSight)
- `leef` - Log Event Extended Format (IBM QRadar)
- `syslog` - Syslog format (RFC 5424)

**Response:**
```json
{
  "format": "csv",
  "data": "id,app_id,source_plugin,...",
  "rowCount": 1000,
  "exportedAt": "2026-02-10T12:00:00Z"
}
```

### Retention Policies

#### POST /v1/retention
Create a retention policy.

**Request:**
```json
{
  "name": "auth-events-7-years",
  "description": "Retain auth events for 7 years per compliance",
  "eventTypePattern": "auth.*",
  "retentionDays": 2555,
  "enabled": true
}
```

**Response:**
```json
{
  "id": "pol_abc123",
  "name": "auth-events-7-years",
  "description": "Retain auth events for 7 years per compliance",
  "eventTypePattern": "auth.*",
  "retentionDays": 2555,
  "enabled": true,
  "lastExecutedAt": null,
  "createdAt": "2026-02-10T12:00:00Z",
  "updatedAt": "2026-02-10T12:00:00Z"
}
```

#### GET /v1/retention
List all retention policies.

#### GET /v1/retention/:id
Get a specific retention policy.

#### PATCH /v1/retention/:id
Update a retention policy.

#### DELETE /v1/retention/:id
Delete a retention policy.

#### POST /v1/retention/execute
Execute all enabled retention policies.

**Response:**
```json
{
  "policiesExecuted": 3,
  "eventsDeleted": 1000,
  "executedAt": "2026-02-10T12:00:00Z"
}
```

### Alert Rules

#### POST /v1/alerts
Create an alert rule.

**Request:**
```json
{
  "name": "critical-security-events",
  "description": "Alert on all critical security events",
  "eventTypePattern": "*.security.*",
  "severityThreshold": "critical",
  "conditions": {
    "outcomeFilter": "failure"
  },
  "webhookUrl": "https://example.com/webhooks/alerts",
  "enabled": true
}
```

**Response:**
```json
{
  "id": "rule_abc123",
  "name": "critical-security-events",
  "description": "Alert on all critical security events",
  "eventTypePattern": "*.security.*",
  "severityThreshold": "critical",
  "conditions": {},
  "webhookUrl": "https://example.com/webhooks/alerts",
  "enabled": true,
  "lastTriggeredAt": null,
  "triggerCount": 0,
  "createdAt": "2026-02-10T12:00:00Z",
  "updatedAt": "2026-02-10T12:00:00Z"
}
```

#### GET /v1/alerts
List all alert rules.

#### GET /v1/alerts/:id
Get a specific alert rule.

#### PATCH /v1/alerts/:id
Update an alert rule.

#### DELETE /v1/alerts/:id
Delete an alert rule.

### Compliance Reports

#### POST /v1/compliance/reports
Generate a compliance report.

**Request:**
```json
{
  "framework": "SOC2",
  "startDate": "2026-01-01T00:00:00Z",
  "endDate": "2026-03-31T23:59:59Z"
}
```

**Response:**
```json
{
  "framework": "SOC2",
  "period": {
    "startDate": "2026-01-01T00:00:00Z",
    "endDate": "2026-03-31T23:59:59Z"
  },
  "summary": {
    "totalEvents": 100000,
    "criticalEvents": 10,
    "highSeverityEvents": 100,
    "failedActions": 50,
    "uniqueActors": 1000,
    "uniqueResources": 5000
  },
  "eventsByType": {
    "auth.login.success": 50000,
    "auth.login.failure": 50
  },
  "eventsBySeverity": {
    "low": 90000,
    "medium": 9000,
    "high": 900,
    "critical": 100
  },
  "eventsByOutcome": {
    "success": 99900,
    "failure": 100
  },
  "topActors": [
    { "actorId": "user123", "eventCount": 1000 }
  ],
  "topResources": [
    { "resourceType": "session", "resourceId": "sess_abc", "eventCount": 500 }
  ],
  "alertsTriggered": 5,
  "complianceChecks": [
    {
      "control": "CC6.1",
      "requirement": "Logical and physical access controls",
      "status": "pass",
      "details": "5000 authentication events logged"
    }
  ],
  "generatedAt": "2026-02-10T12:00:00Z"
}
```

### Verification

#### POST /v1/verify
Verify event integrity using checksum.

**Request:**
```json
{
  "eventId": "evt_abc123"
}
```

**Response:**
```json
{
  "eventId": "evt_abc123",
  "valid": true,
  "expectedChecksum": "a1b2c3d4e5f6...",
  "actualChecksum": "a1b2c3d4e5f6...",
  "message": "Event integrity verified"
}
```

---

## Database Schema

### audit_events (Immutable)

Primary audit event table with immutability triggers.

```sql
CREATE TABLE audit_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  source_plugin VARCHAR(128) NOT NULL,
  event_type VARCHAR(255) NOT NULL,
  actor_id VARCHAR(255),
  actor_type VARCHAR(128),
  resource_type VARCHAR(128),
  resource_id VARCHAR(255),
  action VARCHAR(255) NOT NULL,
  outcome VARCHAR(20) NOT NULL DEFAULT 'success',
  severity VARCHAR(20) NOT NULL DEFAULT 'low',
  ip_address VARCHAR(45),
  user_agent TEXT,
  location VARCHAR(255),
  details JSONB DEFAULT '{}',
  metadata JSONB DEFAULT '{}',
  checksum VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Immutability triggers prevent UPDATE and DELETE
CREATE TRIGGER audit_events_prevent_update
BEFORE UPDATE ON audit_events
FOR EACH ROW EXECUTE FUNCTION audit_prevent_modifications();

CREATE TRIGGER audit_events_prevent_delete
BEFORE DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION audit_prevent_modifications();
```

### audit_retention_policies

Retention policy definitions.

```sql
CREATE TABLE audit_retention_policies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  event_type_pattern VARCHAR(255) NOT NULL,
  retention_days INTEGER NOT NULL CHECK (retention_days > 0),
  enabled BOOLEAN DEFAULT true,
  last_executed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(app_id, name)
);
```

### audit_alert_rules

Alert rule definitions.

```sql
CREATE TABLE audit_alert_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  event_type_pattern VARCHAR(255) NOT NULL,
  severity_threshold VARCHAR(20) NOT NULL DEFAULT 'high',
  conditions JSONB DEFAULT '{}',
  webhook_url TEXT,
  enabled BOOLEAN DEFAULT true,
  last_triggered_at TIMESTAMPTZ,
  trigger_count INTEGER DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(app_id, name)
);
```

### audit_webhook_events

Outbound webhook event log.

```sql
CREATE TABLE audit_webhook_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(255) NOT NULL,
  payload JSONB NOT NULL,
  delivered BOOLEAN DEFAULT false,
  delivered_at TIMESTAMPTZ,
  delivery_attempts INTEGER DEFAULT 0,
  last_error TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Event Envelope

All audit events follow this standard structure:

```typescript
{
  sourcePlugin: string;        // Plugin that emitted the event (e.g., "auth", "stripe")
  eventType: string;            // Event type (e.g., "auth.login.success")
  actorId?: string;             // Who performed the action (user ID, API key, etc.)
  actorType?: string;           // Type of actor (user, service, system)
  resourceType?: string;        // What was acted upon (user, order, file)
  resourceId?: string;          // ID of the resource
  action: string;               // Action performed (create, update, delete, access)
  outcome?: 'success' | 'failure' | 'unknown';  // Result
  severity?: 'low' | 'medium' | 'high' | 'critical';  // Severity level
  ipAddress?: string;           // Source IP address
  userAgent?: string;           // User agent string
  location?: string;            // Geographic location
  details?: Record<string, unknown>;  // Additional structured data
  metadata?: Record<string, unknown>; // Plugin-specific metadata
}
```

---

## SIEM Integration

### Splunk HEC

```bash
# Configure
export AUDIT_SIEM_SPLUNK_HEC_URL="https://splunk.example.com:8088/services/collector/event"
export AUDIT_SIEM_SPLUNK_HEC_TOKEN="your-hec-token"

# Events are automatically forwarded
# Or export manually
npm run cli -- export --format cef --output splunk.cef
```

### Elasticsearch/ELK

```bash
# Configure
export AUDIT_SIEM_ELK_URL="https://elasticsearch.example.com:9200"
export AUDIT_SIEM_ELK_INDEX="audit-logs"
export AUDIT_SIEM_ELK_API_KEY="your-api-key"

# Events are automatically forwarded
```

### Datadog

```bash
# Configure
export AUDIT_SIEM_DATADOG_API_KEY="your-datadog-api-key"
export AUDIT_SIEM_DATADOG_SITE="datadoghq.com"

# Events are automatically forwarded
```

---

## Compliance Frameworks

### SOC 2

Checks:
- **CC6.1:** Logical and physical access controls
- **CC7.2:** System monitoring for security incidents
- **CC7.3:** Evaluation and response to security incidents

### HIPAA

Checks:
- **164.312(b):** Audit controls implementation
- **164.308(a)(1)(ii)(D):** Regular review of information system activity
- **164.312(a)(2)(i):** Unique user identification

### GDPR

Checks:
- **Article 30:** Records of processing activities
- **Article 32:** Security of processing
- **Article 33:** Notification of personal data breach

### PCI DSS

Checks:
- **10.1:** Implement audit trails to link access to system components
- **10.2:** Automated audit trails for critical security events
- **10.3:** Record required audit trail entries

---

## Multi-App Support

The audit plugin supports multi-app isolation using the `app_id` column:

```bash
# Configure multiple apps
export AUDIT_APP_IDS="primary,production,staging"

# Log event for specific app
curl -X POST http://localhost:3303/v1/events \
  -H "X-App-Id: production" \
  -H "Content-Type: application/json" \
  -d '{...}'

# Query events for specific app
curl http://localhost:3303/v1/events?limit=10 \
  -H "X-App-Id: production"
```

---

## Security Features

### Immutability

Events cannot be modified or deleted once written:

```sql
-- These operations will fail with an exception
UPDATE audit_events SET ... WHERE ...;  -- ❌ Blocked by trigger
DELETE FROM audit_events WHERE ...;     -- ❌ Blocked by trigger
```

### Cryptographic Checksums

Each event has a SHA-256 checksum computed from all fields:

```bash
# Verify event integrity
npm run cli -- verify --event-id evt_abc123
```

### Fallback Logging

If the database fails, events are written to a fallback log file:

```bash
# Configure fallback path
export AUDIT_FALLBACK_LOG_PATH=/var/log/nself/audit-fallback.jsonl

# Events are written as JSON Lines
tail -f /var/log/nself/audit-fallback.jsonl
```

---

## Troubleshooting

### Immutability Triggers Not Working

Check if triggers are in place:

```sql
SELECT tgname, tgrelid::regclass, tgenabled
FROM pg_trigger
WHERE tgname IN ('audit_events_prevent_update', 'audit_events_prevent_delete');
```

Reinitialize if missing:

```bash
npm run cli -- init
```

### Events Not Forwarding to SIEM

Check configuration:

```bash
# Verify SIEM credentials are set
env | grep AUDIT_SIEM

# Check server logs for errors
npm run dev
```

### Retention Policy Not Deleting Events

Retention execution requires superuser privileges to bypass immutability triggers. Run as a scheduled job with appropriate permissions, or use a temporary `DISABLE TRIGGER` approach.

### Database Connection Issues

Verify PostgreSQL connection:

```bash
# Test connection
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB -c "SELECT 1;"

# Check credentials in .env
cat .env | grep POSTGRES
```

---

## Best Practices

1. **Use Descriptive Event Types** - Follow a namespace pattern: `plugin.resource.action`
   - Example: `auth.login.success`, `stripe.payment.failed`

2. **Set Appropriate Severity** - Use severity levels consistently:
   - **Low:** Normal operations (login, logout, view)
   - **Medium:** Modifications (create, update)
   - **High:** Deletions, sensitive data access
   - **Critical:** Security violations, breaches, admin actions

3. **Include Actor Context** - Always specify `actorId` and `actorType`
   - User actions: `actorId=user123`, `actorType=user`
   - System actions: `actorId=system`, `actorType=system`
   - API actions: `actorId=api_key_xyz`, `actorType=api_key`

4. **Resource Tracking** - Specify `resourceType` and `resourceId` for all actions
   - Example: `resourceType=order`, `resourceId=ord_abc123`

5. **Structured Details** - Use the `details` field for structured data, not free-form text

6. **Regular Verification** - Periodically verify event checksums to detect tampering

7. **Retention Policies** - Configure retention based on compliance requirements:
   - SOC2: 1 year minimum
   - HIPAA: 6 years
   - GDPR: Data retention policies apply
   - PCI: 1 year minimum

8. **Alert Rules** - Create alert rules for critical events:
   - Failed authentication attempts (rate limiting)
   - Critical security events
   - Compliance violations

---

## Production Deployment

### Docker Compose

```yaml
version: '3.8'
services:
  audit:
    build: ./plugins/audit/ts
    ports:
      - "3303:3303"
    environment:
      - POSTGRES_HOST=postgres
      - POSTGRES_DB=nself
      - POSTGRES_USER=nself
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - AUDIT_SIEM_SPLUNK_HEC_URL=${SPLUNK_HEC_URL}
      - AUDIT_SIEM_SPLUNK_HEC_TOKEN=${SPLUNK_HEC_TOKEN}
    depends_on:
      - postgres
    restart: unless-stopped
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: audit-plugin
spec:
  replicas: 3
  selector:
    matchLabels:
      app: audit
  template:
    metadata:
      labels:
        app: audit
    spec:
      containers:
      - name: audit
        image: nself/audit-plugin:1.0.0
        ports:
        - containerPort: 3303
        env:
        - name: POSTGRES_HOST
          value: postgres
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: password
        livenessProbe:
          httpGet:
            path: /live
            port: 3303
        readinessProbe:
          httpGet:
            path: /ready
            port: 3303
```

---

## License

Source-Available License
