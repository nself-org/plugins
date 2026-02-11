# Compliance

GDPR/CCPA compliance management with DSARs, consent tracking, data retention, breach notification, and audit trails.

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Database Schema](#database-schema)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

The Compliance plugin provides comprehensive GDPR and CCPA compliance management for nself applications. It handles Data Subject Access Requests (DSARs), consent management, privacy policy versioning, data retention policies, breach notifications, and maintains a complete audit trail of all compliance-related activities.

This plugin is essential for any application that handles EU or California user data, providing the tools needed to meet regulatory requirements while maintaining detailed records for regulatory audits.

### Key Features

- **GDPR & CCPA Support**: Full support for both GDPR (30-day DSARs) and CCPA (45-day requests)
- **Data Subject Access Requests (DSARs)**: Complete lifecycle management from submission through verification, processing, and fulfillment
- **Consent Management**: Track and manage user consent with expiration, withdrawal, and history tracking
- **Privacy Policy Versioning**: Maintain multiple policy versions with re-acceptance tracking
- **Data Retention Policies**: Automated deletion, anonymization, or archival based on configurable retention rules
- **Breach Management**: 72-hour breach notification tracking with authority and user notifications
- **Processing Records**: GDPR Article 30 compliance with full processing activity documentation
- **Data Processor Tracking**: Maintain records of third-party data processors and DPAs
- **Comprehensive Audit Log**: Immutable audit trail of all compliance activities
- **Data Export**: Generate complete user data packages in JSON or CSV format
- **Multi-Account Isolation**: Full support for multi-tenant applications

### Supported Regulations

- **GDPR** (General Data Protection Regulation)
  - Right to access (Article 15)
  - Right to erasure (Article 17)
  - Right to rectification (Article 16)
  - Right to data portability (Article 20)
  - Right to restriction of processing (Article 18)
  - Right to object (Article 21)
  - Breach notification (Article 33-34)
  - Processing records (Article 30)

- **CCPA** (California Consumer Privacy Act)
  - Right to know (CCPA Disclosure)
  - Right to delete (CCPA Deletion)
  - Right to opt-out of sale (CCPA Opt-Out)

### Use Cases

1. **SaaS Applications**: Meet GDPR/CCPA requirements for user data handling
2. **Healthcare Platforms**: Manage patient data with full audit trails
3. **E-commerce Sites**: Track consent for marketing and data processing
4. **Financial Services**: Maintain regulatory compliance records
5. **Educational Platforms**: Manage student data privacy requirements

## Quick Start

```bash
# Install the plugin
nself plugin install compliance

# Set environment variables
export DATABASE_URL="postgresql://user:pass@localhost:5432/mydb"
export COMPLIANCE_PLUGIN_PORT=3706

# Initialize database schema
nself plugin compliance init

# Start the compliance server
nself plugin compliance server

# Check status
nself plugin compliance status
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `COMPLIANCE_PLUGIN_PORT` | No | `3706` | HTTP server port |
| `COMPLIANCE_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `nself` | PostgreSQL database name |
| `POSTGRES_USER` | No | `postgres` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | ` ` (empty) | PostgreSQL password |
| `POSTGRES_SSL` | No | `false` | Enable SSL for PostgreSQL |
| `COMPLIANCE_GDPR_ENABLED` | No | `true` | Enable GDPR compliance features |
| `COMPLIANCE_CCPA_ENABLED` | No | `true` | Enable CCPA compliance features |
| `COMPLIANCE_DSAR_DEADLINE_DAYS` | No | `30` | GDPR DSAR response deadline (days) |
| `COMPLIANCE_DSAR_AUTO_VERIFICATION` | No | `false` | Automatically verify DSARs without manual approval |
| `COMPLIANCE_CCPA_DEADLINE_DAYS` | No | `45` | CCPA request response deadline (days) |
| `COMPLIANCE_BREACH_NOTIFICATION_HOURS` | No | `72` | Hours until breach notification deadline |
| `COMPLIANCE_CONSENT_REQUIRED` | No | `true` | Require explicit consent for data processing |
| `COMPLIANCE_CONSENT_EXPIRY_DAYS` | No | `365` | Days until consent expires |
| `COMPLIANCE_CONSENT_METHOD` | No | `explicit` | Default consent method (explicit, implicit, opt_in, opt_out) |
| `COMPLIANCE_RETENTION_ENABLED` | No | `true` | Enable data retention policy execution |
| `COMPLIANCE_RETENTION_GRACE_PERIOD_DAYS` | No | `7` | Grace period before retention execution |
| `COMPLIANCE_NOTIFY_DSAR_ASSIGNED` | No | `true` | Send notifications when DSAR is assigned |
| `COMPLIANCE_NOTIFY_DSAR_DEADLINE_DAYS` | No | `3` | Send deadline reminder N days before deadline |
| `COMPLIANCE_NOTIFY_POLICY_UPDATES` | No | `true` | Notify users of privacy policy updates |
| `COMPLIANCE_EXPORT_FORMAT` | No | `json` | Default export format (json, csv) |
| `COMPLIANCE_EXPORT_ENCRYPTION` | No | `true` | Encrypt data exports |
| `COMPLIANCE_EXPORT_EXPIRY_HOURS` | No | `72` | Hours until export package expires |
| `COMPLIANCE_AUDIT_ENABLED` | No | `true` | Enable compliance audit logging |
| `COMPLIANCE_AUDIT_RETENTION_DAYS` | No | `2555` | Days to retain audit logs (7 years) |
| `COMPLIANCE_API_KEY` | No | - | API key for authenticated requests |
| `COMPLIANCE_RATE_LIMIT_MAX` | No | `100` | Maximum requests per window |
| `COMPLIANCE_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |
| `LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |

### Example .env

```bash
# Database Configuration
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
POSTGRES_SSL=false

# Server Configuration
COMPLIANCE_PLUGIN_PORT=3706
COMPLIANCE_PLUGIN_HOST=0.0.0.0

# GDPR Configuration
COMPLIANCE_GDPR_ENABLED=true
COMPLIANCE_DSAR_DEADLINE_DAYS=30
COMPLIANCE_DSAR_AUTO_VERIFICATION=false
COMPLIANCE_BREACH_NOTIFICATION_HOURS=72

# CCPA Configuration
COMPLIANCE_CCPA_ENABLED=true
COMPLIANCE_CCPA_DEADLINE_DAYS=45

# Consent Management
COMPLIANCE_CONSENT_REQUIRED=true
COMPLIANCE_CONSENT_EXPIRY_DAYS=365
COMPLIANCE_CONSENT_METHOD=explicit

# Data Retention
COMPLIANCE_RETENTION_ENABLED=true
COMPLIANCE_RETENTION_GRACE_PERIOD_DAYS=7

# Notifications
COMPLIANCE_NOTIFY_DSAR_ASSIGNED=true
COMPLIANCE_NOTIFY_DSAR_DEADLINE_DAYS=3
COMPLIANCE_NOTIFY_POLICY_UPDATES=true

# Data Export
COMPLIANCE_EXPORT_FORMAT=json
COMPLIANCE_EXPORT_ENCRYPTION=true
COMPLIANCE_EXPORT_EXPIRY_HOURS=72

# Audit Logging
COMPLIANCE_AUDIT_ENABLED=true
COMPLIANCE_AUDIT_RETENTION_DAYS=2555  # 7 years

# Security
COMPLIANCE_API_KEY=your-secret-api-key-here
COMPLIANCE_RATE_LIMIT_MAX=100
COMPLIANCE_RATE_LIMIT_WINDOW_MS=60000

# Logging
LOG_LEVEL=info
```

## CLI Commands

### Global Commands

#### `init`
Initialize the compliance plugin database schema.

```bash
nself plugin compliance init
```

Creates all required tables, indexes, and constraints.

#### `server`
Start the compliance plugin HTTP server.

```bash
nself plugin compliance server
nself plugin compliance server --port 3706
```

**Options:**
- `-p, --port <port>` - Server port (default: 3706)

#### `status`
Display current compliance plugin status and statistics.

```bash
nself plugin compliance status
```

Shows configuration, DSAR counts, retention policies, breach status, and active privacy policy.

### DSAR Commands

#### `dsar create`
Create a new Data Subject Access Request.

```bash
nself plugin compliance dsar create \
  --email user@example.com \
  --type access \
  --name "John Doe" \
  --user-id user_123 \
  --description "Request for all personal data" \
  --categories "profile,orders,messages"
```

**Options:**
- `-e, --email <email>` - Requester email (required)
- `-t, --type <type>` - Request type (required):
  - `access` - GDPR Article 15
  - `erasure` - GDPR Article 17
  - `portability` - GDPR Article 20
  - `rectification` - GDPR Article 16
  - `restriction` - GDPR Article 18
  - `objection` - GDPR Article 21
  - `ccpa_disclosure` - CCPA Right to Know
  - `ccpa_deletion` - CCPA Right to Delete
  - `ccpa_opt_out` - CCPA Opt-Out of Sale
- `-n, --name <name>` - Requester name
- `-u, --user-id <userId>` - Associated user ID
- `-d, --description <description>` - Request description
- `-c, --categories <categories>` - Comma-separated data categories

#### `dsar list`
List all DSARs with optional filtering.

```bash
nself plugin compliance dsar list
nself plugin compliance dsar list --status pending
nself plugin compliance dsar list --user-id user_123 --limit 10
```

**Options:**
- `-s, --status <status>` - Filter by status (pending, in_progress, approved, rejected, completed)
- `-u, --user-id <userId>` - Filter by user ID
- `-l, --limit <limit>` - Limit results (default: 50)

#### `dsar process`
Approve or reject a DSAR.

```bash
nself plugin compliance dsar process \
  --id dsar-uuid \
  --action approve \
  --notes "Identity verified via government ID" \
  --assign-to admin_user_123
```

**Options:**
- `-i, --id <id>` - DSAR ID (required)
- `-a, --action <action>` - Action: approve or reject (required)
- `-n, --notes <notes>` - Resolution notes
- `-r, --reason <reason>` - Rejection reason (for reject action)
- `--assign-to <assignTo>` - Assign to user ID

#### `dsar complete`
Mark a DSAR as completed with optional data package URL.

```bash
nself plugin compliance dsar complete \
  --id dsar-uuid \
  --url https://s3.amazonaws.com/exports/user_123_data.json
```

**Options:**
- `-i, --id <id>` - DSAR ID (required)
- `-u, --url <url>` - Data package URL

#### `dsar export`
Export user data for a DSAR.

```bash
nself plugin compliance dsar export --id dsar-uuid --format json
```

**Options:**
- `-i, --id <id>` - DSAR ID (required)
- `-f, --format <format>` - Export format (json, csv) (default: json)

### Consent Commands

#### `consent grant`
Grant consent for a user.

```bash
nself plugin compliance consent grant \
  --user-id user_123 \
  --purpose marketing_emails \
  --description "Receive promotional emails" \
  --method explicit \
  --policy-version 2.0.0 \
  --expires 2025-12-31T23:59:59Z
```

**Options:**
- `-u, --user-id <userId>` - User ID (required)
- `-p, --purpose <purpose>` - Consent purpose (required)
- `-d, --description <description>` - Purpose description
- `-t, --text <text>` - Consent text
- `-m, --method <method>` - Consent method (explicit, implicit, opt_in, opt_out)
- `-v, --policy-version <version>` - Privacy policy version
- `-e, --expires <expiresAt>` - Expiry date (ISO format)

#### `consent withdraw`
Withdraw a consent.

```bash
nself plugin compliance consent withdraw \
  --id consent-uuid \
  --reason "User requested withdrawal"
```

**Options:**
- `-i, --id <id>` - Consent ID (required)
- `-r, --reason <reason>` - Withdrawal reason

#### `consent list`
List consent records.

```bash
nself plugin compliance consent list
nself plugin compliance consent list --user-id user_123
nself plugin compliance consent list --purpose marketing_emails
```

**Options:**
- `-u, --user-id <userId>` - Filter by user ID
- `-p, --purpose <purpose>` - Filter by purpose

#### `consent check`
Check if a user has valid consent for a purpose.

```bash
nself plugin compliance consent check \
  --user-id user_123 \
  --purpose marketing_emails
```

**Options:**
- `-u, --user-id <userId>` - User ID (required)
- `-p, --purpose <purpose>` - Consent purpose (required)

Returns exit code 0 if consent is valid, 1 otherwise.

### Retention Commands

#### `retention create`
Create a data retention policy.

```bash
nself plugin compliance retention create \
  --name "Delete old user logs" \
  --category user_logs \
  --days 90 \
  --action delete \
  --description "Remove user activity logs after 90 days" \
  --table user_activity_logs \
  --legal-basis "Legitimate interest - system security" \
  --regulation GDPR
```

**Options:**
- `-n, --name <name>` - Policy name (required)
- `-c, --category <category>` - Data category (required)
- `-d, --days <days>` - Retention period in days (required)
- `-a, --action <action>` - Retention action (required): delete, anonymize, archive, notify
- `--description <description>` - Policy description
- `--table <table>` - Target table name
- `--legal-basis <basis>` - Legal basis for processing
- `--regulation <regulation>` - Regulation (GDPR, CCPA)

#### `retention list`
List retention policies.

```bash
nself plugin compliance retention list
nself plugin compliance retention list --enabled-only
```

**Options:**
- `-e, --enabled-only` - Show only enabled policies

#### `retention execute`
Execute a retention policy manually.

```bash
nself plugin compliance retention execute --id policy-uuid
```

**Options:**
- `-i, --id <policyId>` - Policy ID (required)

#### `retention report`
Show retention execution report.

```bash
nself plugin compliance retention report
nself plugin compliance retention report --id policy-uuid --limit 20
```

**Options:**
- `-i, --id <policyId>` - Policy ID (show all if not specified)
- `-l, --limit <limit>` - Limit results (default: 20)

### Breach Commands

#### `breach create`
Report a new data breach.

```bash
nself plugin compliance breach create \
  --title "Database access breach" \
  --description "Unauthorized access to customer database detected" \
  --severity high \
  --categories "email,name,phone" \
  --discovered-by admin_123 \
  --affected-users 1500 \
  --data-description "Customer contact information exposed"
```

**Options:**
- `-t, --title <title>` - Breach title (required)
- `-d, --description <description>` - Breach description (required)
- `-s, --severity <severity>` - Severity (required): low, medium, high, critical
- `-c, --categories <categories>` - Comma-separated data categories (required)
- `--discovered-by <discoveredBy>` - Discovered by user ID
- `--affected-users <count>` - Number of affected users
- `--data-description <desc>` - Description of data involved
- `--no-notification` - Notification not required

#### `breach list`
List data breaches.

```bash
nself plugin compliance breach list
nself plugin compliance breach list --status investigating
nself plugin compliance breach list --severity high
```

**Options:**
- `-s, --status <status>` - Filter by status (investigating, contained, notified, resolved)
- `--severity <severity>` - Filter by severity (low, medium, high, critical)

#### `breach notify`
Send breach notification.

```bash
nself plugin compliance breach notify \
  --id breach-uuid \
  --type authority \
  --recipient-type supervisory_authority \
  --email dpo@data-authority.gov \
  --subject "Data Breach Notification" \
  --message "We are notifying you of a data breach that occurred on..."
```

**Options:**
- `-i, --id <id>` - Breach ID (required)
- `-t, --type <type>` - Notification type (required): authority, user, media
- `--recipient-type <recipientType>` - Recipient type (required)
- `-e, --email <email>` - Recipient email
- `-s, --subject <subject>` - Notification subject
- `-m, --message <message>` - Notification message

### Policy Commands

#### `policy create`
Create a new privacy policy version.

```bash
nself plugin compliance policy create \
  --version 2.0.0 \
  --version-number 2 \
  --title "Privacy Policy v2.0" \
  --content "$(cat privacy_policy_v2.txt)" \
  --effective-from 2024-03-01T00:00:00Z \
  --summary "Updated to include new AI features" \
  --changes "Added section on AI data processing" \
  --reacceptance \
  --language en \
  --jurisdiction "EU"
```

**Options:**
- `-v, --version <version>` - Policy version (e.g., 2.0.0) (required)
- `-n, --version-number <number>` - Version number (integer) (required)
- `-t, --title <title>` - Policy title (required)
- `-c, --content <content>` - Policy content (required)
- `-e, --effective-from <date>` - Effective from date (ISO format) (required)
- `-s, --summary <summary>` - Policy summary
- `--changes <changes>` - Changes summary from previous version
- `-r, --reacceptance` - Requires re-acceptance
- `-l, --language <language>` - Language code (default: en)
- `-j, --jurisdiction <jurisdiction>` - Jurisdiction

#### `policy publish`
Publish a privacy policy (makes it active).

```bash
nself plugin compliance policy publish --id policy-uuid
```

**Options:**
- `-i, --id <id>` - Policy ID (required)

#### `policy current`
Show current active privacy policy.

```bash
nself plugin compliance policy current
```

### Audit Commands

#### `audit list`
List compliance audit log entries.

```bash
nself plugin compliance audit list
nself plugin compliance audit list --category dsar --limit 100
nself plugin compliance audit list --actor admin_123
nself plugin compliance audit list --subject user_456
```

**Options:**
- `-c, --category <category>` - Filter by event category (dsar, consent, retention, breach, policy, webhook)
- `-a, --actor <actorId>` - Filter by actor ID
- `-s, --subject <subjectId>` - Filter by data subject ID
- `-l, --limit <limit>` - Limit results (default: 50)

#### `audit export`
Export audit logs as JSON.

```bash
nself plugin compliance audit export --category dsar --limit 1000 > audit_export.json
```

**Options:**
- `-c, --category <category>` - Filter by event category
- `-a, --actor <actorId>` - Filter by actor ID
- `-s, --subject <subjectId>` - Filter by data subject ID
- `-l, --limit <limit>` - Limit results (default: 1000)

### Export Command

#### `export`
Export user data for compliance purposes.

```bash
nself plugin compliance export --user-id user_123
nself plugin compliance export --user-id user_123 --categories "profile,orders" --format json
```

**Options:**
- `-u, --user-id <userId>` - User ID (required)
- `-c, --categories <categories>` - Comma-separated data categories
- `-f, --format <format>` - Export format (json, csv) (default: json)

## REST API

### Health Check Endpoints

#### `GET /health`
Basic health check.

**Response:**
```json
{
  "status": "ok",
  "plugin": "compliance",
  "timestamp": "2024-02-11T10:00:00Z"
}
```

#### `GET /ready`
Readiness check (includes database connectivity).

**Response:**
```json
{
  "ready": true,
  "plugin": "compliance",
  "timestamp": "2024-02-11T10:00:00Z"
}
```

#### `GET /live`
Liveness check with detailed status.

**Response:**
```json
{
  "alive": true,
  "plugin": "compliance",
  "version": "1.0.0",
  "uptime": 3600.5,
  "memory": {
    "rss": 52428800,
    "heapTotal": 20971520,
    "heapUsed": 15728640,
    "external": 1048576
  },
  "config": {
    "gdprEnabled": true,
    "ccpaEnabled": true,
    "retentionEnabled": true,
    "auditEnabled": true
  },
  "stats": {
    "totalDsars": 42
  },
  "timestamp": "2024-02-11T10:00:00Z"
}
```

### Status Endpoint

#### `GET /v1/status`
Get compliance plugin status with statistics.

**Response:**
```json
{
  "plugin": "compliance",
  "version": "1.0.0",
  "status": "running",
  "config": {
    "gdprEnabled": true,
    "ccpaEnabled": true,
    "dsarDeadlineDays": 30,
    "breachNotificationHours": 72,
    "retentionEnabled": true,
    "auditEnabled": true
  },
  "stats": {
    "totalDsars": 42,
    "retentionPolicies": 5,
    "activeBreaches": 0
  },
  "timestamp": "2024-02-11T10:00:00Z"
}
```

### DSAR Endpoints

#### `POST /api/compliance/dsars`
Create a new DSAR.

**Request:**
```json
{
  "request_type": "access",
  "email": "user@example.com",
  "name": "John Doe",
  "user_id": "user_123",
  "description": "Request for all personal data",
  "data_categories": ["profile", "orders", "messages"],
  "specific_data_requested": "All data related to my account",
  "regulation": "GDPR",
  "jurisdiction": "EU"
}
```

**Response (201):**
```json
{
  "dsar_id": "550e8400-e29b-41d4-a716-446655440000",
  "request_number": "DSAR-2024-00042",
  "status": "pending",
  "deadline": "2024-03-13T10:00:00Z",
  "verification_required": true,
  "created_at": "2024-02-11T10:00:00Z"
}
```

#### `GET /api/compliance/dsars`
List DSARs with optional filtering.

**Query Parameters:**
- `status` - Filter by status
- `user_id` - Filter by user ID
- `limit` - Limit results (default: 50)
- `offset` - Offset for pagination (default: 0)

**Response:**
```json
{
  "dsars": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "source_account_id": "primary",
      "request_type": "access",
      "request_number": "DSAR-2024-00042",
      "user_id": "user_123",
      "requester_email": "user@example.com",
      "requester_name": "John Doe",
      "status": "pending",
      "deadline": "2024-03-13T10:00:00Z",
      "regulation": "GDPR",
      "created_at": "2024-02-11T10:00:00Z",
      "updated_at": "2024-02-11T10:00:00Z"
    }
  ],
  "total": 42,
  "limit": 50,
  "offset": 0
}
```

#### `GET /api/compliance/dsars/:id`
Get a specific DSAR with activities.

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source_account_id": "primary",
  "request_type": "access",
  "request_number": "DSAR-2024-00042",
  "user_id": "user_123",
  "requester_email": "user@example.com",
  "requester_name": "John Doe",
  "status": "in_progress",
  "assigned_to": "admin_123",
  "deadline": "2024-03-13T10:00:00Z",
  "regulation": "GDPR",
  "created_at": "2024-02-11T10:00:00Z",
  "updated_at": "2024-02-11T11:00:00Z",
  "activities": [
    {
      "id": "activity-uuid",
      "activity_type": "created",
      "description": "DSAR request created",
      "performed_by": null,
      "created_at": "2024-02-11T10:00:00Z"
    },
    {
      "id": "activity-uuid-2",
      "activity_type": "verified",
      "description": "Identity verification completed",
      "performed_by": "admin_123",
      "created_at": "2024-02-11T11:00:00Z"
    }
  ]
}
```

#### `POST /api/compliance/dsars/:id/verify`
Verify DSAR identity.

**Request:**
```json
{
  "verification_token": "abc123xyz789"
}
```

**Response:**
```json
{
  "verified": true
}
```

#### `POST /api/compliance/dsars/:id/process`
Approve or reject a DSAR.

**Request:**
```json
{
  "action": "approve",
  "notes": "Identity verified via government ID",
  "assigned_to": "admin_123"
}
```

**Response:**
```json
{
  "success": true,
  "dsar": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "approved",
    "assigned_to": "admin_123",
    "resolution_notes": "Identity verified via government ID",
    "updated_at": "2024-02-11T11:00:00Z"
  }
}
```

#### `POST /api/compliance/dsars/:id/complete`
Complete a DSAR.

**Request:**
```json
{
  "data_package_url": "https://s3.amazonaws.com/exports/user_123_data.json"
}
```

**Response:**
```json
{
  "success": true,
  "dsar": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "completed",
    "completed_at": "2024-02-11T12:00:00Z",
    "data_package_url": "https://s3.amazonaws.com/exports/user_123_data.json",
    "data_package_generated_at": "2024-02-11T12:00:00Z"
  }
}
```

#### `GET /api/compliance/dsars/:id/activities`
Get DSAR activity log.

**Response:**
```json
{
  "activities": [
    {
      "id": "activity-uuid",
      "dsar_id": "550e8400-e29b-41d4-a716-446655440000",
      "activity_type": "created",
      "description": "DSAR request created",
      "performed_by": null,
      "created_at": "2024-02-11T10:00:00Z"
    }
  ],
  "count": 1
}
```

### Consent Endpoints

#### `POST /api/compliance/consent`
Create or update consent.

**Request:**
```json
{
  "user_id": "user_123",
  "purpose": "marketing_emails",
  "purpose_description": "Receive promotional emails",
  "status": "granted",
  "consent_method": "explicit",
  "consent_text": "I agree to receive marketing emails",
  "privacy_policy_version": "2.0.0",
  "expires_at": "2025-12-31T23:59:59Z"
}
```

**Response (201):**
```json
{
  "consent_id": "consent-uuid",
  "user_id": "user_123",
  "purpose": "marketing_emails",
  "status": "granted",
  "granted_at": "2024-02-11T10:00:00Z",
  "expires_at": "2025-12-31T23:59:59Z"
}
```

#### `GET /api/compliance/consent`
List consents.

**Query Parameters:**
- `user_id` - Filter by user ID
- `purpose` - Filter by purpose

**Response:**
```json
{
  "consents": [
    {
      "id": "consent-uuid",
      "user_id": "user_123",
      "purpose": "marketing_emails",
      "status": "granted",
      "granted_at": "2024-02-11T10:00:00Z",
      "expires_at": "2025-12-31T23:59:59Z"
    }
  ],
  "count": 1
}
```

#### `GET /api/compliance/consent/:id`
Get specific consent.

**Response:**
```json
{
  "id": "consent-uuid",
  "user_id": "user_123",
  "purpose": "marketing_emails",
  "purpose_description": "Receive promotional emails",
  "status": "granted",
  "granted_at": "2024-02-11T10:00:00Z",
  "expires_at": "2025-12-31T23:59:59Z",
  "consent_method": "explicit",
  "consent_text": "I agree to receive marketing emails",
  "privacy_policy_version": "2.0.0"
}
```

#### `POST /api/compliance/consent/:id/withdraw`
Withdraw consent.

**Request:**
```json
{
  "reason": "User requested withdrawal"
}
```

**Response:**
```json
{
  "success": true,
  "consent": {
    "id": "consent-uuid",
    "status": "withdrawn",
    "withdrawn_at": "2024-02-11T11:00:00Z"
  }
}
```

#### `GET /api/compliance/consent/check`
Check if user has valid consent.

**Query Parameters:**
- `user_id` - User ID (required)
- `purpose` - Consent purpose (required)

**Response:**
```json
{
  "user_id": "user_123",
  "purpose": "marketing_emails",
  "has_consent": true
}
```

### Privacy Policy Endpoints

#### `GET /api/compliance/privacy-policy`
Get active privacy policy.

**Query Parameters:**
- `version` - Get specific version (optional)

**Response:**
```json
{
  "id": "policy-uuid",
  "version": "2.0.0",
  "version_number": 2,
  "title": "Privacy Policy v2.0",
  "content": "...",
  "summary": "Updated to include new AI features",
  "is_active": true,
  "effective_from": "2024-03-01T00:00:00Z",
  "language": "en",
  "jurisdiction": "EU"
}
```

#### `GET /api/compliance/privacy-policies`
List all privacy policies.

**Response:**
```json
{
  "policies": [
    {
      "id": "policy-uuid",
      "version": "2.0.0",
      "version_number": 2,
      "title": "Privacy Policy v2.0",
      "summary": "Updated to include new AI features",
      "is_active": true,
      "effective_from": "2024-03-01T00:00:00Z",
      "language": "en",
      "created_at": "2024-02-01T10:00:00Z"
    }
  ],
  "count": 1
}
```

#### `POST /api/compliance/privacy-policies`
Create a new privacy policy version.

**Request:**
```json
{
  "version": "2.0.0",
  "version_number": 2,
  "title": "Privacy Policy v2.0",
  "content": "...",
  "summary": "Updated to include new AI features",
  "changes_summary": "Added section on AI data processing",
  "requires_reacceptance": true,
  "effective_from": "2024-03-01T00:00:00Z",
  "language": "en",
  "jurisdiction": "EU"
}
```

**Response (201):**
```json
{
  "id": "policy-uuid",
  "version": "2.0.0",
  "version_number": 2,
  "title": "Privacy Policy v2.0",
  "is_active": false,
  "created_at": "2024-02-01T10:00:00Z"
}
```

#### `POST /api/compliance/privacy-policies/:id/publish`
Publish a privacy policy.

**Response:**
```json
{
  "success": true,
  "policy": {
    "id": "policy-uuid",
    "version": "2.0.0",
    "is_active": true
  }
}
```

#### `POST /api/compliance/privacy-policy/accept`
Accept a privacy policy.

**Request:**
```json
{
  "user_id": "user_123",
  "policy_id": "policy-uuid"
}
```

**Response (201):**
```json
{
  "acceptance_id": "acceptance-uuid",
  "user_id": "user_123",
  "policy_id": "policy-uuid",
  "accepted_at": "2024-02-11T10:00:00Z"
}
```

### Data Retention Endpoints

#### `GET /api/compliance/retention/policies`
List retention policies.

**Query Parameters:**
- `enabled_only` - Show only enabled policies (default: false)

**Response:**
```json
{
  "policies": [
    {
      "id": "policy-uuid",
      "name": "Delete old user logs",
      "data_category": "user_logs",
      "table_name": "user_activity_logs",
      "retention_days": 90,
      "retention_action": "delete",
      "is_enabled": true,
      "priority": 100,
      "legal_basis": "Legitimate interest - system security",
      "regulation": "GDPR"
    }
  ],
  "count": 1
}
```

#### `POST /api/compliance/retention/policies`
Create a retention policy.

**Request:**
```json
{
  "name": "Delete old user logs",
  "description": "Remove user activity logs after 90 days",
  "data_category": "user_logs",
  "table_name": "user_activity_logs",
  "retention_days": 90,
  "retention_action": "delete",
  "conditions": {},
  "legal_basis": "Legitimate interest - system security",
  "regulation": "GDPR"
}
```

**Response (201):**
```json
{
  "id": "policy-uuid",
  "name": "Delete old user logs",
  "data_category": "user_logs",
  "retention_days": 90,
  "retention_action": "delete",
  "is_enabled": true,
  "created_at": "2024-02-11T10:00:00Z"
}
```

#### `POST /api/compliance/retention/execute`
Execute a retention policy.

**Request:**
```json
{
  "policy_id": "policy-uuid"
}
```

**Response:**
```json
{
  "execution_id": "execution-uuid",
  "policy_id": "policy-uuid",
  "status": "completed",
  "records_processed": 150,
  "records_deleted": 150,
  "records_anonymized": 0,
  "records_archived": 0,
  "execution_time_ms": 1250
}
```

#### `GET /api/compliance/retention/executions/:policyId`
Get execution history for a policy.

**Query Parameters:**
- `limit` - Limit results (default: 20)

**Response:**
```json
{
  "executions": [
    {
      "id": "execution-uuid",
      "policy_id": "policy-uuid",
      "executed_at": "2024-02-11T10:00:00Z",
      "records_processed": 150,
      "records_deleted": 150,
      "status": "completed",
      "execution_time_ms": 1250
    }
  ],
  "count": 1
}
```

### Data Processor Endpoints

#### `GET /api/compliance/processors`
List data processors.

**Query Parameters:**
- `active_only` - Show only active processors (default: true)

**Response:**
```json
{
  "processors": [
    {
      "id": "processor-uuid",
      "processor_name": "Email Service Provider Inc.",
      "processor_type": "email",
      "contact_email": "dpo@emailprovider.com",
      "country": "Ireland",
      "is_eu_based": true,
      "dpa_signed": true,
      "dpa_signed_date": "2024-01-01",
      "dpa_expiry_date": "2026-01-01",
      "processing_purposes": ["email_delivery", "bounce_tracking"],
      "data_categories": ["email", "name"],
      "is_active": true
    }
  ],
  "count": 1
}
```

### Data Breach Endpoints

#### `POST /api/compliance/breaches`
Report a new data breach.

**Request:**
```json
{
  "title": "Database access breach",
  "description": "Unauthorized access to customer database detected",
  "severity": "high",
  "data_categories": ["email", "name", "phone"],
  "discovered_by": "admin_123",
  "affected_users_count": 1500,
  "data_description": "Customer contact information exposed",
  "notification_required": true
}
```

**Response (201):**
```json
{
  "breach_id": "breach-uuid",
  "breach_number": "BREACH-2024-00001",
  "severity": "high",
  "status": "investigating",
  "notification_required": true,
  "notification_deadline": "2024-02-14T10:00:00Z",
  "created_at": "2024-02-11T10:00:00Z"
}
```

#### `GET /api/compliance/breaches`
List data breaches.

**Query Parameters:**
- `status` - Filter by status (investigating, contained, notified, resolved)
- `severity` - Filter by severity (low, medium, high, critical)

**Response:**
```json
{
  "breaches": [
    {
      "id": "breach-uuid",
      "breach_number": "BREACH-2024-00001",
      "title": "Database access breach",
      "severity": "high",
      "status": "investigating",
      "affected_users_count": 1500,
      "discovered_at": "2024-02-11T10:00:00Z",
      "notification_deadline": "2024-02-14T10:00:00Z"
    }
  ],
  "count": 1
}
```

#### `GET /api/compliance/breaches/:id`
Get breach details with notifications.

**Response:**
```json
{
  "id": "breach-uuid",
  "breach_number": "BREACH-2024-00001",
  "title": "Database access breach",
  "description": "Unauthorized access to customer database detected",
  "severity": "high",
  "status": "notified",
  "affected_users_count": 1500,
  "data_categories": ["email", "name", "phone"],
  "discovered_at": "2024-02-11T10:00:00Z",
  "authority_notified_at": "2024-02-11T12:00:00Z",
  "users_notified_at": "2024-02-11T13:00:00Z",
  "notification_deadline": "2024-02-14T10:00:00Z",
  "notifications": [
    {
      "id": "notification-uuid",
      "notification_type": "authority",
      "recipient_type": "supervisory_authority",
      "sent_at": "2024-02-11T12:00:00Z",
      "delivery_status": "sent"
    }
  ]
}
```

#### `POST /api/compliance/breaches/:id/notify`
Send breach notification.

**Request:**
```json
{
  "notification_type": "authority",
  "recipient_type": "supervisory_authority",
  "recipient_email": "dpo@data-authority.gov",
  "subject": "Data Breach Notification",
  "message_body": "We are notifying you of a data breach..."
}
```

**Response (201):**
```json
{
  "id": "notification-uuid",
  "breach_id": "breach-uuid",
  "notification_type": "authority",
  "recipient_type": "supervisory_authority",
  "sent_at": "2024-02-11T12:00:00Z",
  "delivery_status": "sent"
}
```

### Data Export Endpoints

#### `POST /api/compliance/export`
Export user data.

**Request:**
```json
{
  "user_id": "user_123",
  "data_categories": ["profile", "orders", "messages"],
  "format": "json"
}
```

**Response:**
```json
{
  "user_id": "user_123",
  "data": {
    "user_id": "user_123",
    "exported_at": "2024-02-11T10:00:00Z",
    "consents": [...],
    "dsars": [...],
    "policy_acceptances": [...]
  },
  "format": "json",
  "exported_at": "2024-02-11T10:00:00Z",
  "expires_at": "2024-02-14T10:00:00Z"
}
```

### Audit Log Endpoints

#### `POST /api/compliance/audit`
Create audit log entry.

**Request:**
```json
{
  "event_type": "data.accessed",
  "event_category": "access",
  "actor_id": "admin_123",
  "actor_type": "user",
  "target_type": "user_profile",
  "target_id": "user_456",
  "accessed_data_categories": ["email", "name"],
  "data_subject_id": "user_456",
  "details": {"reason": "Customer support request"},
  "legal_basis": "Legitimate interest"
}
```

**Response (201):**
```json
{
  "id": "audit-uuid",
  "event_type": "data.accessed",
  "event_category": "access",
  "created_at": "2024-02-11T10:00:00Z"
}
```

#### `GET /api/compliance/audit`
List audit logs.

**Query Parameters:**
- `event_category` - Filter by category
- `actor_id` - Filter by actor
- `data_subject_id` - Filter by data subject
- `limit` - Limit results (default: 50)
- `offset` - Offset for pagination (default: 0)

**Response:**
```json
{
  "logs": [
    {
      "id": "audit-uuid",
      "event_type": "data.accessed",
      "event_category": "access",
      "actor_id": "admin_123",
      "actor_type": "user",
      "target_type": "user_profile",
      "target_id": "user_456",
      "data_subject_id": "user_456",
      "created_at": "2024-02-11T10:00:00Z"
    }
  ],
  "total": 1000,
  "limit": 50,
  "offset": 0
}
```

### Processing Records Endpoints

#### `GET /api/compliance/processing-records`
List processing records (GDPR Article 30).

**Query Parameters:**
- `active_only` - Show only active records (default: true)

**Response:**
```json
{
  "records": [
    {
      "id": "record-uuid",
      "activity_name": "Customer email marketing",
      "processing_purpose": "Direct marketing to customers",
      "legal_basis": "Consent",
      "data_categories": ["email", "name", "preferences"],
      "data_subjects": ["customers"],
      "recipient_categories": ["email_service_provider"],
      "third_party_transfers": true,
      "third_party_countries": ["United States"],
      "safeguards": "Standard contractual clauses",
      "retention_period": "2 years from last contact",
      "is_active": true
    }
  ],
  "count": 1
}
```

#### `POST /api/compliance/processing-records`
Create a processing record.

**Request:**
```json
{
  "activity_name": "Customer email marketing",
  "activity_description": "Sending promotional emails to customers",
  "processing_purpose": "Direct marketing to customers",
  "legal_basis": "Consent",
  "data_categories": ["email", "name", "preferences"],
  "data_subjects": ["customers"],
  "recipient_categories": ["email_service_provider"],
  "third_party_transfers": true,
  "third_party_countries": ["United States"],
  "safeguards": "Standard contractual clauses",
  "retention_period": "2 years from last contact",
  "security_measures": "Encryption at rest and in transit, access controls"
}
```

**Response (201):**
```json
{
  "id": "record-uuid",
  "activity_name": "Customer email marketing",
  "processing_purpose": "Direct marketing to customers",
  "legal_basis": "Consent",
  "is_active": true,
  "created_at": "2024-02-11T10:00:00Z"
}
```

### Webhook Endpoint

#### `POST /webhook`
Receive webhook events from external systems.

**Request:**
```json
{
  "type": "user.deleted",
  "data": {
    "user_id": "user_123",
    "deleted_at": "2024-02-11T10:00:00Z"
  }
}
```

**Response:**
```json
{
  "received": true,
  "type": "user.deleted"
}
```

## Webhook Events

The Compliance plugin emits webhook events for all major compliance activities:

### DSAR Events

| Event | Description | Payload |
|-------|-------------|---------|
| `dsar.created` | New DSAR submitted | `{dsar_id, request_number, request_type, email, deadline}` |
| `dsar.completed` | DSAR processing completed | `{dsar_id, request_number, completed_at, data_package_url}` |
| `dsar.overdue` | DSAR deadline approaching | `{dsar_id, request_number, deadline, days_remaining}` |

### Consent Events

| Event | Description | Payload |
|-------|-------------|---------|
| `consent.granted` | User granted consent | `{consent_id, user_id, purpose, granted_at}` |
| `consent.withdrawn` | User withdrew consent | `{consent_id, user_id, purpose, withdrawn_at, reason}` |

### Policy Events

| Event | Description | Payload |
|-------|-------------|---------|
| `policy.published` | New privacy policy published | `{policy_id, version, effective_from, requires_reacceptance}` |

### Retention Events

| Event | Description | Payload |
|-------|-------------|---------|
| `retention.executed` | Data retention policy executed | `{policy_id, execution_id, records_processed, records_deleted}` |

### Breach Events

| Event | Description | Payload |
|-------|-------------|---------|
| `breach.created` | New data breach recorded | `{breach_id, breach_number, severity, notification_deadline}` |
| `breach.notified` | Breach notification sent | `{breach_id, notification_type, recipient_type, sent_at}` |

## Database Schema

### compliance_dsars

Data Subject Access Requests (GDPR/CCPA).

```sql
CREATE TABLE IF NOT EXISTS compliance_dsars (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  request_type VARCHAR(50) NOT NULL,
  request_number VARCHAR(50) NOT NULL,
  user_id VARCHAR(255),
  requester_email VARCHAR(255) NOT NULL,
  requester_name VARCHAR(255),
  verification_token VARCHAR(255),
  verification_sent_at TIMESTAMP WITH TIME ZONE,
  verification_completed_at TIMESTAMP WITH TIME ZONE,
  verified_by VARCHAR(255),
  description TEXT,
  data_categories TEXT[] DEFAULT '{}',
  specific_data_requested TEXT,
  status VARCHAR(30) NOT NULL DEFAULT 'pending',
  assigned_to VARCHAR(255),
  started_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  deadline TIMESTAMP WITH TIME ZONE NOT NULL,
  data_package_url TEXT,
  data_package_size_bytes BIGINT,
  data_package_generated_at TIMESTAMP WITH TIME ZONE,
  resolution_notes TEXT,
  rejection_reason TEXT,
  regulation VARCHAR(50) NOT NULL DEFAULT 'GDPR',
  jurisdiction VARCHAR(100),
  legal_basis TEXT,
  ip_address VARCHAR(45),
  user_agent TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, request_number)
);

CREATE INDEX IF NOT EXISTS idx_dsars_account ON compliance_dsars(source_account_id);
CREATE INDEX IF NOT EXISTS idx_dsars_user ON compliance_dsars(source_account_id, user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dsars_status ON compliance_dsars(source_account_id, status, deadline);
CREATE INDEX IF NOT EXISTS idx_dsars_assigned ON compliance_dsars(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dsars_number ON compliance_dsars(source_account_id, request_number);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| request_type | VARCHAR(50) | No | - | Type: access, erasure, portability, ccpa_deletion, etc. |
| request_number | VARCHAR(50) | No | - | Unique DSAR number (e.g., DSAR-2024-00042) |
| user_id | VARCHAR(255) | Yes | - | Associated user ID |
| requester_email | VARCHAR(255) | No | - | Email address of requester |
| requester_name | VARCHAR(255) | Yes | - | Name of requester |
| verification_token | VARCHAR(255) | Yes | - | Token for identity verification |
| verification_sent_at | TIMESTAMP WITH TIME ZONE | Yes | - | When verification email was sent |
| verification_completed_at | TIMESTAMP WITH TIME ZONE | Yes | - | When identity was verified |
| verified_by | VARCHAR(255) | Yes | - | User ID who verified identity |
| description | TEXT | Yes | - | Description of request |
| data_categories | TEXT[] | No | {} | Categories of data requested |
| specific_data_requested | TEXT | Yes | - | Specific data items requested |
| status | VARCHAR(30) | No | 'pending' | Status: pending, in_progress, approved, rejected, completed |
| assigned_to | VARCHAR(255) | Yes | - | User ID of assigned handler |
| started_at | TIMESTAMP WITH TIME ZONE | Yes | - | When processing started |
| completed_at | TIMESTAMP WITH TIME ZONE | Yes | - | When processing completed |
| deadline | TIMESTAMP WITH TIME ZONE | No | - | Regulatory deadline (30 days GDPR, 45 days CCPA) |
| data_package_url | TEXT | Yes | - | URL to generated data package |
| data_package_size_bytes | BIGINT | Yes | - | Size of data package in bytes |
| data_package_generated_at | TIMESTAMP WITH TIME ZONE | Yes | - | When data package was generated |
| resolution_notes | TEXT | Yes | - | Notes about resolution |
| rejection_reason | TEXT | Yes | - | Reason for rejection |
| regulation | VARCHAR(50) | No | 'GDPR' | Regulation: GDPR or CCPA |
| jurisdiction | VARCHAR(100) | Yes | - | Jurisdiction (EU, California, etc.) |
| legal_basis | TEXT | Yes | - | Legal basis for processing |
| ip_address | VARCHAR(45) | Yes | - | IP address of requester |
| user_agent | TEXT | Yes | - | User agent of requester |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### compliance_dsar_activities

Activity log for DSAR lifecycle tracking.

```sql
CREATE TABLE IF NOT EXISTS compliance_dsar_activities (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  dsar_id UUID NOT NULL REFERENCES compliance_dsars(id) ON DELETE CASCADE,
  activity_type VARCHAR(100) NOT NULL,
  description TEXT,
  performed_by VARCHAR(255),
  performed_by_name VARCHAR(255),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dsar_activities_account ON compliance_dsar_activities(source_account_id);
CREATE INDEX IF NOT EXISTS idx_dsar_activities_dsar ON compliance_dsar_activities(dsar_id, created_at);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| dsar_id | UUID | No | - | Foreign key to compliance_dsars |
| activity_type | VARCHAR(100) | No | - | Type: created, verified, approved, rejected, completed |
| description | TEXT | Yes | - | Activity description |
| performed_by | VARCHAR(255) | Yes | - | User ID who performed activity |
| performed_by_name | VARCHAR(255) | Yes | - | Name of user who performed activity |
| metadata | JSONB | No | {} | Additional metadata |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Activity timestamp |

### compliance_consents

User consent records for data processing.

```sql
CREATE TABLE IF NOT EXISTS compliance_consents (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  purpose VARCHAR(255) NOT NULL,
  purpose_description TEXT,
  status VARCHAR(20) NOT NULL DEFAULT 'granted',
  granted_at TIMESTAMP WITH TIME ZONE,
  denied_at TIMESTAMP WITH TIME ZONE,
  withdrawn_at TIMESTAMP WITH TIME ZONE,
  expires_at TIMESTAMP WITH TIME ZONE,
  consent_method VARCHAR(100),
  consent_text TEXT,
  privacy_policy_version VARCHAR(50),
  ip_address VARCHAR(45),
  user_agent TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_consents_account ON compliance_consents(source_account_id);
CREATE INDEX IF NOT EXISTS idx_consents_user ON compliance_consents(source_account_id, user_id, purpose);
CREATE INDEX IF NOT EXISTS idx_consents_status ON compliance_consents(source_account_id, status, purpose);
CREATE INDEX IF NOT EXISTS idx_consents_expires ON compliance_consents(expires_at) WHERE expires_at IS NOT NULL;
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| user_id | VARCHAR(255) | No | - | User ID |
| purpose | VARCHAR(255) | No | - | Purpose: marketing_emails, analytics, etc. |
| purpose_description | TEXT | Yes | - | Human-readable purpose description |
| status | VARCHAR(20) | No | 'granted' | Status: granted, denied, withdrawn |
| granted_at | TIMESTAMP WITH TIME ZONE | Yes | - | When consent was granted |
| denied_at | TIMESTAMP WITH TIME ZONE | Yes | - | When consent was denied |
| withdrawn_at | TIMESTAMP WITH TIME ZONE | Yes | - | When consent was withdrawn |
| expires_at | TIMESTAMP WITH TIME ZONE | Yes | - | Expiration timestamp |
| consent_method | VARCHAR(100) | Yes | - | Method: explicit, implicit, opt_in, opt_out |
| consent_text | TEXT | Yes | - | Full consent text shown to user |
| privacy_policy_version | VARCHAR(50) | Yes | - | Privacy policy version at time of consent |
| ip_address | VARCHAR(45) | Yes | - | IP address when consent granted |
| user_agent | TEXT | Yes | - | User agent when consent granted |
| metadata | JSONB | No | {} | Additional metadata |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### compliance_consent_history

Change history for consent records.

```sql
CREATE TABLE IF NOT EXISTS compliance_consent_history (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  consent_id UUID NOT NULL REFERENCES compliance_consents(id) ON DELETE CASCADE,
  previous_status VARCHAR(20),
  new_status VARCHAR(20) NOT NULL,
  change_reason VARCHAR(255),
  changed_by VARCHAR(255),
  ip_address VARCHAR(45),
  user_agent TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_consent_history_account ON compliance_consent_history(source_account_id);
CREATE INDEX IF NOT EXISTS idx_consent_history_consent ON compliance_consent_history(consent_id, created_at);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| consent_id | UUID | No | - | Foreign key to compliance_consents |
| previous_status | VARCHAR(20) | Yes | - | Previous consent status |
| new_status | VARCHAR(20) | No | - | New consent status |
| change_reason | VARCHAR(255) | Yes | - | Reason for status change |
| changed_by | VARCHAR(255) | Yes | - | User ID who changed status |
| ip_address | VARCHAR(45) | Yes | - | IP address of change |
| user_agent | TEXT | Yes | - | User agent of change |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Change timestamp |

### compliance_privacy_policies

Privacy policy versions.

```sql
CREATE TABLE IF NOT EXISTS compliance_privacy_policies (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  version VARCHAR(50) NOT NULL,
  version_number INTEGER NOT NULL,
  title VARCHAR(255) NOT NULL,
  content TEXT NOT NULL,
  summary TEXT,
  changes_summary TEXT,
  is_active BOOLEAN NOT NULL DEFAULT false,
  requires_reacceptance BOOLEAN NOT NULL DEFAULT false,
  effective_from TIMESTAMP WITH TIME ZONE NOT NULL,
  effective_until TIMESTAMP WITH TIME ZONE,
  language VARCHAR(10) NOT NULL DEFAULT 'en',
  jurisdiction VARCHAR(100),
  created_by VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, version)
);

CREATE INDEX IF NOT EXISTS idx_privacy_policies_account ON compliance_privacy_policies(source_account_id);
CREATE INDEX IF NOT EXISTS idx_privacy_policies_active ON compliance_privacy_policies(source_account_id, is_active, effective_from);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| version | VARCHAR(50) | No | - | Version string (e.g., 2.0.0) |
| version_number | INTEGER | No | - | Incrementing version number |
| title | VARCHAR(255) | No | - | Policy title |
| content | TEXT | No | - | Full policy content |
| summary | TEXT | Yes | - | Brief summary of policy |
| changes_summary | TEXT | Yes | - | Summary of changes from previous version |
| is_active | BOOLEAN | No | false | Whether this is the active policy |
| requires_reacceptance | BOOLEAN | No | false | Whether users must re-accept |
| effective_from | TIMESTAMP WITH TIME ZONE | No | - | When policy becomes effective |
| effective_until | TIMESTAMP WITH TIME ZONE | Yes | - | When policy expires |
| language | VARCHAR(10) | No | 'en' | Language code |
| jurisdiction | VARCHAR(100) | Yes | - | Jurisdiction |
| created_by | VARCHAR(255) | Yes | - | User ID who created policy |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |

### compliance_policy_acceptances

User acceptances of privacy policies.

```sql
CREATE TABLE IF NOT EXISTS compliance_policy_acceptances (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  policy_id UUID NOT NULL REFERENCES compliance_privacy_policies(id) ON DELETE CASCADE,
  accepted_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  ip_address VARCHAR(45),
  user_agent TEXT,
  metadata JSONB DEFAULT '{}',
  UNIQUE(source_account_id, user_id, policy_id)
);

CREATE INDEX IF NOT EXISTS idx_policy_acceptances_account ON compliance_policy_acceptances(source_account_id);
CREATE INDEX IF NOT EXISTS idx_policy_acceptances_user ON compliance_policy_acceptances(source_account_id, user_id, accepted_at DESC);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| user_id | VARCHAR(255) | No | - | User ID |
| policy_id | UUID | No | - | Foreign key to compliance_privacy_policies |
| accepted_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Acceptance timestamp |
| ip_address | VARCHAR(45) | Yes | - | IP address at acceptance |
| user_agent | TEXT | Yes | - | User agent at acceptance |
| metadata | JSONB | No | {} | Additional metadata |

### compliance_retention_policies

Data retention policies.

```sql
CREATE TABLE IF NOT EXISTS compliance_retention_policies (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  data_category VARCHAR(50) NOT NULL,
  table_name VARCHAR(255),
  retention_days INTEGER NOT NULL,
  retention_action VARCHAR(20) NOT NULL DEFAULT 'delete',
  conditions JSONB DEFAULT '{}',
  is_enabled BOOLEAN NOT NULL DEFAULT true,
  priority INTEGER NOT NULL DEFAULT 100,
  legal_basis TEXT,
  regulation VARCHAR(50),
  created_by VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_retention_policies_account ON compliance_retention_policies(source_account_id);
CREATE INDEX IF NOT EXISTS idx_retention_policies_enabled ON compliance_retention_policies(source_account_id, is_enabled, priority);
CREATE INDEX IF NOT EXISTS idx_retention_policies_category ON compliance_retention_policies(source_account_id, data_category);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| name | VARCHAR(255) | No | - | Policy name |
| description | TEXT | Yes | - | Policy description |
| data_category | VARCHAR(50) | No | - | Data category |
| table_name | VARCHAR(255) | Yes | - | Target database table |
| retention_days | INTEGER | No | - | Retention period in days |
| retention_action | VARCHAR(20) | No | 'delete' | Action: delete, anonymize, archive, notify |
| conditions | JSONB | No | {} | Additional conditions for execution |
| is_enabled | BOOLEAN | No | true | Whether policy is enabled |
| priority | INTEGER | No | 100 | Execution priority (lower runs first) |
| legal_basis | TEXT | Yes | - | Legal basis for retention policy |
| regulation | VARCHAR(50) | Yes | - | Regulation: GDPR, CCPA |
| created_by | VARCHAR(255) | Yes | - | User ID who created policy |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### compliance_retention_executions

Execution history for retention policies.

```sql
CREATE TABLE IF NOT EXISTS compliance_retention_executions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  policy_id UUID NOT NULL REFERENCES compliance_retention_policies(id) ON DELETE CASCADE,
  executed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  records_processed INTEGER NOT NULL DEFAULT 0,
  records_deleted INTEGER NOT NULL DEFAULT 0,
  records_anonymized INTEGER NOT NULL DEFAULT 0,
  records_archived INTEGER NOT NULL DEFAULT 0,
  status VARCHAR(50) NOT NULL DEFAULT 'completed',
  error_message TEXT,
  execution_time_ms INTEGER,
  metadata JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_retention_executions_account ON compliance_retention_executions(source_account_id);
CREATE INDEX IF NOT EXISTS idx_retention_executions_policy ON compliance_retention_executions(policy_id, executed_at DESC);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| policy_id | UUID | No | - | Foreign key to compliance_retention_policies |
| executed_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Execution timestamp |
| records_processed | INTEGER | No | 0 | Number of records processed |
| records_deleted | INTEGER | No | 0 | Number of records deleted |
| records_anonymized | INTEGER | No | 0 | Number of records anonymized |
| records_archived | INTEGER | No | 0 | Number of records archived |
| status | VARCHAR(50) | No | 'completed' | Status: running, completed, failed |
| error_message | TEXT | Yes | - | Error message if failed |
| execution_time_ms | INTEGER | Yes | - | Execution time in milliseconds |
| metadata | JSONB | No | {} | Additional execution metadata |

### compliance_processing_records

GDPR Article 30 processing records.

```sql
CREATE TABLE IF NOT EXISTS compliance_processing_records (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  activity_name VARCHAR(255) NOT NULL,
  activity_description TEXT,
  processing_purpose TEXT NOT NULL,
  legal_basis VARCHAR(100) NOT NULL,
  data_categories TEXT[] NOT NULL,
  data_subjects TEXT[],
  recipient_categories TEXT[],
  third_party_transfers BOOLEAN NOT NULL DEFAULT false,
  third_party_countries TEXT[],
  safeguards TEXT,
  retention_period VARCHAR(255),
  security_measures TEXT,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_by VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processing_records_account ON compliance_processing_records(source_account_id);
CREATE INDEX IF NOT EXISTS idx_processing_records_active ON compliance_processing_records(source_account_id, is_active);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| activity_name | VARCHAR(255) | No | - | Processing activity name |
| activity_description | TEXT | Yes | - | Activity description |
| processing_purpose | TEXT | No | - | Purpose of processing |
| legal_basis | VARCHAR(100) | No | - | Legal basis: consent, contract, legal_obligation, vital_interest, public_task, legitimate_interest |
| data_categories | TEXT[] | No | - | Categories of data processed |
| data_subjects | TEXT[] | Yes | - | Categories of data subjects |
| recipient_categories | TEXT[] | Yes | - | Categories of recipients |
| third_party_transfers | BOOLEAN | No | false | Whether data is transferred to third parties |
| third_party_countries | TEXT[] | Yes | - | Countries data is transferred to |
| safeguards | TEXT | Yes | - | Safeguards for transfers |
| retention_period | VARCHAR(255) | Yes | - | Retention period description |
| security_measures | TEXT | Yes | - | Security measures in place |
| is_active | BOOLEAN | No | true | Whether activity is active |
| created_by | VARCHAR(255) | Yes | - | User ID who created record |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### compliance_data_processors

Third-party data processor records.

```sql
CREATE TABLE IF NOT EXISTS compliance_data_processors (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  processor_name VARCHAR(255) NOT NULL,
  processor_type VARCHAR(100),
  contact_name VARCHAR(255),
  contact_email VARCHAR(255),
  contact_phone VARCHAR(50),
  country VARCHAR(100),
  is_eu_based BOOLEAN NOT NULL DEFAULT false,
  dpa_signed BOOLEAN NOT NULL DEFAULT false,
  dpa_signed_date DATE,
  dpa_expiry_date DATE,
  dpa_document_url TEXT,
  processing_purposes TEXT[],
  data_categories TEXT[],
  has_privacy_shield BOOLEAN DEFAULT false,
  has_scc BOOLEAN DEFAULT false,
  has_bcr BOOLEAN DEFAULT false,
  security_certifications TEXT[],
  last_security_audit DATE,
  is_active BOOLEAN NOT NULL DEFAULT true,
  notes TEXT,
  metadata JSONB DEFAULT '{}',
  created_by VARCHAR(255),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_data_processors_account ON compliance_data_processors(source_account_id);
CREATE INDEX IF NOT EXISTS idx_data_processors_active ON compliance_data_processors(source_account_id, is_active);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| processor_name | VARCHAR(255) | No | - | Data processor name |
| processor_type | VARCHAR(100) | Yes | - | Type: email, analytics, payment, etc. |
| contact_name | VARCHAR(255) | Yes | - | Contact person name |
| contact_email | VARCHAR(255) | Yes | - | Contact email |
| contact_phone | VARCHAR(50) | Yes | - | Contact phone |
| country | VARCHAR(100) | Yes | - | Country of operation |
| is_eu_based | BOOLEAN | No | false | Whether processor is based in EU |
| dpa_signed | BOOLEAN | No | false | Whether Data Processing Agreement is signed |
| dpa_signed_date | DATE | Yes | - | DPA signing date |
| dpa_expiry_date | DATE | Yes | - | DPA expiry date |
| dpa_document_url | TEXT | Yes | - | URL to DPA document |
| processing_purposes | TEXT[] | Yes | - | Purposes of processing |
| data_categories | TEXT[] | Yes | - | Categories of data processed |
| has_privacy_shield | BOOLEAN | No | false | Has Privacy Shield certification |
| has_scc | BOOLEAN | No | false | Uses Standard Contractual Clauses |
| has_bcr | BOOLEAN | No | false | Has Binding Corporate Rules |
| security_certifications | TEXT[] | Yes | - | Security certifications (ISO 27001, SOC 2, etc.) |
| last_security_audit | DATE | Yes | - | Date of last security audit |
| is_active | BOOLEAN | No | true | Whether processor relationship is active |
| notes | TEXT | Yes | - | Additional notes |
| metadata | JSONB | No | {} | Additional metadata |
| created_by | VARCHAR(255) | Yes | - | User ID who created record |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### compliance_data_breaches

Data breach incidents.

```sql
CREATE TABLE IF NOT EXISTS compliance_data_breaches (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  breach_number VARCHAR(50) NOT NULL,
  title VARCHAR(255) NOT NULL,
  description TEXT NOT NULL,
  discovered_at TIMESTAMP WITH TIME ZONE NOT NULL,
  discovered_by VARCHAR(255),
  severity VARCHAR(20) NOT NULL,
  affected_users_count INTEGER,
  data_categories TEXT[] NOT NULL,
  data_description TEXT,
  risk_assessment TEXT,
  mitigation_steps TEXT,
  notification_required BOOLEAN NOT NULL DEFAULT true,
  authority_notified_at TIMESTAMP WITH TIME ZONE,
  users_notified_at TIMESTAMP WITH TIME ZONE,
  notification_deadline TIMESTAMP WITH TIME ZONE,
  resolved_at TIMESTAMP WITH TIME ZONE,
  resolution_summary TEXT,
  root_cause TEXT,
  preventive_measures TEXT,
  status VARCHAR(50) NOT NULL DEFAULT 'investigating',
  assigned_to VARCHAR(255),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, breach_number)
);

CREATE INDEX IF NOT EXISTS idx_data_breaches_account ON compliance_data_breaches(source_account_id);
CREATE INDEX IF NOT EXISTS idx_data_breaches_status ON compliance_data_breaches(source_account_id, status, discovered_at DESC);
CREATE INDEX IF NOT EXISTS idx_data_breaches_severity ON compliance_data_breaches(source_account_id, severity);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| breach_number | VARCHAR(50) | No | - | Unique breach number (e.g., BREACH-2024-00001) |
| title | VARCHAR(255) | No | - | Breach title |
| description | TEXT | No | - | Breach description |
| discovered_at | TIMESTAMP WITH TIME ZONE | No | - | When breach was discovered |
| discovered_by | VARCHAR(255) | Yes | - | User ID who discovered breach |
| severity | VARCHAR(20) | No | - | Severity: low, medium, high, critical |
| affected_users_count | INTEGER | Yes | - | Number of affected users |
| data_categories | TEXT[] | No | - | Categories of data affected |
| data_description | TEXT | Yes | - | Description of affected data |
| risk_assessment | TEXT | Yes | - | Risk assessment |
| mitigation_steps | TEXT | Yes | - | Steps taken to mitigate |
| notification_required | BOOLEAN | No | true | Whether notification is required |
| authority_notified_at | TIMESTAMP WITH TIME ZONE | Yes | - | When supervisory authority was notified |
| users_notified_at | TIMESTAMP WITH TIME ZONE | Yes | - | When users were notified |
| notification_deadline | TIMESTAMP WITH TIME ZONE | Yes | - | 72-hour notification deadline |
| resolved_at | TIMESTAMP WITH TIME ZONE | Yes | - | When breach was resolved |
| resolution_summary | TEXT | Yes | - | Summary of resolution |
| root_cause | TEXT | Yes | - | Root cause analysis |
| preventive_measures | TEXT | Yes | - | Preventive measures implemented |
| status | VARCHAR(50) | No | 'investigating' | Status: investigating, contained, notified, resolved |
| assigned_to | VARCHAR(255) | Yes | - | User ID of assigned handler |
| metadata | JSONB | No | {} | Additional metadata |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |
| updated_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Last update timestamp |

### compliance_breach_notifications

Breach notification records.

```sql
CREATE TABLE IF NOT EXISTS compliance_breach_notifications (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  breach_id UUID NOT NULL REFERENCES compliance_data_breaches(id) ON DELETE CASCADE,
  notification_type VARCHAR(50) NOT NULL,
  recipient_type VARCHAR(50) NOT NULL,
  recipient_email VARCHAR(255),
  subject VARCHAR(255),
  message_body TEXT,
  sent_at TIMESTAMP WITH TIME ZONE,
  delivery_status VARCHAR(50),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_breach_notifications_account ON compliance_breach_notifications(source_account_id);
CREATE INDEX IF NOT EXISTS idx_breach_notifications_breach ON compliance_breach_notifications(breach_id, sent_at);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| breach_id | UUID | No | - | Foreign key to compliance_data_breaches |
| notification_type | VARCHAR(50) | No | - | Type: authority, user, media |
| recipient_type | VARCHAR(50) | No | - | Recipient type: supervisory_authority, affected_user, media |
| recipient_email | VARCHAR(255) | Yes | - | Recipient email address |
| subject | VARCHAR(255) | Yes | - | Notification subject |
| message_body | TEXT | Yes | - | Notification message body |
| sent_at | TIMESTAMP WITH TIME ZONE | Yes | - | When notification was sent |
| delivery_status | VARCHAR(50) | Yes | - | Delivery status: sent, delivered, bounced, failed |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Creation timestamp |

### compliance_audit_log

Comprehensive audit log for all compliance activities.

```sql
CREATE TABLE IF NOT EXISTS compliance_audit_log (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
  event_type VARCHAR(100) NOT NULL,
  event_category VARCHAR(50) NOT NULL,
  actor_id VARCHAR(255),
  actor_type VARCHAR(50) NOT NULL DEFAULT 'user',
  target_type VARCHAR(50),
  target_id VARCHAR(255),
  accessed_data_categories TEXT[],
  data_subject_id VARCHAR(255),
  details JSONB NOT NULL DEFAULT '{}',
  ip_address VARCHAR(45),
  user_agent TEXT,
  legal_basis VARCHAR(100),
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_compliance_audit_account ON compliance_audit_log(source_account_id);
CREATE INDEX IF NOT EXISTS idx_compliance_audit_event ON compliance_audit_log(source_account_id, event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_compliance_audit_actor ON compliance_audit_log(source_account_id, actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_compliance_audit_subject ON compliance_audit_log(source_account_id, data_subject_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_compliance_audit_created ON compliance_audit_log(source_account_id, created_at DESC);
```

**Columns:**

| Column | Type | Nullable | Default | Description |
|--------|------|----------|---------|-------------|
| id | UUID | No | uuid_generate_v4() | Primary key |
| source_account_id | VARCHAR(128) | No | 'primary' | Multi-account isolation |
| event_type | VARCHAR(100) | No | - | Event type: dsar.created, consent.granted, data.accessed, etc. |
| event_category | VARCHAR(50) | No | - | Category: dsar, consent, retention, breach, policy, access, webhook |
| actor_id | VARCHAR(255) | Yes | - | User ID who performed action |
| actor_type | VARCHAR(50) | No | 'user' | Actor type: user, system, api |
| target_type | VARCHAR(50) | Yes | - | Target type: dsar, consent, policy, breach, etc. |
| target_id | VARCHAR(255) | Yes | - | Target ID |
| accessed_data_categories | TEXT[] | Yes | - | Categories of data accessed |
| data_subject_id | VARCHAR(255) | Yes | - | Data subject (user) ID |
| details | JSONB | No | {} | Event details |
| ip_address | VARCHAR(45) | Yes | - | IP address of actor |
| user_agent | TEXT | Yes | - | User agent of actor |
| legal_basis | VARCHAR(100) | Yes | - | Legal basis for action |
| created_at | TIMESTAMP WITH TIME ZONE | No | NOW() | Event timestamp |

## Examples

### Example 1: Complete DSAR Workflow

```bash
# 1. User submits a DSAR via API
curl -X POST http://localhost:3706/api/compliance/dsars \
  -H "Content-Type: application/json" \
  -d '{
    "request_type": "access",
    "email": "john.doe@example.com",
    "name": "John Doe",
    "user_id": "user_12345",
    "description": "I would like to receive a copy of all my personal data",
    "data_categories": ["profile", "orders", "messages"],
    "regulation": "GDPR"
  }'

# Response:
# {
#   "dsar_id": "550e8400-e29b-41d4-a716-446655440000",
#   "request_number": "DSAR-2024-00042",
#   "status": "pending",
#   "deadline": "2024-03-13T10:00:00Z",
#   "verification_required": true,
#   "created_at": "2024-02-11T10:00:00Z"
# }

# 2. Admin reviews and verifies identity
nself plugin compliance dsar process \
  --id 550e8400-e29b-41d4-a716-446655440000 \
  --action approve \
  --notes "Identity verified via government ID" \
  --assign-to admin_123

# 3. Export user data
nself plugin compliance export \
  --user-id user_12345 \
  --format json > user_12345_data.json

# 4. Upload to S3 and complete DSAR
nself plugin compliance dsar complete \
  --id 550e8400-e29b-41d4-a716-446655440000 \
  --url https://s3.amazonaws.com/exports/user_12345_data.json

# 5. Check audit log
nself plugin compliance audit list \
  --category dsar \
  --subject user_12345 \
  --limit 10
```

### Example 2: Consent Management

```sql
-- Grant marketing consent via SQL
INSERT INTO compliance_consents (
  user_id, purpose, status, consent_method,
  consent_text, privacy_policy_version, expires_at
) VALUES (
  'user_12345',
  'marketing_emails',
  'granted',
  'explicit',
  'I agree to receive promotional emails about new features',
  '2.0.0',
  NOW() + INTERVAL '365 days'
);

-- Check consent programmatically
SELECT EXISTS (
  SELECT 1 FROM compliance_consents
  WHERE user_id = 'user_12345'
    AND purpose = 'marketing_emails'
    AND status = 'granted'
    AND (expires_at IS NULL OR expires_at > NOW())
) AS has_consent;

-- Withdraw consent
UPDATE compliance_consents
SET status = 'withdrawn', withdrawn_at = NOW(), updated_at = NOW()
WHERE user_id = 'user_12345' AND purpose = 'marketing_emails';
```

### Example 3: Data Breach Notification

```bash
# 1. Report breach
nself plugin compliance breach create \
  --title "Database access breach" \
  --description "Unauthorized access to customer database" \
  --severity high \
  --categories "email,name,phone" \
  --affected-users 1500 \
  --data-description "Customer contact information exposed"

# Response: BREACH-2024-00001
# Notification deadline: 2024-02-14T10:00:00Z (72 hours)

# 2. Notify supervisory authority
nself plugin compliance breach notify \
  --id BREACH-2024-00001 \
  --type authority \
  --recipient-type supervisory_authority \
  --email dpo@data-authority.gov \
  --subject "GDPR Breach Notification - BREACH-2024-00001" \
  --message "We are notifying you of a data breach that occurred..."

# 3. Notify affected users
nself plugin compliance breach notify \
  --id BREACH-2024-00001 \
  --type user \
  --recipient-type affected_user \
  --subject "Important Security Notice"

# 4. Track breach status
nself plugin compliance breach list --status investigating
```

### Example 4: Automated Data Retention

```bash
# 1. Create retention policy
nself plugin compliance retention create \
  --name "Delete old activity logs" \
  --category user_logs \
  --days 90 \
  --action delete \
  --description "Remove user activity logs after 90 days per data minimization principle" \
  --table user_activity_logs \
  --legal-basis "Data minimization - GDPR Article 5(1)(c)" \
  --regulation GDPR

# 2. Execute manually
nself plugin compliance retention execute --id <policy-uuid>

# 3. View execution history
nself plugin compliance retention report --id <policy-uuid>

# 4. Schedule automatic execution via cron
# Add to crontab:
# 0 2 * * * nself plugin compliance retention execute --id <policy-uuid>
```

### Example 5: Privacy Policy Versioning

```http
POST http://localhost:3706/api/compliance/privacy-policies
Content-Type: application/json

{
  "version": "3.0.0",
  "version_number": 3,
  "title": "Privacy Policy v3.0",
  "content": "# Privacy Policy\n\n## 1. Introduction\n...",
  "summary": "Updated to comply with new AI regulations",
  "changes_summary": "Added section 7 on AI data processing and user rights",
  "requires_reacceptance": true,
  "effective_from": "2024-04-01T00:00:00Z",
  "language": "en",
  "jurisdiction": "EU"
}

# Publish policy
POST http://localhost:3706/api/compliance/privacy-policies/{policy-id}/publish

# Users accept policy
POST http://localhost:3706/api/compliance/privacy-policy/accept
Content-Type: application/json

{
  "user_id": "user_12345",
  "policy_id": "{policy-id}"
}

# Check which users need to re-accept
SELECT u.id, u.email
FROM users u
LEFT JOIN compliance_policy_acceptances pa
  ON u.id = pa.user_id AND pa.policy_id = '{new-policy-id}'
WHERE pa.id IS NULL;
```

## Troubleshooting

### DSAR Deadline Warnings

**Problem**: DSARs approaching their 30-day (GDPR) or 45-day (CCPA) deadlines.

**Solution:**
```sql
-- Find overdue or near-deadline DSARs
SELECT request_number, requester_email, status, deadline,
       deadline - NOW() AS time_remaining
FROM compliance_dsars
WHERE status NOT IN ('completed', 'rejected')
  AND deadline < NOW() + INTERVAL '3 days'
ORDER BY deadline ASC;
```

Set up automated notifications:
```bash
# Add to crontab to check daily
0 9 * * * nself plugin compliance dsar list --status pending | grep -i overdue && echo "Overdue DSARs!" | mail -s "DSAR Alert" compliance-team@company.com
```

### Consent Expiry Management

**Problem**: Expired consents not being detected.

**Solution:**
```sql
-- Find expired consents
SELECT user_id, purpose, granted_at, expires_at
FROM compliance_consents
WHERE status = 'granted'
  AND expires_at IS NOT NULL
  AND expires_at < NOW();

-- Auto-mark expired consents
UPDATE compliance_consents
SET status = 'expired', updated_at = NOW()
WHERE status = 'granted'
  AND expires_at IS NOT NULL
  AND expires_at < NOW();
```

Schedule periodic cleanup:
```bash
# Add to crontab
0 0 * * * psql $DATABASE_URL -c "UPDATE compliance_consents SET status = 'expired' WHERE status = 'granted' AND expires_at < NOW();"
```

### Breach Notification Deadline

**Problem**: 72-hour breach notification deadline approaching.

**Solution:**
```sql
-- Find breaches needing notification
SELECT breach_number, title, severity, discovered_at, notification_deadline,
       notification_deadline - NOW() AS time_remaining
FROM compliance_data_breaches
WHERE notification_required = true
  AND authority_notified_at IS NULL
  AND notification_deadline > NOW()
ORDER BY notification_deadline ASC;
```

### Audit Log Too Large

**Problem**: Audit log table growing too large.

**Solution:**
```sql
-- Check audit log size
SELECT
  pg_size_pretty(pg_total_relation_size('compliance_audit_log')) AS total_size,
  COUNT(*) AS row_count,
  MIN(created_at) AS oldest_entry,
  MAX(created_at) AS newest_entry
FROM compliance_audit_log;

-- Archive old audit logs (older than 7 years)
-- First, export to archive
COPY (
  SELECT * FROM compliance_audit_log
  WHERE created_at < NOW() - INTERVAL '2555 days'
) TO '/path/to/audit_archive_2024.csv' CSV HEADER;

-- Then delete archived entries
DELETE FROM compliance_audit_log
WHERE created_at < NOW() - INTERVAL '2555 days';

-- Vacuum to reclaim space
VACUUM FULL compliance_audit_log;
```

### Performance Issues

**Problem**: Slow queries on compliance tables.

**Solution:**
```sql
-- Check index usage
SELECT schemaname, tablename, indexname, idx_scan, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes
WHERE schemaname = 'public'
  AND tablename LIKE 'compliance_%'
ORDER BY idx_scan DESC;

-- Analyze tables
ANALYZE compliance_dsars;
ANALYZE compliance_consents;
ANALYZE compliance_audit_log;

-- Add missing indexes if needed
CREATE INDEX CONCURRENTLY idx_audit_created_desc
  ON compliance_audit_log(created_at DESC);
```

### Database Connection Issues

**Problem**: Cannot connect to PostgreSQL database.

**Solution:**
```bash
# Test connection
psql $DATABASE_URL -c "SELECT 1;"

# Check credentials
echo $DATABASE_URL
echo $POSTGRES_HOST
echo $POSTGRES_PORT
echo $POSTGRES_DB
echo $POSTGRES_USER

# Test with explicit connection
PGPASSWORD=$POSTGRES_PASSWORD psql -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -d $POSTGRES_DB -c "SELECT 1;"
```

### API Authentication Failing

**Problem**: API requests return 401 Unauthorized.

**Solution:**
```bash
# Verify API key is set
echo $COMPLIANCE_API_KEY

# Include API key in request header
curl -X GET http://localhost:3706/api/compliance/dsars \
  -H "Authorization: Bearer $COMPLIANCE_API_KEY"

# Or use X-API-Key header
curl -X GET http://localhost:3706/api/compliance/dsars \
  -H "X-API-Key: $COMPLIANCE_API_KEY"
```

### Rate Limiting Issues

**Problem**: Requests being rate limited.

**Solution:**
```bash
# Increase rate limits
export COMPLIANCE_RATE_LIMIT_MAX=500
export COMPLIANCE_RATE_LIMIT_WINDOW_MS=60000

# Restart server
nself plugin compliance server

# Monitor rate limit headers in responses
curl -v http://localhost:3706/api/compliance/dsars

# Response headers:
# X-RateLimit-Limit: 500
# X-RateLimit-Remaining: 499
# X-RateLimit-Reset: 1707652800
```

---

For additional support, consult the [nself-plugins GitHub repository](https://github.com/acamarata/nself-plugins) or file an issue.
