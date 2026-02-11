# Compliance Plugin

Comprehensive compliance and audit platform for GDPR, CCPA, HIPAA, SOC2, and PCI compliance management. Includes Data Subject Access Requests (DSARs), consent tracking, privacy policies, data retention, breach notification, immutable audit logging, SIEM integration, and compliance reporting.

---

## Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [Database Schema](#database-schema)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Webhook Events](#webhook-events)
- [Compliance Frameworks](#compliance-frameworks)
- [DSAR Management](#dsar-management)
- [Consent Management](#consent-management)
- [Privacy Policy Versioning](#privacy-policy-versioning)
- [Data Retention](#data-retention)
- [Breach Notification](#breach-notification)
- [Audit Logging](#audit-logging)
- [SIEM Integration](#siem-integration)
- [Compliance Reporting](#compliance-reporting)
- [SQL Query Examples](#sql-query-examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Compliance plugin provides a unified platform for managing compliance across multiple regulatory frameworks. It combines privacy compliance features (DSARs, consent, retention) with enterprise audit logging and SIEM integration.

### What This Plugin Does

- **Multi-Framework Compliance** - Support for GDPR, CCPA, HIPAA, SOC2, and PCI DSS
- **Data Subject Rights** - Automated DSAR processing with 30-day deadline tracking
- **Consent Management** - Granular consent tracking with full audit history
- **Privacy Policies** - Version-controlled policies with user acceptance tracking
- **Data Retention** - Automated retention policy execution and data lifecycle management
- **Breach Management** - 72-hour breach notification tracking and compliance
- **Processor Tracking** - Third-party data processor and DPA management
- **Immutable Audit Log** - Append-only audit trail with cryptographic integrity verification
- **SIEM Integration** - Real-time event forwarding to Splunk, ELK Stack, and Datadog
- **Compliance Reports** - Automated SOC2, HIPAA, GDPR, and PCI compliance reporting
- **Alert System** - Rule-based alerting for compliance violations

### Unified Compliance + Audit

This plugin merges two previously separate systems:

1. **Compliance Features** (DSARs, consent, retention, breaches)
2. **Audit Features** (immutable logs, SIEM, integrity verification)

Both systems share the same database, server, and CLI for seamless compliance operations.

---

## Key Features

### Privacy Compliance

- ✅ **GDPR Article 15-22** - Full implementation of data subject rights
- ✅ **CCPA Compliance** - California Consumer Privacy Act requirements
- ✅ **HIPAA Privacy Rule** - Protected Health Information (PHI) safeguards
- ✅ **SOC2 Type II** - Security, availability, and confidentiality controls
- ✅ **PCI DSS** - Payment card data protection requirements

### DSAR Lifecycle

- ✅ **30-Day Deadline** - Automatic deadline tracking and overdue alerts
- ✅ **Request Types** - Access, portability, erasure, rectification, restriction
- ✅ **Activity Timeline** - Full audit trail of DSAR processing steps
- ✅ **Data Exports** - Automated user data package generation
- ✅ **Status Tracking** - Pending, in_progress, completed, rejected states

### Consent Management

- ✅ **Granular Purposes** - Marketing, analytics, profiling, data_sharing, etc.
- ✅ **Opt-in/Opt-out** - Support for both consent models
- ✅ **Consent History** - Complete audit trail of all consent changes
- ✅ **Expiry Tracking** - Automatic consent expiration (default 365 days)
- ✅ **Withdrawal Support** - Easy consent withdrawal with timestamp tracking

### Data Retention

- ✅ **Policy Engine** - Define retention rules by data type
- ✅ **Automated Execution** - Schedule-based policy enforcement
- ✅ **Legal Hold** - Override retention for legal requirements
- ✅ **Execution History** - Track all retention runs with affected records
- ✅ **Dry Run Mode** - Preview what would be deleted before execution

### Breach Management

- ✅ **72-Hour Rule** - GDPR breach notification deadline tracking
- ✅ **Severity Levels** - Low, medium, high, critical classification
- ✅ **Notification Tracking** - Authority, affected users, public disclosure
- ✅ **Mitigation Steps** - Document remediation actions taken
- ✅ **Timeline Management** - Track discovery, containment, resolution dates

### Audit & Forensics

- ✅ **Immutable Log** - Append-only audit trail with cryptographic integrity
- ✅ **Event Types** - User actions, API calls, data access, compliance events
- ✅ **Actor Tracking** - User, service, system, webhook actor types
- ✅ **Rich Metadata** - IP address, user agent, request ID, resource tracking
- ✅ **Checksum Verification** - SHA-256 checksums for event integrity
- ✅ **Retention Policies** - Automated audit log retention (default 7 years)

### SIEM Integration

- ✅ **Splunk HEC** - HTTP Event Collector integration
- ✅ **ELK Stack** - Elasticsearch/Logstash/Kibana support
- ✅ **Datadog Logs** - Datadog log ingestion API
- ✅ **Real-time Forwarding** - Events sent to SIEM within seconds
- ✅ **Fallback Logging** - File-based backup when SIEM unavailable

### Compliance Reporting

- ✅ **SOC2 Reports** - Trust Services Criteria compliance evidence
- ✅ **HIPAA Reports** - Privacy Rule and Security Rule compliance
- ✅ **GDPR Reports** - Data processing, DSARs, breach notifications
- ✅ **PCI Reports** - Cardholder data environment audit trails
- ✅ **Custom Frameworks** - Extensible framework support

---

## Quick Start

```bash
# Install the plugin
nself plugin install compliance

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "COMPLIANCE_GDPR_ENABLED=true" >> .env
echo "COMPLIANCE_CCPA_ENABLED=true" >> .env

# Initialize database schema
nself plugin compliance init

# Start the server
nself plugin compliance server --port 3706

# Submit a DSAR
nself plugin compliance dsars create \
  --email "user@example.com" \
  --type "access" \
  --description "Request my personal data"

# Track DSAR status
nself plugin compliance dsars list

# Log an audit event
nself plugin compliance log \
  --action "user.login" \
  --actor-type "user" \
  --actor-id "user_123" \
  --ip "192.168.1.100"

# Generate compliance report
nself plugin compliance compliance report --framework gdpr
```

---

## Installation

### Prerequisites

- PostgreSQL 12+ database
- Node.js 18+ (for TypeScript implementation)
- nself CLI 0.4.8 or higher

### Install via nself CLI

```bash
# Install the plugin
nself plugin install compliance

# Verify installation
nself plugin list | grep compliance
```

### Manual Installation

```bash
# Clone the repository
git clone https://github.com/acamarata/nself-plugins.git
cd nself-plugins/plugins/compliance

# Install dependencies
cd ts
npm install

# Build TypeScript
npm run build

# Link to nself
nself plugin link .
```

### Database Initialization

```bash
# Create database schema (15 tables)
nself plugin compliance init

# Verify tables were created
psql $DATABASE_URL -c "\dt compliance_*"
psql $DATABASE_URL -c "\dt audit_*"
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | **Yes** | - | PostgreSQL connection string |
| `COMPLIANCE_PLUGIN_PORT` | No | `3706` | HTTP server port |
| `COMPLIANCE_PLUGIN_HOST` | No | `0.0.0.0` | HTTP server bind address |
| `COMPLIANCE_APP_IDS` | No | - | Comma-separated app IDs for multi-app isolation |
| `COMPLIANCE_LOG_LEVEL` | No | `info` | Logging level (debug, info, warn, error) |
| `COMPLIANCE_GDPR_ENABLED` | No | `true` | Enable GDPR compliance features |
| `COMPLIANCE_CCPA_ENABLED` | No | `true` | Enable CCPA compliance features |
| `COMPLIANCE_HIPAA_ENABLED` | No | `false` | Enable HIPAA compliance features |
| `COMPLIANCE_SOC2_ENABLED` | No | `false` | Enable SOC2 compliance features |
| `COMPLIANCE_PCI_ENABLED` | No | `false` | Enable PCI DSS compliance features |
| `COMPLIANCE_DSAR_DEADLINE_DAYS` | No | `30` | DSAR deadline in days (GDPR default) |
| `COMPLIANCE_BREACH_NOTIFICATION_HOURS` | No | `72` | Breach notification deadline in hours |
| `COMPLIANCE_CONSENT_REQUIRED` | No | `false` | Require consent for all data processing |
| `COMPLIANCE_CONSENT_EXPIRY_DAYS` | No | `365` | Consent expiration period (0 = never) |
| `COMPLIANCE_RETENTION_ENABLED` | No | `true` | Enable automated retention policy execution |
| `COMPLIANCE_AUDIT_ENABLED` | No | `true` | Enable immutable audit logging |
| `COMPLIANCE_AUDIT_RETENTION_DAYS` | No | `2555` | Audit log retention (7 years default) |
| `COMPLIANCE_EXPORT_FORMAT` | No | `json` | Default export format (json, csv, xml) |
| `COMPLIANCE_EXPORT_EXPIRY_HOURS` | No | `72` | Export link expiration time |
| `COMPLIANCE_EXPORT_MAX_ROWS` | No | `100000` | Maximum rows per export |
| `COMPLIANCE_API_KEY` | No | - | API key for authenticated endpoints |
| `COMPLIANCE_RATE_LIMIT_MAX` | No | `100` | Max requests per window |
| `COMPLIANCE_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window (1 minute default) |
| `AUDIT_FALLBACK_LOG_PATH` | No | `/var/log/audit` | File path for fallback logging |
| `AUDIT_SIEM_SPLUNK_HEC_URL` | No | - | Splunk HTTP Event Collector URL |
| `AUDIT_SIEM_SPLUNK_HEC_TOKEN` | No | - | Splunk HEC authentication token |
| `AUDIT_SIEM_ELK_URL` | No | - | Elasticsearch API endpoint URL |
| `AUDIT_SIEM_ELK_INDEX` | No | `audit-logs` | Elasticsearch index name |
| `AUDIT_SIEM_ELK_API_KEY` | No | - | Elasticsearch API key |
| `AUDIT_SIEM_DATADOG_API_KEY` | No | - | Datadog API key for log ingestion |
| `AUDIT_SIEM_DATADOG_SITE` | No | `datadoghq.com` | Datadog site (datadoghq.com, datadoghq.eu) |
| `AUDIT_DEFAULT_RETENTION_DAYS` | No | `2555` | Default retention for audit events (7 years) |
| `AUDIT_COMPLIANCE_FRAMEWORKS` | No | `SOC2,HIPAA,GDPR,PCI` | Enabled compliance frameworks |
| `AUDIT_ALERT_WEBHOOK_URL` | No | - | Webhook URL for alert notifications |

### Compliance Framework Configuration

Enable the frameworks you need to comply with:

```bash
# GDPR + CCPA (default for most SaaS apps)
COMPLIANCE_GDPR_ENABLED=true
COMPLIANCE_CCPA_ENABLED=true
COMPLIANCE_DSAR_DEADLINE_DAYS=30
COMPLIANCE_BREACH_NOTIFICATION_HOURS=72

# Healthcare (HIPAA)
COMPLIANCE_HIPAA_ENABLED=true
COMPLIANCE_AUDIT_RETENTION_DAYS=2555  # 7 years required

# Financial services (PCI DSS)
COMPLIANCE_PCI_ENABLED=true
COMPLIANCE_AUDIT_RETENTION_DAYS=365   # 1 year minimum

# Enterprise (SOC2)
COMPLIANCE_SOC2_ENABLED=true
COMPLIANCE_AUDIT_ENABLED=true
```

### SIEM Configuration Examples

#### Splunk HEC

```bash
AUDIT_SIEM_SPLUNK_HEC_URL=https://splunk.example.com:8088/services/collector
AUDIT_SIEM_SPLUNK_HEC_TOKEN=abcd1234-5678-90ab-cdef-1234567890ab
```

#### ELK Stack

```bash
AUDIT_SIEM_ELK_URL=https://elasticsearch.example.com:9200
AUDIT_SIEM_ELK_INDEX=compliance-audit-logs
AUDIT_SIEM_ELK_API_KEY=base64_encoded_api_key
```

#### Datadog

```bash
AUDIT_SIEM_DATADOG_API_KEY=1234567890abcdef1234567890abcdef
AUDIT_SIEM_DATADOG_SITE=datadoghq.com
```

### Example .env File

```bash
# Database
DATABASE_URL=postgresql://compliance:secure_password@localhost:5432/compliance_db

# Server
COMPLIANCE_PLUGIN_PORT=3706
COMPLIANCE_PLUGIN_HOST=0.0.0.0
COMPLIANCE_LOG_LEVEL=info

# Compliance Frameworks
COMPLIANCE_GDPR_ENABLED=true
COMPLIANCE_CCPA_ENABLED=true
COMPLIANCE_HIPAA_ENABLED=false
COMPLIANCE_SOC2_ENABLED=true
COMPLIANCE_PCI_ENABLED=false

# DSAR Configuration
COMPLIANCE_DSAR_DEADLINE_DAYS=30
COMPLIANCE_EXPORT_FORMAT=json
COMPLIANCE_EXPORT_EXPIRY_HOURS=72

# Breach Management
COMPLIANCE_BREACH_NOTIFICATION_HOURS=72

# Consent Management
COMPLIANCE_CONSENT_EXPIRY_DAYS=365
COMPLIANCE_CONSENT_REQUIRED=false

# Data Retention
COMPLIANCE_RETENTION_ENABLED=true

# Audit & SIEM
COMPLIANCE_AUDIT_ENABLED=true
COMPLIANCE_AUDIT_RETENTION_DAYS=2555  # 7 years
AUDIT_SIEM_SPLUNK_HEC_URL=https://splunk.company.com:8088/services/collector
AUDIT_SIEM_SPLUNK_HEC_TOKEN=your_hec_token_here
AUDIT_FALLBACK_LOG_PATH=/var/log/compliance-audit

# Compliance Reporting
AUDIT_COMPLIANCE_FRAMEWORKS=SOC2,HIPAA,GDPR,PCI

# Security
COMPLIANCE_API_KEY=your_secure_api_key_here
COMPLIANCE_RATE_LIMIT_MAX=100
COMPLIANCE_RATE_LIMIT_WINDOW_MS=60000

# Alerts
AUDIT_ALERT_WEBHOOK_URL=https://alerts.company.com/webhook
```

### Multi-Application Support

The plugin supports multi-application isolation using the `source_account_id` column:

```bash
# Configure multiple application IDs
COMPLIANCE_APP_IDS=app1,app2,app3

# All records will be tagged with source_account_id
# Query per app: SELECT * FROM compliance_dsars WHERE source_account_id = 'app1'
```

---

## Database Schema

The plugin creates **15 tables** for comprehensive compliance and audit tracking.

### Compliance Tables (12 tables)

#### 1. compliance_dsars

Data Subject Access Requests with 30-day deadline tracking.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier for multi-app isolation |
| `request_type` | VARCHAR(50) | access, portability, erasure, rectification, restriction |
| `subject_email` | VARCHAR(255) | Data subject's email address |
| `subject_user_id` | VARCHAR(255) | Optional internal user ID |
| `description` | TEXT | Request description from user |
| `status` | VARCHAR(50) | pending, in_progress, completed, rejected |
| `priority` | VARCHAR(20) | normal, high, urgent |
| `deadline` | TIMESTAMP | Calculated deadline (created_at + 30 days) |
| `completed_at` | TIMESTAMP | When DSAR was completed |
| `rejected_reason` | TEXT | Reason if request was rejected |
| `export_url` | TEXT | URL to download data export |
| `export_expires_at` | TIMESTAMP | Export link expiration time |
| `assigned_to` | VARCHAR(255) | User/team assigned to process DSAR |
| `metadata` | JSONB | Additional custom data |
| `created_at` | TIMESTAMP | Request submission time |
| `updated_at` | TIMESTAMP | Last modification time |
| `synced_at` | TIMESTAMP | Last sync time |

**Indexes:**
- `idx_compliance_dsars_email` on `subject_email`
- `idx_compliance_dsars_status` on `status`
- `idx_compliance_dsars_deadline` on `deadline`
- `idx_compliance_dsars_account` on `source_account_id`

#### 2. compliance_dsar_activities

Activity timeline for each DSAR (audit trail of processing steps).

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `dsar_id` | VARCHAR(255) | Foreign key to compliance_dsars |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `activity_type` | VARCHAR(50) | created, assigned, status_changed, note_added, completed |
| `description` | TEXT | Activity description |
| `performed_by` | VARCHAR(255) | User who performed the action |
| `old_value` | TEXT | Previous value (for changes) |
| `new_value` | TEXT | New value (for changes) |
| `metadata` | JSONB | Additional activity data |
| `created_at` | TIMESTAMP | Activity timestamp |

**Indexes:**
- `idx_compliance_dsar_activities_dsar` on `dsar_id`
- `idx_compliance_dsar_activities_created` on `created_at`

#### 3. compliance_consents

User consent records for various processing purposes.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `user_id` | VARCHAR(255) | User identifier |
| `user_email` | VARCHAR(255) | User email address |
| `purpose` | VARCHAR(100) | marketing, analytics, profiling, data_sharing, etc. |
| `consent_given` | BOOLEAN | Current consent status |
| `consent_method` | VARCHAR(50) | opt_in, opt_out, implicit, explicit |
| `consent_source` | VARCHAR(100) | web_form, mobile_app, api, email, etc. |
| `ip_address` | VARCHAR(45) | IP address when consent was given |
| `user_agent` | TEXT | Browser user agent string |
| `consent_text` | TEXT | Exact text user consented to |
| `language` | VARCHAR(10) | Language code (en, es, fr, etc.) |
| `expires_at` | TIMESTAMP | Consent expiration date (if applicable) |
| `withdrawn_at` | TIMESTAMP | When consent was withdrawn (if applicable) |
| `metadata` | JSONB | Additional consent data |
| `created_at` | TIMESTAMP | Initial consent timestamp |
| `updated_at` | TIMESTAMP | Last modification time |
| `synced_at` | TIMESTAMP | Last sync time |

**Indexes:**
- `idx_compliance_consents_user` on `user_id`
- `idx_compliance_consents_email` on `user_email`
- `idx_compliance_consents_purpose` on `purpose`
- `idx_compliance_consents_account` on `source_account_id`

#### 4. compliance_consent_history

Complete audit trail of all consent changes.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `consent_id` | VARCHAR(255) | Foreign key to compliance_consents |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `action` | VARCHAR(50) | granted, withdrawn, updated, expired |
| `consent_given` | BOOLEAN | Consent status after action |
| `reason` | TEXT | Reason for change (if applicable) |
| `ip_address` | VARCHAR(45) | IP address of change |
| `user_agent` | TEXT | User agent string |
| `metadata` | JSONB | Additional change data |
| `created_at` | TIMESTAMP | Change timestamp |

**Indexes:**
- `idx_compliance_consent_history_consent` on `consent_id`
- `idx_compliance_consent_history_created` on `created_at`

#### 5. compliance_privacy_policies

Version-controlled privacy policy documents.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `version` | VARCHAR(50) | Version number (e.g., 1.0.0, 2.1.3) |
| `title` | VARCHAR(255) | Policy title |
| `content` | TEXT | Full policy content (HTML/Markdown) |
| `content_hash` | VARCHAR(64) | SHA-256 hash of content |
| `language` | VARCHAR(10) | Language code |
| `status` | VARCHAR(50) | draft, published, archived |
| `published_at` | TIMESTAMP | When policy became active |
| `archived_at` | TIMESTAMP | When policy was superseded |
| `changes_summary` | TEXT | Summary of changes from previous version |
| `metadata` | JSONB | Additional policy data |
| `created_at` | TIMESTAMP | Policy creation time |
| `updated_at` | TIMESTAMP | Last modification time |

**Indexes:**
- `idx_compliance_privacy_policies_version` on `version`
- `idx_compliance_privacy_policies_status` on `status`
- `idx_compliance_privacy_policies_published` on `published_at`

#### 6. compliance_policy_acceptances

User acceptance tracking for privacy policies.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `policy_id` | VARCHAR(255) | Foreign key to compliance_privacy_policies |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `user_id` | VARCHAR(255) | User identifier |
| `user_email` | VARCHAR(255) | User email address |
| `accepted_version` | VARCHAR(50) | Policy version accepted |
| `acceptance_method` | VARCHAR(50) | checkbox, button, api, implicit |
| `ip_address` | VARCHAR(45) | IP address at acceptance time |
| `user_agent` | TEXT | Browser user agent |
| `metadata` | JSONB | Additional acceptance data |
| `accepted_at` | TIMESTAMP | Acceptance timestamp |

**Indexes:**
- `idx_compliance_policy_acceptances_user` on `user_id`
- `idx_compliance_policy_acceptances_policy` on `policy_id`
- `idx_compliance_policy_acceptances_accepted` on `accepted_at`

#### 7. compliance_retention_policies

Data retention rules and schedules.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `name` | VARCHAR(255) | Policy name |
| `description` | TEXT | Policy description |
| `data_type` | VARCHAR(100) | users, logs, transactions, backups, etc. |
| `retention_period_days` | INTEGER | How long to retain data |
| `action` | VARCHAR(50) | delete, anonymize, archive |
| `schedule` | VARCHAR(100) | Cron expression for execution |
| `enabled` | BOOLEAN | Whether policy is active |
| `legal_hold` | BOOLEAN | Whether data is on legal hold |
| `last_executed_at` | TIMESTAMP | Last execution time |
| `next_execution_at` | TIMESTAMP | Scheduled next execution |
| `metadata` | JSONB | Additional policy data |
| `created_at` | TIMESTAMP | Policy creation time |
| `updated_at` | TIMESTAMP | Last modification time |

**Indexes:**
- `idx_compliance_retention_policies_enabled` on `enabled`
- `idx_compliance_retention_policies_next_exec` on `next_execution_at`

#### 8. compliance_retention_executions

History of retention policy executions.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `policy_id` | VARCHAR(255) | Foreign key to compliance_retention_policies |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `status` | VARCHAR(50) | success, failed, partial |
| `records_affected` | INTEGER | Number of records processed |
| `records_deleted` | INTEGER | Number of records deleted |
| `records_anonymized` | INTEGER | Number of records anonymized |
| `records_archived` | INTEGER | Number of records archived |
| `error_message` | TEXT | Error details if failed |
| `dry_run` | BOOLEAN | Whether this was a dry run |
| `started_at` | TIMESTAMP | Execution start time |
| `completed_at` | TIMESTAMP | Execution end time |
| `duration_seconds` | INTEGER | Execution duration |
| `metadata` | JSONB | Additional execution data |

**Indexes:**
- `idx_compliance_retention_executions_policy` on `policy_id`
- `idx_compliance_retention_executions_started` on `started_at`

#### 9. compliance_processing_records

Records of Processing Activities (ROPA) for GDPR Article 30.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `activity_name` | VARCHAR(255) | Name of processing activity |
| `purpose` | TEXT | Purpose of processing |
| `legal_basis` | VARCHAR(100) | consent, contract, legal_obligation, vital_interests, public_task, legitimate_interests |
| `data_categories` | TEXT[] | Categories of personal data |
| `data_subjects` | TEXT[] | Categories of data subjects |
| `recipients` | TEXT[] | Categories of recipients |
| `transfers` | TEXT[] | International transfers |
| `retention_period` | VARCHAR(255) | Data retention period |
| `security_measures` | TEXT | Technical and organizational measures |
| `dpia_required` | BOOLEAN | Whether DPIA is required |
| `dpia_completed` | BOOLEAN | Whether DPIA is completed |
| `metadata` | JSONB | Additional ROPA data |
| `created_at` | TIMESTAMP | Record creation time |
| `updated_at` | TIMESTAMP | Last modification time |

**Indexes:**
- `idx_compliance_processing_records_activity` on `activity_name`
- `idx_compliance_processing_records_account` on `source_account_id`

#### 10. compliance_data_processors

Third-party data processor tracking and DPA management.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `processor_name` | VARCHAR(255) | Processor company name |
| `contact_email` | VARCHAR(255) | Processor contact email |
| `contact_phone` | VARCHAR(50) | Processor contact phone |
| `website` | VARCHAR(255) | Processor website URL |
| `country` | VARCHAR(100) | Processor location country |
| `services_provided` | TEXT | Description of services |
| `data_categories` | TEXT[] | Categories of data processed |
| `dpa_signed` | BOOLEAN | Whether DPA is signed |
| `dpa_signed_date` | DATE | DPA signature date |
| `dpa_expiry_date` | DATE | DPA expiration date |
| `dpa_document_url` | TEXT | URL to DPA document |
| `certification` | TEXT[] | Certifications (ISO 27001, SOC2, etc.) |
| `status` | VARCHAR(50) | active, inactive, under_review |
| `metadata` | JSONB | Additional processor data |
| `created_at` | TIMESTAMP | Record creation time |
| `updated_at` | TIMESTAMP | Last modification time |

**Indexes:**
- `idx_compliance_data_processors_name` on `processor_name`
- `idx_compliance_data_processors_status` on `status`
- `idx_compliance_data_processors_dpa_expiry` on `dpa_expiry_date`

#### 11. compliance_data_breaches

Data breach tracking with 72-hour notification requirement.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `title` | VARCHAR(255) | Breach title/name |
| `description` | TEXT | Detailed breach description |
| `severity` | VARCHAR(50) | low, medium, high, critical |
| `affected_records` | INTEGER | Number of records affected |
| `affected_users` | INTEGER | Number of users affected |
| `data_types` | TEXT[] | Types of data exposed |
| `discovered_at` | TIMESTAMP | When breach was discovered |
| `contained_at` | TIMESTAMP | When breach was contained |
| `resolved_at` | TIMESTAMP | When breach was fully resolved |
| `root_cause` | TEXT | Root cause analysis |
| `mitigation_steps` | TEXT | Steps taken to mitigate |
| `notification_required` | BOOLEAN | Whether notification is required |
| `notification_deadline` | TIMESTAMP | Notification deadline (72 hours) |
| `authority_notified` | BOOLEAN | Whether authority was notified |
| `users_notified` | BOOLEAN | Whether affected users were notified |
| `public_disclosure` | BOOLEAN | Whether public disclosure was made |
| `status` | VARCHAR(50) | discovered, contained, resolved, closed |
| `metadata` | JSONB | Additional breach data |
| `created_at` | TIMESTAMP | Record creation time |
| `updated_at` | TIMESTAMP | Last modification time |

**Indexes:**
- `idx_compliance_data_breaches_severity` on `severity`
- `idx_compliance_data_breaches_discovered` on `discovered_at`
- `idx_compliance_data_breaches_deadline` on `notification_deadline`

#### 12. compliance_breach_notifications

Tracking of breach notifications sent to authorities and users.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `breach_id` | VARCHAR(255) | Foreign key to compliance_data_breaches |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `notification_type` | VARCHAR(50) | authority, user, public |
| `recipient` | VARCHAR(255) | Recipient name/authority |
| `recipient_email` | VARCHAR(255) | Recipient email address |
| `subject` | VARCHAR(255) | Notification subject line |
| `message` | TEXT | Notification message content |
| `sent_at` | TIMESTAMP | When notification was sent |
| `acknowledged_at` | TIMESTAMP | When recipient acknowledged |
| `metadata` | JSONB | Additional notification data |
| `created_at` | TIMESTAMP | Record creation time |

**Indexes:**
- `idx_compliance_breach_notifications_breach` on `breach_id`
- `idx_compliance_breach_notifications_sent` on `sent_at`

#### 13. compliance_audit_log

Audit log for compliance operations (separate from main audit system).

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGSERIAL | Primary key (auto-incrementing) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `event_type` | VARCHAR(100) | Type of compliance event |
| `resource_type` | VARCHAR(100) | dsar, consent, breach, policy, etc. |
| `resource_id` | VARCHAR(255) | ID of affected resource |
| `action` | VARCHAR(50) | created, updated, deleted, etc. |
| `actor_id` | VARCHAR(255) | User/system that performed action |
| `actor_type` | VARCHAR(50) | user, system, api |
| `ip_address` | VARCHAR(45) | IP address of actor |
| `user_agent` | TEXT | User agent string |
| `changes` | JSONB | Before/after values |
| `metadata` | JSONB | Additional event data |
| `created_at` | TIMESTAMP | Event timestamp |

**Indexes:**
- `idx_compliance_audit_log_resource` on `resource_type, resource_id`
- `idx_compliance_audit_log_created` on `created_at`
- `idx_compliance_audit_log_actor` on `actor_id`

### Audit Tables (3 tables)

#### 14. audit_events

Immutable append-only audit trail with cryptographic integrity.

| Column | Type | Description |
|--------|------|-------------|
| `id` | BIGSERIAL | Primary key (auto-incrementing, ensures order) |
| `source_account_id` | VARCHAR(255) | Application identifier for multi-app isolation |
| `event_id` | UUID | Globally unique event identifier |
| `timestamp` | TIMESTAMP | Event occurrence time (with timezone) |
| `action` | VARCHAR(255) | Action performed (user.login, api.call, etc.) |
| `actor_type` | VARCHAR(50) | user, service, system, webhook |
| `actor_id` | VARCHAR(255) | Actor identifier (user ID, service name) |
| `resource_type` | VARCHAR(100) | Type of resource affected |
| `resource_id` | VARCHAR(255) | Resource identifier |
| `status` | VARCHAR(50) | success, failure, error |
| `ip_address` | INET | IP address of actor |
| `user_agent` | TEXT | User agent string |
| `request_id` | VARCHAR(255) | Request/trace ID for correlation |
| `metadata` | JSONB | Additional event context |
| `checksum` | VARCHAR(64) | SHA-256 checksum for integrity verification |
| `previous_checksum` | VARCHAR(64) | Checksum of previous event (chain integrity) |
| `compliance_frameworks` | TEXT[] | Applicable frameworks (SOC2, HIPAA, etc.) |
| `created_at` | TIMESTAMP | Record insertion time |

**Indexes:**
- `idx_audit_events_timestamp` on `timestamp`
- `idx_audit_events_action` on `action`
- `idx_audit_events_actor` on `actor_id`
- `idx_audit_events_resource` on `resource_type, resource_id`
- `idx_audit_events_account` on `source_account_id`
- `idx_audit_events_frameworks` on `compliance_frameworks` (GIN index)

#### 15. audit_retention_policies

Retention policies specifically for audit logs.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `name` | VARCHAR(255) | Policy name |
| `description` | TEXT | Policy description |
| `retention_days` | INTEGER | How long to retain audit events |
| `event_filters` | JSONB | Filters to match events (action, actor_type, etc.) |
| `compliance_framework` | VARCHAR(50) | Framework this policy supports |
| `enabled` | BOOLEAN | Whether policy is active |
| `last_executed_at` | TIMESTAMP | Last execution time |
| `next_execution_at` | TIMESTAMP | Scheduled next execution |
| `metadata` | JSONB | Additional policy data |
| `created_at` | TIMESTAMP | Policy creation time |
| `updated_at` | TIMESTAMP | Last modification time |

**Indexes:**
- `idx_audit_retention_policies_enabled` on `enabled`
- `idx_audit_retention_policies_next_exec` on `next_execution_at`

#### 16. audit_alert_rules

Alert rules for compliance violations and suspicious activity.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `name` | VARCHAR(255) | Rule name |
| `description` | TEXT | Rule description |
| `rule_type` | VARCHAR(50) | threshold, pattern, anomaly |
| `conditions` | JSONB | Rule conditions (JSON expression) |
| `severity` | VARCHAR(50) | low, medium, high, critical |
| `enabled` | BOOLEAN | Whether rule is active |
| `notification_channels` | TEXT[] | email, slack, webhook, pagerduty |
| `notification_config` | JSONB | Channel-specific configuration |
| `last_triggered_at` | TIMESTAMP | Last time rule was triggered |
| `trigger_count` | INTEGER | Total number of triggers |
| `metadata` | JSONB | Additional rule data |
| `created_at` | TIMESTAMP | Rule creation time |
| `updated_at` | TIMESTAMP | Last modification time |

**Indexes:**
- `idx_audit_alert_rules_enabled` on `enabled`
- `idx_audit_alert_rules_severity` on `severity`

#### 17. audit_webhook_events

Webhook events sent from the audit system.

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Primary key (generated UUID) |
| `source_account_id` | VARCHAR(255) | Application identifier |
| `event_type` | VARCHAR(100) | Webhook event type |
| `payload` | JSONB | Complete webhook payload |
| `url` | TEXT | Webhook URL |
| `status` | VARCHAR(50) | pending, sent, failed |
| `attempts` | INTEGER | Number of delivery attempts |
| `last_attempt_at` | TIMESTAMP | Last delivery attempt time |
| `response_code` | INTEGER | HTTP response code |
| `response_body` | TEXT | HTTP response body |
| `error_message` | TEXT | Error details if failed |
| `created_at` | TIMESTAMP | Event creation time |
| `sent_at` | TIMESTAMP | Successful delivery time |

**Indexes:**
- `idx_audit_webhook_events_status` on `status`
- `idx_audit_webhook_events_type` on `event_type`
- `idx_audit_webhook_events_created` on `created_at`

---

## CLI Commands

The plugin provides comprehensive CLI commands for all compliance operations.

### Plugin Management

```bash
# Initialize database schema (creates all 15 tables)
nself plugin compliance init

# Check plugin status
nself plugin compliance status

# View comprehensive statistics
nself plugin compliance stats
```

### Server Management

```bash
# Start HTTP server (default port 3706)
nself plugin compliance server

# Start on custom port
nself plugin compliance server --port 8080

# Start with specific host binding
nself plugin compliance server --host 127.0.0.1 --port 3706
```

### DSAR Commands

```bash
# List all DSARs
nself plugin compliance dsars list

# List with filters
nself plugin compliance dsars list --status pending
nself plugin compliance dsars list --status overdue
nself plugin compliance dsars list --priority urgent

# Get specific DSAR by ID
nself plugin compliance dsars get dsar_abc123

# Create new DSAR
nself plugin compliance dsars create \
  --email "user@example.com" \
  --type "access" \
  --description "I want a copy of my personal data"

# Create DSAR with priority
nself plugin compliance dsars create \
  --email "user@example.com" \
  --type "erasure" \
  --priority "urgent" \
  --description "Delete all my data immediately"

# Update DSAR status
nself plugin compliance dsars update dsar_abc123 \
  --status "in_progress" \
  --assigned-to "compliance@company.com"

# Complete DSAR
nself plugin compliance dsars complete dsar_abc123 \
  --export-url "https://exports.company.com/user_data.zip"

# Reject DSAR
nself plugin compliance dsars reject dsar_abc123 \
  --reason "Unable to verify identity"

# Add activity to DSAR
nself plugin compliance dsars activity dsar_abc123 \
  --type "note_added" \
  --description "Contacted user for additional verification"

# Search DSARs by email
nself plugin compliance dsars search user@example.com

# Export DSAR list to CSV
nself plugin compliance dsars list --format csv > dsars.csv
```

### Consent Commands

```bash
# List all consents
nself plugin compliance consent list

# List consents for specific user
nself plugin compliance consent list --user-id user_123
nself plugin compliance consent list --email user@example.com

# Get consent details
nself plugin compliance consent get consent_abc123

# Grant consent
nself plugin compliance consent grant \
  --user-id "user_123" \
  --email "user@example.com" \
  --purpose "marketing" \
  --method "opt_in" \
  --ip "192.168.1.100"

# Withdraw consent
nself plugin compliance consent withdraw consent_abc123 \
  --reason "User requested opt-out"

# Update consent
nself plugin compliance consent update consent_abc123 \
  --expires-at "2025-12-31"

# View consent history
nself plugin compliance consent history consent_abc123

# Check if user has active consent
nself plugin compliance consent check \
  --user-id "user_123" \
  --purpose "marketing"

# List expired consents
nself plugin compliance consent list --expired

# Export consent records
nself plugin compliance consent list --format csv > consents.csv
```

### Privacy Policy Commands

```bash
# List all privacy policies
nself plugin compliance policies list

# Get policy by ID
nself plugin compliance policies get policy_abc123

# Create new policy
nself plugin compliance policies create \
  --version "2.0.0" \
  --title "Privacy Policy" \
  --content-file "./privacy-policy.md" \
  --language "en"

# Publish policy
nself plugin compliance policies publish policy_abc123

# Archive old policy
nself plugin compliance policies archive policy_abc123

# View policy acceptances
nself plugin compliance policies acceptances policy_abc123

# Record user acceptance
nself plugin compliance policies accept \
  --policy-id "policy_abc123" \
  --user-id "user_123" \
  --email "user@example.com" \
  --ip "192.168.1.100"

# List users who haven't accepted latest policy
nself plugin compliance policies non-acceptances

# Compare policy versions
nself plugin compliance policies diff policy_v1 policy_v2
```

### Data Retention Commands

```bash
# List retention policies
nself plugin compliance retention list

# Get policy details
nself plugin compliance retention get policy_abc123

# Create retention policy
nself plugin compliance retention create \
  --name "User Data Retention" \
  --data-type "users" \
  --retention-days 730 \
  --action "anonymize" \
  --schedule "0 2 * * 0"

# Enable policy
nself plugin compliance retention enable policy_abc123

# Disable policy
nself plugin compliance retention disable policy_abc123

# Execute policy manually (dry run)
nself plugin compliance retention execute policy_abc123 --dry-run

# Execute policy (actual deletion)
nself plugin compliance retention execute policy_abc123

# View execution history
nself plugin compliance retention history policy_abc123

# Set legal hold
nself plugin compliance retention hold policy_abc123 \
  --reason "Litigation hold for case #12345"

# Release legal hold
nself plugin compliance retention unhold policy_abc123
```

### Breach Management Commands

```bash
# List all breaches
nself plugin compliance breaches list

# List active breaches
nself plugin compliance breaches list --status discovered

# Get breach details
nself plugin compliance breaches get breach_abc123

# Report new breach
nself plugin compliance breaches create \
  --title "Database Exposure" \
  --description "Misconfigured S3 bucket exposed user emails" \
  --severity "high" \
  --affected-records 1000 \
  --affected-users 800 \
  --data-types "email,name"

# Update breach status
nself plugin compliance breaches update breach_abc123 \
  --status "contained" \
  --contained-at "2026-02-11T14:30:00Z"

# Record mitigation steps
nself plugin compliance breaches update breach_abc123 \
  --mitigation "Fixed S3 bucket permissions, rotated credentials"

# Mark breach as resolved
nself plugin compliance breaches resolve breach_abc123 \
  --root-cause "Misconfigured IAM role" \
  --resolved-at "2026-02-11T16:00:00Z"

# Send authority notification
nself plugin compliance breaches notify breach_abc123 \
  --type "authority" \
  --recipient "data-protection@regulator.gov" \
  --message-file "./breach-notice.txt"

# Send user notifications
nself plugin compliance breaches notify breach_abc123 \
  --type "user" \
  --batch-send

# Check notification deadline
nself plugin compliance breaches deadline breach_abc123
```

### Data Processor Commands

```bash
# List all processors
nself plugin compliance processors list

# List active processors
nself plugin compliance processors list --status active

# Get processor details
nself plugin compliance processors get processor_abc123

# Add new processor
nself plugin compliance processors create \
  --name "Email Service Provider" \
  --contact-email "legal@emailprovider.com" \
  --country "United States" \
  --services "Email delivery and tracking" \
  --data-categories "email,name"

# Update DPA status
nself plugin compliance processors update processor_abc123 \
  --dpa-signed true \
  --dpa-signed-date "2026-01-15" \
  --dpa-expiry-date "2028-01-15"

# Upload DPA document
nself plugin compliance processors update processor_abc123 \
  --dpa-url "https://docs.company.com/dpa-emailprovider.pdf"

# Deactivate processor
nself plugin compliance processors deactivate processor_abc123

# List processors with expiring DPAs
nself plugin compliance processors list --dpa-expiring 30
```

### Audit Log Commands

```bash
# Log a new audit event
nself plugin compliance log \
  --action "user.login" \
  --actor-type "user" \
  --actor-id "user_123" \
  --status "success" \
  --ip "192.168.1.100"

# Log with metadata
nself plugin compliance log \
  --action "api.call" \
  --actor-type "service" \
  --actor-id "payment-service" \
  --resource-type "payment" \
  --resource-id "pay_abc123" \
  --status "success" \
  --metadata '{"amount": 99.99, "currency": "USD"}'

# Query audit events
nself plugin compliance query \
  --action "user.login" \
  --from "2026-02-01" \
  --to "2026-02-11"

# Query by actor
nself plugin compliance query --actor-id "user_123"

# Query by resource
nself plugin compliance query \
  --resource-type "payment" \
  --resource-id "pay_abc123"

# Query failed events
nself plugin compliance query --status "failure"

# Export audit events
nself plugin compliance export \
  --from "2026-01-01" \
  --to "2026-01-31" \
  --format "json" \
  --output "audit-jan-2026.json"

# Export to CSV
nself plugin compliance export \
  --from "2026-02-01" \
  --format "csv" \
  --output "audit-feb-2026.csv"

# Verify event integrity
nself plugin compliance verify --event-id event_abc123

# Verify event chain
nself plugin compliance verify --from-id 1000 --to-id 2000

# View audit statistics
nself plugin compliance audit stats

# View events by compliance framework
nself plugin compliance query --framework "HIPAA"
```

### Alert Management Commands

```bash
# List alert rules
nself plugin compliance alerts list

# Get rule details
nself plugin compliance alerts get rule_abc123

# Create threshold alert
nself plugin compliance alerts create \
  --name "Failed Login Attempts" \
  --type "threshold" \
  --conditions '{"action": "user.login", "status": "failure", "count": 5, "window": "5m"}' \
  --severity "high" \
  --channels "email,webhook"

# Create pattern alert
nself plugin compliance alerts create \
  --name "Suspicious Data Access" \
  --type "pattern" \
  --conditions '{"resource_type": "pii", "actor_type": "service", "off_hours": true}' \
  --severity "critical"

# Enable rule
nself plugin compliance alerts enable rule_abc123

# Disable rule
nself plugin compliance alerts disable rule_abc123

# View rule triggers
nself plugin compliance alerts triggers rule_abc123

# Test rule
nself plugin compliance alerts test rule_abc123
```

### Compliance Reporting Commands

```bash
# Generate SOC2 report
nself plugin compliance compliance report --framework soc2

# Generate SOC2 report for date range
nself plugin compliance compliance report \
  --framework soc2 \
  --from "2026-01-01" \
  --to "2026-12-31"

# Generate HIPAA report
nself plugin compliance compliance report --framework hipaa

# Generate GDPR report
nself plugin compliance compliance report --framework gdpr

# Generate PCI report
nself plugin compliance compliance report --framework pci

# Generate all framework reports
nself plugin compliance compliance report --framework all

# Export report to file
nself plugin compliance compliance report \
  --framework soc2 \
  --output "soc2-compliance-report.json"

# Generate PDF report (if supported)
nself plugin compliance compliance report \
  --framework soc2 \
  --format pdf \
  --output "soc2-report.pdf"
```

---

## REST API

The plugin exposes a comprehensive REST API on port **3706** (configurable).

### Health & Status

#### GET /health

Health check endpoint.

```bash
curl http://localhost:3706/health
```

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2026-02-11T12:00:00Z",
  "version": "1.0.0"
}
```

#### GET /api/status

Plugin status and statistics.

```bash
curl http://localhost:3706/api/status
```

**Response:**
```json
{
  "plugin": "compliance",
  "version": "1.0.0",
  "database": "connected",
  "frameworks": ["GDPR", "CCPA", "SOC2"],
  "dsars": {
    "total": 45,
    "pending": 5,
    "overdue": 1
  },
  "consents": {
    "total": 1234,
    "active": 1100,
    "withdrawn": 134
  },
  "breaches": {
    "total": 2,
    "active": 0
  },
  "audit_events": {
    "total": 150000,
    "today": 500
  }
}
```

### DSAR Endpoints

#### POST /api/dsars

Create new DSAR.

```bash
curl -X POST http://localhost:3706/api/dsars \
  -H "Content-Type: application/json" \
  -d '{
    "request_type": "access",
    "subject_email": "user@example.com",
    "subject_user_id": "user_123",
    "description": "I want a copy of my personal data",
    "priority": "normal"
  }'
```

#### GET /api/dsars

List all DSARs.

```bash
# List all
curl http://localhost:3706/api/dsars

# Filter by status
curl "http://localhost:3706/api/dsars?status=pending"

# Filter by priority
curl "http://localhost:3706/api/dsars?priority=urgent"

# Pagination
curl "http://localhost:3706/api/dsars?limit=20&offset=40"
```

#### GET /api/dsars/:id

Get DSAR by ID.

```bash
curl http://localhost:3706/api/dsars/dsar_abc123
```

#### PATCH /api/dsars/:id

Update DSAR.

```bash
curl -X PATCH http://localhost:3706/api/dsars/dsar_abc123 \
  -H "Content-Type: application/json" \
  -d '{
    "status": "in_progress",
    "assigned_to": "compliance@company.com"
  }'
```

#### POST /api/dsars/:id/complete

Complete DSAR.

```bash
curl -X POST http://localhost:3706/api/dsars/dsar_abc123/complete \
  -H "Content-Type: application/json" \
  -d '{
    "export_url": "https://exports.company.com/user_data.zip",
    "export_expires_at": "2026-02-14T12:00:00Z"
  }'
```

#### POST /api/dsars/:id/activities

Add activity to DSAR.

```bash
curl -X POST http://localhost:3706/api/dsars/dsar_abc123/activities \
  -H "Content-Type: application/json" \
  -d '{
    "activity_type": "note_added",
    "description": "Contacted user for verification",
    "performed_by": "admin_user"
  }'
```

### Consent Endpoints

#### POST /api/consents

Grant consent.

```bash
curl -X POST http://localhost:3706/api/consents \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_123",
    "user_email": "user@example.com",
    "purpose": "marketing",
    "consent_given": true,
    "consent_method": "opt_in",
    "consent_source": "web_form",
    "ip_address": "192.168.1.100",
    "consent_text": "I agree to receive marketing emails"
  }'
```

#### GET /api/consents

List consents.

```bash
# List all
curl http://localhost:3706/api/consents

# Filter by user
curl "http://localhost:3706/api/consents?user_id=user_123"

# Filter by purpose
curl "http://localhost:3706/api/consents?purpose=marketing"

# Filter by status
curl "http://localhost:3706/api/consents?consent_given=true"
```

#### GET /api/consents/:id

Get consent by ID.

```bash
curl http://localhost:3706/api/consents/consent_abc123
```

#### PATCH /api/consents/:id

Update consent (withdraw).

```bash
curl -X PATCH http://localhost:3706/api/consents/consent_abc123 \
  -H "Content-Type: application/json" \
  -d '{
    "consent_given": false,
    "withdrawn_at": "2026-02-11T12:00:00Z"
  }'
```

#### GET /api/consents/:id/history

Get consent history.

```bash
curl http://localhost:3706/api/consents/consent_abc123/history
```

### Privacy Policy Endpoints

#### POST /api/policies

Create privacy policy.

```bash
curl -X POST http://localhost:3706/api/policies \
  -H "Content-Type: application/json" \
  -d '{
    "version": "2.0.0",
    "title": "Privacy Policy",
    "content": "Full policy content here...",
    "language": "en",
    "status": "draft"
  }'
```

#### GET /api/policies

List policies.

```bash
curl http://localhost:3706/api/policies
```

#### GET /api/policies/:id

Get policy by ID.

```bash
curl http://localhost:3706/api/policies/policy_abc123
```

#### POST /api/policies/:id/publish

Publish policy.

```bash
curl -X POST http://localhost:3706/api/policies/policy_abc123/publish
```

#### POST /api/policies/:id/accept

Record user acceptance.

```bash
curl -X POST http://localhost:3706/api/policies/policy_abc123/accept \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user_123",
    "user_email": "user@example.com",
    "ip_address": "192.168.1.100"
  }'
```

### Retention Policy Endpoints

#### POST /api/retention

Create retention policy.

```bash
curl -X POST http://localhost:3706/api/retention \
  -H "Content-Type: application/json" \
  -d '{
    "name": "User Data Retention",
    "data_type": "users",
    "retention_period_days": 730,
    "action": "anonymize",
    "schedule": "0 2 * * 0",
    "enabled": true
  }'
```

#### GET /api/retention

List retention policies.

```bash
curl http://localhost:3706/api/retention
```

#### POST /api/retention/:id/execute

Execute retention policy.

```bash
# Dry run
curl -X POST "http://localhost:3706/api/retention/policy_abc123/execute?dry_run=true"

# Actual execution
curl -X POST http://localhost:3706/api/retention/policy_abc123/execute
```

### Breach Management Endpoints

#### POST /api/breaches

Report data breach.

```bash
curl -X POST http://localhost:3706/api/breaches \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Database Exposure",
    "description": "Misconfigured S3 bucket",
    "severity": "high",
    "affected_records": 1000,
    "affected_users": 800,
    "data_types": ["email", "name"],
    "discovered_at": "2026-02-11T10:00:00Z"
  }'
```

#### GET /api/breaches

List breaches.

```bash
curl http://localhost:3706/api/breaches
```

#### GET /api/breaches/:id

Get breach details.

```bash
curl http://localhost:3706/api/breaches/breach_abc123
```

#### POST /api/breaches/:id/notify

Send breach notification.

```bash
curl -X POST http://localhost:3706/api/breaches/breach_abc123/notify \
  -H "Content-Type: application/json" \
  -d '{
    "notification_type": "authority",
    "recipient": "data-protection@regulator.gov",
    "subject": "Data Breach Notification",
    "message": "Notification message..."
  }'
```

### Audit Log Endpoints

#### POST /api/audit/log

Log audit event.

```bash
curl -X POST http://localhost:3706/api/audit/log \
  -H "Content-Type: application/json" \
  -d '{
    "action": "user.login",
    "actor_type": "user",
    "actor_id": "user_123",
    "status": "success",
    "ip_address": "192.168.1.100",
    "metadata": {"device": "iPhone"}
  }'
```

#### GET /api/audit/events

Query audit events.

```bash
# Query all
curl http://localhost:3706/api/audit/events

# Filter by action
curl "http://localhost:3706/api/audit/events?action=user.login"

# Filter by actor
curl "http://localhost:3706/api/audit/events?actor_id=user_123"

# Filter by date range
curl "http://localhost:3706/api/audit/events?from=2026-02-01&to=2026-02-11"

# Filter by framework
curl "http://localhost:3706/api/audit/events?framework=HIPAA"
```

#### POST /api/audit/export

Export audit events.

```bash
curl -X POST http://localhost:3706/api/audit/export \
  -H "Content-Type: application/json" \
  -d '{
    "from": "2026-02-01",
    "to": "2026-02-11",
    "format": "json",
    "filters": {
      "action": "user.login"
    }
  }'
```

#### POST /api/audit/verify

Verify event integrity.

```bash
curl -X POST http://localhost:3706/api/audit/verify \
  -H "Content-Type: application/json" \
  -d '{
    "event_id": "550e8400-e29b-41d4-a716-446655440000"
  }'
```

### Alert Endpoints

#### POST /api/alerts

Create alert rule.

```bash
curl -X POST http://localhost:3706/api/alerts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Failed Login Attempts",
    "rule_type": "threshold",
    "conditions": {
      "action": "user.login",
      "status": "failure",
      "count": 5,
      "window": "5m"
    },
    "severity": "high",
    "notification_channels": ["email", "webhook"],
    "enabled": true
  }'
```

#### GET /api/alerts

List alert rules.

```bash
curl http://localhost:3706/api/alerts
```

### Compliance Reporting Endpoints

#### POST /api/compliance/report

Generate compliance report.

```bash
curl -X POST http://localhost:3706/api/compliance/report \
  -H "Content-Type: application/json" \
  -d '{
    "framework": "soc2",
    "from": "2026-01-01",
    "to": "2026-12-31"
  }'
```

#### GET /api/compliance/frameworks

List supported frameworks.

```bash
curl http://localhost:3706/api/compliance/frameworks
```

### Webhook Endpoint

#### POST /webhook

Receive webhook events.

```bash
curl -X POST http://localhost:3706/webhook \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Signature: sha256=..." \
  -d '{
    "event": "dsar.created",
    "data": {...}
  }'
```

---

## Webhook Events

The plugin emits webhook events for compliance activities.

### DSAR Events

| Event | Description | Payload |
|-------|-------------|---------|
| `dsar.created` | New DSAR submitted | DSAR object |
| `dsar.completed` | DSAR processing completed | DSAR object with export URL |
| `dsar.overdue` | DSAR deadline approaching (7 days) | DSAR object with deadline |

### Consent Events

| Event | Description | Payload |
|-------|-------------|---------|
| `consent.granted` | User granted consent | Consent object |
| `consent.withdrawn` | User withdrew consent | Consent object with withdrawal timestamp |

### Policy Events

| Event | Description | Payload |
|-------|-------------|---------|
| `policy.published` | New privacy policy published | Policy object |

### Retention Events

| Event | Description | Payload |
|-------|-------------|---------|
| `retention.executed` | Retention policy executed | Execution summary with records affected |

### Breach Events

| Event | Description | Payload |
|-------|-------------|---------|
| `breach.created` | New data breach recorded | Breach object |
| `breach.notified` | Breach notification sent | Notification object |

### Audit Events

| Event | Description | Payload |
|-------|-------------|---------|
| `audit.event.created` | Immutable audit event created | Audit event object |
| `audit.event.exported` | Audit events exported | Export metadata |
| `audit.alert.triggered` | Alert rule triggered | Alert details |
| `audit.retention.executed` | Audit retention policy executed | Execution summary |
| `audit.compliance.report_generated` | Compliance report generated | Report metadata |

### Webhook Configuration

```bash
# Set webhook URL for alerts
AUDIT_ALERT_WEBHOOK_URL=https://your-app.com/webhooks/compliance

# Webhook payload example
{
  "event": "dsar.overdue",
  "timestamp": "2026-02-11T12:00:00Z",
  "data": {
    "id": "dsar_abc123",
    "subject_email": "user@example.com",
    "request_type": "access",
    "deadline": "2026-02-15T12:00:00Z",
    "days_remaining": 4
  }
}
```

---

## Compliance Frameworks

### GDPR (General Data Protection Regulation)

**Coverage:**
- Article 15: Right to access
- Article 16: Right to rectification
- Article 17: Right to erasure ("right to be forgotten")
- Article 18: Right to restriction of processing
- Article 20: Right to data portability
- Article 30: Records of Processing Activities (ROPA)
- Article 33: Breach notification to authority (72 hours)
- Article 34: Breach notification to data subjects

**Configuration:**
```bash
COMPLIANCE_GDPR_ENABLED=true
COMPLIANCE_DSAR_DEADLINE_DAYS=30
COMPLIANCE_BREACH_NOTIFICATION_HOURS=72
```

### CCPA (California Consumer Privacy Act)

**Coverage:**
- Right to know (data access)
- Right to delete
- Right to opt-out of sale
- Right to non-discrimination

**Configuration:**
```bash
COMPLIANCE_CCPA_ENABLED=true
COMPLIANCE_DSAR_DEADLINE_DAYS=45  # CCPA allows 45 days
```

### HIPAA (Health Insurance Portability and Accountability Act)

**Coverage:**
- PHI access and audit trails
- Business Associate Agreement (BAA) tracking
- 7-year audit log retention
- Access controls and authentication logging

**Configuration:**
```bash
COMPLIANCE_HIPAA_ENABLED=true
COMPLIANCE_AUDIT_RETENTION_DAYS=2555  # 7 years
AUDIT_COMPLIANCE_FRAMEWORKS=HIPAA
```

### SOC2 (Service Organization Control 2)

**Coverage:**
- Trust Services Criteria (security, availability, confidentiality)
- Audit trail requirements
- Access control logging
- Change management tracking

**Configuration:**
```bash
COMPLIANCE_SOC2_ENABLED=true
COMPLIANCE_AUDIT_ENABLED=true
AUDIT_COMPLIANCE_FRAMEWORKS=SOC2
```

### PCI DSS (Payment Card Industry Data Security Standard)

**Coverage:**
- Cardholder data environment audit trails
- Access control logging
- 1-year minimum audit retention
- Security event monitoring

**Configuration:**
```bash
COMPLIANCE_PCI_ENABLED=true
COMPLIANCE_AUDIT_RETENTION_DAYS=365  # 1 year minimum
AUDIT_COMPLIANCE_FRAMEWORKS=PCI
```

---

## DSAR Management

### DSAR Lifecycle

```
1. Created (pending)
   ↓
2. Assigned (in_progress)
   ↓
3. Data Collected
   ↓
4. Export Generated
   ↓
5. Completed (or Rejected)
```

### DSAR Types

| Type | Description | GDPR Article |
|------|-------------|--------------|
| `access` | Provide copy of all personal data | Article 15 |
| `portability` | Export data in machine-readable format | Article 20 |
| `erasure` | Delete all personal data ("right to be forgotten") | Article 17 |
| `rectification` | Correct inaccurate personal data | Article 16 |
| `restriction` | Restrict processing of personal data | Article 18 |

### Deadline Tracking

- Default deadline: **30 days** (GDPR)
- Warning sent **7 days** before deadline
- Overdue DSARs flagged automatically
- Email notifications for approaching deadlines

### Data Export Format

DSARs generate ZIP packages containing:

```
user_data_123.zip
├── manifest.json          # Export metadata
├── profile.json           # User profile data
├── transactions.json      # Transaction history
├── consents.json          # Consent records
├── audit_trail.json       # User activity log
└── attachments/           # User-uploaded files
```

### Example DSAR Workflow

```bash
# 1. User submits DSAR
nself plugin compliance dsars create \
  --email "john@example.com" \
  --type "access"

# 2. Assign to compliance team
nself plugin compliance dsars update dsar_abc123 \
  --status "in_progress" \
  --assigned-to "compliance@company.com"

# 3. Add processing note
nself plugin compliance dsars activity dsar_abc123 \
  --type "note_added" \
  --description "Collecting data from all systems"

# 4. Generate export (automated)
nself plugin compliance export \
  --user-id "user_123" \
  --format "json"

# 5. Complete DSAR
nself plugin compliance dsars complete dsar_abc123 \
  --export-url "https://secure-exports.company.com/user_123.zip"

# 6. User receives email with download link (72-hour expiry)
```

---

## Consent Management

### Consent Purposes

Common consent purposes:

- `marketing` - Marketing communications
- `analytics` - Analytics and tracking
- `profiling` - User profiling and targeting
- `data_sharing` - Sharing data with third parties
- `essential` - Essential service operations (no consent required)

### Consent Methods

| Method | Description | Use Case |
|--------|-------------|----------|
| `opt_in` | User explicitly agrees | GDPR-compliant (required) |
| `opt_out` | User must actively disagree | CCPA-compliant |
| `implicit` | Implied by using service | Essential functions only |
| `explicit` | Strong consent (checkbox) | Sensitive data processing |

### Consent Expiry

Consents can expire after a configurable period:

```bash
# Set consent expiry to 1 year
COMPLIANCE_CONSENT_EXPIRY_DAYS=365

# Set to never expire
COMPLIANCE_CONSENT_EXPIRY_DAYS=0
```

### Consent Audit Trail

Every consent change is logged in `compliance_consent_history`:

```sql
SELECT
  h.action,
  h.consent_given,
  h.reason,
  h.created_at,
  c.purpose
FROM compliance_consent_history h
JOIN compliance_consents c ON h.consent_id = c.id
WHERE c.user_email = 'user@example.com'
ORDER BY h.created_at DESC;
```

### Withdrawal Process

```bash
# User withdraws consent
nself plugin compliance consent withdraw consent_abc123

# Check if user has active marketing consent
nself plugin compliance consent check \
  --user-id "user_123" \
  --purpose "marketing"

# Result: false (consent withdrawn)
```

---

## Privacy Policy Versioning

### Version Management

Privacy policies are version-controlled with SHA-256 content hashing:

```bash
# Create new policy version
nself plugin compliance policies create \
  --version "2.1.0" \
  --title "Privacy Policy" \
  --content-file "./privacy-policy-v2.1.md"

# Publish policy (activates it)
nself plugin compliance policies publish policy_abc123

# Previous version automatically archived
```

### User Acceptance Tracking

Track which users have accepted which policy versions:

```sql
-- Users who haven't accepted latest policy
SELECT DISTINCT u.id, u.email
FROM users u
LEFT JOIN compliance_policy_acceptances pa
  ON u.id = pa.user_id
  AND pa.accepted_version = (
    SELECT version FROM compliance_privacy_policies
    WHERE status = 'published'
    ORDER BY published_at DESC
    LIMIT 1
  )
WHERE pa.id IS NULL;
```

### Policy Acceptance Flow

```bash
# 1. Publish new policy
nself plugin compliance policies publish policy_v2

# 2. User logs in, sees new policy
# 3. Record acceptance via API
curl -X POST http://localhost:3706/api/policies/policy_v2/accept \
  -d '{
    "user_id": "user_123",
    "user_email": "user@example.com",
    "ip_address": "192.168.1.100"
  }'

# 4. User can now access service
```

---

## Data Retention

### Retention Policy Configuration

```bash
# Create retention policy for user data
nself plugin compliance retention create \
  --name "Inactive User Cleanup" \
  --data-type "users" \
  --retention-days 1095 \  # 3 years
  --action "anonymize" \
  --schedule "0 2 * * 0"   # Every Sunday at 2 AM
```

### Retention Actions

| Action | Description | Use Case |
|--------|-------------|----------|
| `delete` | Permanently delete records | Non-critical data |
| `anonymize` | Remove PII, keep aggregates | Analytics data |
| `archive` | Move to cold storage | Historical records |

### Legal Hold

Prevent retention policy execution for legal matters:

```bash
# Set legal hold
nself plugin compliance retention hold policy_abc123 \
  --reason "Litigation hold for case #12345"

# Retention policy won't execute while hold is active

# Release hold
nself plugin compliance retention unhold policy_abc123
```

### Dry Run Testing

Test retention policies without deleting data:

```bash
# Dry run shows what would be deleted
nself plugin compliance retention execute policy_abc123 --dry-run

# Output:
# Would affect 150 records
# Would delete 100 records
# Would anonymize 50 records
```

### Retention Execution History

```sql
-- View retention execution history
SELECT
  p.name,
  e.started_at,
  e.records_affected,
  e.records_deleted,
  e.records_anonymized,
  e.duration_seconds,
  e.status
FROM compliance_retention_executions e
JOIN compliance_retention_policies p ON e.policy_id = p.id
ORDER BY e.started_at DESC
LIMIT 10;
```

---

## Breach Notification

### 72-Hour Rule (GDPR)

GDPR Article 33 requires breach notification within **72 hours** of discovery.

### Breach Severity Levels

| Severity | Description | Example |
|----------|-------------|---------|
| `low` | Minimal risk to individuals | Exposure of non-sensitive metadata |
| `medium` | Moderate risk | Exposure of email addresses |
| `high` | Significant risk | Exposure of financial data |
| `critical` | Severe risk | Exposure of health/biometric data |

### Breach Notification Types

1. **Authority Notification** - Data protection authority (required)
2. **User Notification** - Affected individuals (if high risk)
3. **Public Disclosure** - Public announcement (if widespread)

### Breach Timeline

```
Discovery → Containment → Resolution → Notification → Closure
   ↓            ↓             ↓            ↓            ↓
 Hour 0      Hour 4        Hour 24      Hour 72      Day 30
```

### Example Breach Response

```bash
# 1. Report breach immediately
nself plugin compliance breaches create \
  --title "Misconfigured S3 Bucket" \
  --severity "high" \
  --affected-records 5000 \
  --discovered-at "2026-02-11T08:00:00Z"

# 2. Update as contained
nself plugin compliance breaches update breach_abc123 \
  --status "contained" \
  --contained-at "2026-02-11T12:00:00Z"

# 3. Notify authority within 72 hours
nself plugin compliance breaches notify breach_abc123 \
  --type "authority" \
  --recipient "ico@ico.org.uk"

# 4. Notify affected users
nself plugin compliance breaches notify breach_abc123 \
  --type "user" \
  --batch-send

# 5. Mark as resolved
nself plugin compliance breaches resolve breach_abc123 \
  --root-cause "Misconfigured IAM permissions"
```

---

## Audit Logging

### Immutable Audit Trail

The `audit_events` table is **append-only** with cryptographic integrity:

1. Each event has a SHA-256 checksum
2. Each event links to previous event's checksum (blockchain-like chain)
3. Any tampering breaks the chain and is detectable

### Event Integrity Verification

```bash
# Verify single event
nself plugin compliance verify --event-id abc123

# Verify event chain
nself plugin compliance verify --from-id 1000 --to-id 2000

# Output:
# ✓ All 1000 events verified successfully
# ✓ Chain integrity intact
```

### Common Audit Actions

| Action | Description |
|--------|-------------|
| `user.login` | User authentication |
| `user.logout` | User session end |
| `user.password_change` | Password update |
| `api.call` | API request |
| `data.read` | Data access |
| `data.write` | Data modification |
| `data.delete` | Data deletion |
| `config.change` | Configuration update |
| `permission.grant` | Permission granted |
| `permission.revoke` | Permission revoked |

### Actor Types

- `user` - Human user
- `service` - Service/API client
- `system` - Automated system process
- `webhook` - External webhook

### Audit Event Example

```json
{
  "id": 12345,
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2026-02-11T12:00:00Z",
  "action": "data.read",
  "actor_type": "user",
  "actor_id": "user_123",
  "resource_type": "customer",
  "resource_id": "cust_456",
  "status": "success",
  "ip_address": "192.168.1.100",
  "user_agent": "Mozilla/5.0...",
  "metadata": {
    "endpoint": "/api/customers/cust_456",
    "response_time_ms": 45
  },
  "checksum": "a1b2c3d4...",
  "previous_checksum": "x7y8z9...",
  "compliance_frameworks": ["SOC2", "HIPAA"]
}
```

---

## SIEM Integration

### Supported SIEM Platforms

1. **Splunk** - HTTP Event Collector (HEC)
2. **ELK Stack** - Elasticsearch
3. **Datadog** - Log ingestion API

### Real-time Event Forwarding

All audit events are automatically forwarded to configured SIEM platforms within seconds.

### Splunk Configuration

```bash
AUDIT_SIEM_SPLUNK_HEC_URL=https://splunk.company.com:8088/services/collector
AUDIT_SIEM_SPLUNK_HEC_TOKEN=your_hec_token_here
```

**Splunk HEC Token Setup:**
1. Settings > Data Inputs > HTTP Event Collector
2. Create new token
3. Select source type: `_json`
4. Copy token to environment variable

### ELK Stack Configuration

```bash
AUDIT_SIEM_ELK_URL=https://elasticsearch.company.com:9200
AUDIT_SIEM_ELK_INDEX=compliance-audit-logs
AUDIT_SIEM_ELK_API_KEY=base64_encoded_api_key
```

**Elasticsearch API Key Setup:**
1. Security > API Keys > Create API Key
2. Grant `write` permission to index
3. Copy Base64-encoded key

### Datadog Configuration

```bash
AUDIT_SIEM_DATADOG_API_KEY=your_datadog_api_key
AUDIT_SIEM_DATADOG_SITE=datadoghq.com  # or datadoghq.eu
```

**Datadog API Key Setup:**
1. Organization Settings > API Keys
2. Create new API key
3. Copy key to environment variable

### Fallback Logging

If SIEM is unavailable, events are written to local files:

```bash
AUDIT_FALLBACK_LOG_PATH=/var/log/compliance-audit

# Files created:
# /var/log/compliance-audit/2026-02-11.log
# /var/log/compliance-audit/2026-02-12.log
```

### SIEM Event Format

Events are sent as JSON with standardized fields:

```json
{
  "timestamp": "2026-02-11T12:00:00Z",
  "source": "compliance-plugin",
  "action": "user.login",
  "actor": {
    "type": "user",
    "id": "user_123",
    "ip": "192.168.1.100"
  },
  "resource": {
    "type": "account",
    "id": "acct_456"
  },
  "status": "success",
  "metadata": {...},
  "compliance_frameworks": ["SOC2", "HIPAA"]
}
```

---

## Compliance Reporting

### Framework Reports

Generate compliance reports for auditors:

```bash
# SOC2 Type II Report
nself plugin compliance compliance report \
  --framework soc2 \
  --from "2026-01-01" \
  --to "2026-12-31"

# HIPAA Privacy & Security Rule Report
nself plugin compliance compliance report \
  --framework hipaa \
  --from "2025-01-01" \
  --to "2025-12-31"

# GDPR Compliance Report
nself plugin compliance compliance report --framework gdpr

# PCI DSS Report
nself plugin compliance compliance report --framework pci
```

### Report Contents

#### SOC2 Report

- **Security** - Access control logs, authentication events
- **Availability** - Uptime metrics, incident responses
- **Confidentiality** - Data access logs, encryption events
- **Processing Integrity** - Data validation, error handling
- **Privacy** - Consent records, DSAR processing

#### HIPAA Report

- **Privacy Rule** - PHI access logs, minimum necessary access
- **Security Rule** - Authentication, encryption, audit trails
- **Breach Notification** - Breach records, notification timeline
- **Business Associates** - DPA tracking, processor management

#### GDPR Report

- **DSARs** - Request volume, completion rate, deadline compliance
- **Consent** - Consent records, withdrawal tracking
- **Breaches** - Breach count, 72-hour compliance
- **ROPA** - Records of Processing Activities
- **Data Processors** - Third-party processor list

#### PCI Report

- **Access Control** - Cardholder data access logs
- **Network Security** - Firewall logs, network segmentation
- **Data Protection** - Encryption, tokenization events
- **Monitoring** - Audit trail coverage, log retention

### Report Output Formats

- **JSON** - Machine-readable, API integration
- **CSV** - Spreadsheet import
- **PDF** - Human-readable, auditor distribution (if supported)

### Custom Report Queries

```sql
-- SOC2: Failed login attempts (security)
SELECT
  DATE(timestamp) as date,
  COUNT(*) as failed_attempts,
  COUNT(DISTINCT actor_id) as unique_users
FROM audit_events
WHERE action = 'user.login'
  AND status = 'failure'
  AND 'SOC2' = ANY(compliance_frameworks)
GROUP BY DATE(timestamp)
ORDER BY date DESC;

-- HIPAA: PHI access by user
SELECT
  actor_id,
  COUNT(*) as access_count,
  MIN(timestamp) as first_access,
  MAX(timestamp) as last_access
FROM audit_events
WHERE resource_type = 'phi'
  AND action = 'data.read'
  AND 'HIPAA' = ANY(compliance_frameworks)
GROUP BY actor_id
ORDER BY access_count DESC;

-- GDPR: DSAR completion rate
SELECT
  DATE_TRUNC('month', created_at) as month,
  COUNT(*) as total_dsars,
  SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed,
  ROUND(100.0 * SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) / COUNT(*), 2) as completion_rate
FROM compliance_dsars
GROUP BY DATE_TRUNC('month', created_at)
ORDER BY month DESC;
```

---

## SQL Query Examples

### DSARs

```sql
-- Overdue DSARs
SELECT
  id,
  subject_email,
  request_type,
  deadline,
  EXTRACT(DAY FROM (NOW() - deadline)) as days_overdue
FROM compliance_dsars
WHERE status IN ('pending', 'in_progress')
  AND deadline < NOW()
ORDER BY days_overdue DESC;

-- DSAR processing time
SELECT
  request_type,
  AVG(EXTRACT(EPOCH FROM (completed_at - created_at)) / 86400) as avg_days,
  MIN(EXTRACT(EPOCH FROM (completed_at - created_at)) / 86400) as min_days,
  MAX(EXTRACT(EPOCH FROM (completed_at - created_at)) / 86400) as max_days
FROM compliance_dsars
WHERE status = 'completed'
GROUP BY request_type;

-- DSARs by month
SELECT
  DATE_TRUNC('month', created_at) as month,
  request_type,
  COUNT(*) as count
FROM compliance_dsars
GROUP BY DATE_TRUNC('month', created_at), request_type
ORDER BY month DESC, request_type;
```

### Consents

```sql
-- Active marketing consents
SELECT
  user_email,
  consent_method,
  created_at as granted_at,
  expires_at
FROM compliance_consents
WHERE purpose = 'marketing'
  AND consent_given = true
  AND (expires_at IS NULL OR expires_at > NOW())
  AND withdrawn_at IS NULL
ORDER BY created_at DESC;

-- Consent withdrawal rate
SELECT
  purpose,
  COUNT(*) as total_consents,
  SUM(CASE WHEN withdrawn_at IS NOT NULL THEN 1 ELSE 0 END) as withdrawn,
  ROUND(100.0 * SUM(CASE WHEN withdrawn_at IS NOT NULL THEN 1 ELSE 0 END) / COUNT(*), 2) as withdrawal_rate
FROM compliance_consents
GROUP BY purpose
ORDER BY withdrawal_rate DESC;

-- Consent expiry in next 30 days
SELECT
  user_email,
  purpose,
  expires_at,
  EXTRACT(DAY FROM (expires_at - NOW())) as days_until_expiry
FROM compliance_consents
WHERE consent_given = true
  AND expires_at BETWEEN NOW() AND NOW() + INTERVAL '30 days'
ORDER BY expires_at;
```

### Privacy Policies

```sql
-- Users who haven't accepted latest policy
SELECT u.id, u.email, pp.version as latest_version
FROM users u
CROSS JOIN (
  SELECT version FROM compliance_privacy_policies
  WHERE status = 'published'
  ORDER BY published_at DESC
  LIMIT 1
) pp
LEFT JOIN compliance_policy_acceptances pa
  ON u.id = pa.user_id
  AND pa.accepted_version = pp.version
WHERE pa.id IS NULL;

-- Policy acceptance rate
SELECT
  p.version,
  COUNT(DISTINCT pa.user_id) as users_accepted,
  (SELECT COUNT(*) FROM users) as total_users,
  ROUND(100.0 * COUNT(DISTINCT pa.user_id) / (SELECT COUNT(*) FROM users), 2) as acceptance_rate
FROM compliance_privacy_policies p
LEFT JOIN compliance_policy_acceptances pa ON p.id = pa.policy_id
WHERE p.status = 'published'
GROUP BY p.version
ORDER BY p.published_at DESC;
```

### Data Retention

```sql
-- Retention policy execution summary
SELECT
  p.name,
  e.started_at,
  e.records_deleted + e.records_anonymized as total_affected,
  e.duration_seconds,
  e.status
FROM compliance_retention_executions e
JOIN compliance_retention_policies p ON e.policy_id = p.id
WHERE e.started_at > NOW() - INTERVAL '30 days'
ORDER BY e.started_at DESC;

-- Data eligible for retention
SELECT
  'users' as data_type,
  COUNT(*) as records_eligible
FROM users
WHERE last_login_at < NOW() - INTERVAL '3 years'
  AND deleted_at IS NULL;
```

### Breaches

```sql
-- Breach notification compliance
SELECT
  id,
  title,
  severity,
  discovered_at,
  notification_deadline,
  authority_notified,
  CASE
    WHEN authority_notified AND
         (SELECT MIN(sent_at) FROM compliance_breach_notifications
          WHERE breach_id = b.id AND notification_type = 'authority') < notification_deadline
    THEN 'Compliant'
    ELSE 'Non-Compliant'
  END as compliance_status
FROM compliance_data_breaches b
WHERE notification_required = true;

-- Breach impact summary
SELECT
  severity,
  COUNT(*) as breach_count,
  SUM(affected_users) as total_users_affected,
  SUM(affected_records) as total_records_affected
FROM compliance_data_breaches
GROUP BY severity
ORDER BY
  CASE severity
    WHEN 'critical' THEN 1
    WHEN 'high' THEN 2
    WHEN 'medium' THEN 3
    WHEN 'low' THEN 4
  END;
```

### Audit Events

```sql
-- Failed login attempts in last 24 hours
SELECT
  actor_id,
  COUNT(*) as attempts,
  ARRAY_AGG(DISTINCT ip_address::TEXT) as ip_addresses,
  MAX(timestamp) as last_attempt
FROM audit_events
WHERE action = 'user.login'
  AND status = 'failure'
  AND timestamp > NOW() - INTERVAL '24 hours'
GROUP BY actor_id
HAVING COUNT(*) > 3
ORDER BY attempts DESC;

-- Data access by resource type
SELECT
  resource_type,
  COUNT(*) as access_count,
  COUNT(DISTINCT actor_id) as unique_actors,
  COUNT(DISTINCT DATE(timestamp)) as days_accessed
FROM audit_events
WHERE action IN ('data.read', 'data.write', 'data.delete')
  AND timestamp > NOW() - INTERVAL '30 days'
GROUP BY resource_type
ORDER BY access_count DESC;

-- Audit event volume by hour
SELECT
  DATE_TRUNC('hour', timestamp) as hour,
  COUNT(*) as event_count
FROM audit_events
WHERE timestamp > NOW() - INTERVAL '24 hours'
GROUP BY DATE_TRUNC('hour', timestamp)
ORDER BY hour;

-- Compliance framework coverage
SELECT
  UNNEST(compliance_frameworks) as framework,
  COUNT(*) as event_count
FROM audit_events
WHERE timestamp > NOW() - INTERVAL '30 days'
GROUP BY framework
ORDER BY event_count DESC;
```

---

## Troubleshooting

### Database Connection Issues

**Problem:** `Error: Connection refused`

**Solution:**
```bash
# Verify DATABASE_URL is set
echo $DATABASE_URL

# Test PostgreSQL connection
psql $DATABASE_URL -c "SELECT 1"

# Check PostgreSQL is running
pg_isready
```

### DSAR Deadline Not Calculating

**Problem:** DSAR deadline is NULL

**Solution:**
```sql
-- Update existing DSARs with deadline
UPDATE compliance_dsars
SET deadline = created_at + INTERVAL '30 days'
WHERE deadline IS NULL;
```

### Consent Not Expiring

**Problem:** Expired consents still showing as active

**Solution:**
```bash
# Check expiry configuration
echo $COMPLIANCE_CONSENT_EXPIRY_DAYS

# Manually expire old consents
psql $DATABASE_URL <<SQL
UPDATE compliance_consents
SET consent_given = false
WHERE expires_at < NOW()
  AND consent_given = true;
SQL
```

### Audit Events Not Forwarding to SIEM

**Problem:** Events not appearing in Splunk/ELK/Datadog

**Solution:**
```bash
# Check SIEM configuration
echo $AUDIT_SIEM_SPLUNK_HEC_URL
echo $AUDIT_SIEM_SPLUNK_HEC_TOKEN

# Test SIEM connectivity
curl -X POST $AUDIT_SIEM_SPLUNK_HEC_URL \
  -H "Authorization: Splunk $AUDIT_SIEM_SPLUNK_HEC_TOKEN" \
  -d '{"event": "test"}'

# Check fallback logs
tail -f /var/log/compliance-audit/$(date +%Y-%m-%d).log
```

### Retention Policy Not Executing

**Problem:** Scheduled retention policy not running

**Solution:**
```bash
# Check policy schedule
nself plugin compliance retention list

# Enable policy if disabled
nself plugin compliance retention enable policy_abc123

# Manual execution
nself plugin compliance retention execute policy_abc123
```

### Breach Notification Deadline Passed

**Problem:** Breach not notified within 72 hours

**Solution:**
```bash
# Immediately send notification
nself plugin compliance breaches notify breach_abc123 \
  --type "authority" \
  --recipient "data-protection@regulator.gov"

# Document delay in breach record
nself plugin compliance breaches update breach_abc123 \
  --metadata '{"delay_reason": "Discovery delayed due to system outage"}'
```

### Alert Rules Not Triggering

**Problem:** Alert rule conditions met but no notification

**Solution:**
```bash
# Check rule is enabled
nself plugin compliance alerts list

# Test rule manually
nself plugin compliance alerts test rule_abc123

# Verify webhook URL is accessible
curl -X POST $AUDIT_ALERT_WEBHOOK_URL \
  -H "Content-Type: application/json" \
  -d '{"test": true}'
```

### Event Integrity Verification Failed

**Problem:** Checksum verification fails for audit events

**Solution:**
```bash
# Identify tampered events
nself plugin compliance verify --from-id 1 --to-id 10000

# View event details
psql $DATABASE_URL <<SQL
SELECT id, event_id, action, checksum, previous_checksum
FROM audit_events
WHERE id = <failing_id>;
SQL

# Check for database corruption
psql $DATABASE_URL <<SQL
SELECT pg_stat_database_conflicts.* FROM pg_stat_database_conflicts;
SQL
```

---

**Last Updated:** February 11, 2026
**Plugin Version:** 1.0.0
**Minimum nself Version:** 0.4.8
