# Admin API Plugin

Comprehensive admin dashboard backend for nself applications with system health monitoring, user management, metrics, and audit logging.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [Usage Examples](#usage-examples)
- [Multi-App Support](#multi-app-support)
- [Development](#development)

---

## Overview

The Admin API plugin provides a complete backend for administrative dashboards and system management. It supports:

- **System Health Monitoring** - CPU, memory, disk, database, storage, and service health checks
- **User Management** - List, ban, unban, and delete users
- **Metrics Dashboard** - DAU/WAU/MAU, content metrics, playback metrics
- **Audit Logging** - Immutable audit trail for all admin actions
- **Multi-App Support** - Full tenant isolation with `source_account_id`
- **2 Database Tables** - Admin users and audit log
- **Secure API** - JWT authentication, API key support, rate limiting

### Key Features

| Feature | Description |
|---------|-------------|
| Health Checks | Real-time monitoring of database, storage, queue, and services |
| User Management | Ban, unban, delete users with audit trail |
| Metrics | Active users, content stats, playback analytics |
| Audit Log | Immutable log of all admin actions with IP tracking |
| Role-Based Access | Super admin, admin, and moderator roles |
| Rate Limiting | Configurable rate limits to prevent abuse |

---

## Quick Start

```bash
# Install the plugin
nself plugin install admin-api

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "ADMIN_JWT_SECRET=your-secret-key" >> .env
echo "ADMIN_API_KEY=your-api-key" >> .env

# Initialize database schema
nself plugin admin-api init

# Start the server
nself plugin admin-api server --port 3214

# Check health
curl http://localhost:3214/health
```

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_API_PORT` | `3214` | HTTP server port |
| `ADMIN_JWT_SECRET` | - | Secret key for JWT token signing |
| `ADMIN_SESSION_TIMEOUT_MINUTES` | `60` | Session timeout in minutes |
| `ADMIN_METRICS_COLLECTION_INTERVAL_SECONDS` | `60` | Metrics collection interval |
| `ADMIN_API_KEY` | - | API key for authentication |
| `ADMIN_RATE_LIMIT_MAX` | `100` | Max requests per window |
| `ADMIN_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window in milliseconds |

---

## REST API

### System Health

**Overall Health**
```
GET /api/admin/health
```

Returns overall system health including all subsystems.

**Database Health**
```
GET /api/admin/health/database
```

Returns PostgreSQL connection status and query performance.

**Storage Health**
```
GET /api/admin/health/storage
```

Returns object storage status (MinIO/S3/R2/etc.).

**Queue Health**
```
GET /api/admin/health/queue
```

Returns background job queue status (BullMQ/Redis).

**Service Status**
```
GET /api/admin/health/services
```

Returns status of all enabled nself plugins and services.

---

### User Management

**List Users**
```
GET /api/admin/users
```

Query parameters:
- `page` (number) - Page number for pagination
- `limit` (number) - Results per page
- `search` (string) - Search by email or username
- `status` (string) - Filter by status: `active`, `banned`, `deleted`

**Get User Details**
```
GET /api/admin/users/:id
```

Returns complete user profile including activity history.

**Ban User**
```
PATCH /api/admin/users/:id/ban
```

Request body:
```json
{
  "reason": "Terms violation",
  "expires_at": "2026-03-14T00:00:00Z"  // Optional
}
```

**Unban User**
```
PATCH /api/admin/users/:id/unban
```

**Delete User**
```
DELETE /api/admin/users/:id
```

Permanently deletes user and all associated data.

---

### Content Management

**List Content**
```
GET /api/admin/content
```

Query parameters:
- `type` (string) - Filter by content type: `video`, `audio`, `image`, `document`
- `status` (string) - Filter by status: `active`, `flagged`, `removed`
- `page` (number) - Page number
- `limit` (number) - Results per page

**Delete Content**
```
DELETE /api/admin/content/:id
```

---

### Metrics

**User Metrics**
```
GET /api/admin/metrics/users
```

Returns:
- Daily Active Users (DAU)
- Weekly Active Users (WAU)
- Monthly Active Users (MAU)
- New signups
- Churn rate

**Content Metrics**
```
GET /api/admin/metrics/content
```

Returns:
- Total content items by type
- Upload trends
- Storage usage
- Popular content

**Playback Metrics**
```
GET /api/admin/metrics/playback
```

Returns:
- Total views/plays
- Watch time
- Completion rates
- Peak concurrent users

---

### Alerts

**List Alerts**
```
GET /api/admin/alerts
```

Returns active system alerts.

**Acknowledge Alert**
```
POST /api/admin/alerts/:id/acknowledge
```

---

### Audit Log

**View Audit Log**
```
GET /api/admin/audit-log
```

Query parameters:
- `admin_user_id` (uuid) - Filter by admin user
- `action` (string) - Filter by action type
- `entity_type` (string) - Filter by entity type
- `start_date` (iso8601) - Start date
- `end_date` (iso8601) - End date
- `page` (number) - Page number
- `limit` (number) - Results per page

---

## Database Schema

### np_admin_users

Admin users separate from regular application users.

```sql
CREATE TABLE np_admin_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,  -- 'super_admin', 'admin', 'moderator'
    active BOOLEAN DEFAULT TRUE,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_np_admin_users_email ON np_admin_users(email);
CREATE INDEX idx_np_admin_users_role ON np_admin_users(role);
CREATE INDEX idx_np_admin_users_source_account ON np_admin_users(source_account_id);
```

**Columns:**
- `id` - UUID primary key
- `source_account_id` - Multi-app isolation identifier
- `email` - Admin email (unique)
- `password_hash` - bcrypt password hash
- `role` - Admin role: `super_admin`, `admin`, `moderator`
- `active` - Whether admin account is active
- `last_login_at` - Last login timestamp
- `created_at` - Account creation timestamp
- `updated_at` - Last update timestamp

### np_admin_audit_log

Immutable audit trail for all admin actions.

```sql
CREATE TABLE np_admin_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id VARCHAR(128) DEFAULT 'primary',
    admin_user_id UUID REFERENCES np_admin_users(id),
    action TEXT NOT NULL,
    entity_type TEXT,
    entity_id UUID,
    details JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_np_admin_audit_admin_user ON np_admin_audit_log(admin_user_id);
CREATE INDEX idx_np_admin_audit_action ON np_admin_audit_log(action);
CREATE INDEX idx_np_admin_audit_created ON np_admin_audit_log(created_at DESC);
CREATE INDEX idx_np_admin_audit_source_account ON np_admin_audit_log(source_account_id);
```

**Columns:**
- `id` - UUID primary key
- `source_account_id` - Multi-app isolation identifier
- `admin_user_id` - Reference to admin who performed action
- `action` - Action performed (e.g., `user.ban`, `content.delete`)
- `entity_type` - Type of entity affected (e.g., `user`, `content`)
- `entity_id` - ID of affected entity
- `details` - Additional action details in JSON format
- `ip_address` - IP address of admin
- `user_agent` - Browser/client user agent
- `created_at` - Timestamp (immutable)

---

## Usage Examples

### Start Server

```bash
cd plugins/admin-api/ts
npm install
npm run dev
```

Server runs on port 3214 by default.

### Check Health

```bash
curl http://localhost:3214/health
```

Response:
```json
{
  "status": "ok",
  "timestamp": "2026-02-14T12:00:00Z"
}
```

### View System Health

```bash
curl http://localhost:3214/api/admin/health
```

Response:
```json
{
  "overall": "healthy",
  "database": {
    "status": "healthy",
    "latency_ms": 2.5
  },
  "storage": {
    "status": "healthy",
    "used_gb": 45.2,
    "total_gb": 500
  },
  "queue": {
    "status": "healthy",
    "active_jobs": 3,
    "failed_jobs": 0
  }
}
```

### List Users

```bash
curl http://localhost:3214/api/admin/users?page=1&limit=20
```

### Ban User

```bash
curl -X PATCH http://localhost:3214/api/admin/users/123e4567-e89b-12d3-a456-426614174000/ban \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{"reason": "Spam violation"}'
```

### View Metrics

```bash
curl http://localhost:3214/api/admin/metrics/users
```

Response:
```json
{
  "dau": 1250,
  "wau": 5430,
  "mau": 18920,
  "new_signups_today": 87,
  "churn_rate": 2.3
}
```

### View Audit Log

```bash
curl "http://localhost:3214/api/admin/audit-log?action=user.ban&page=1&limit=50"
```

Response:
```json
{
  "total": 156,
  "page": 1,
  "limit": 50,
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "admin_user_id": "123e4567-e89b-12d3-a456-426614174000",
      "action": "user.ban",
      "entity_type": "user",
      "entity_id": "789e4567-e89b-12d3-a456-426614174000",
      "details": {
        "reason": "Spam violation",
        "expires_at": null
      },
      "ip_address": "192.168.1.100",
      "user_agent": "Mozilla/5.0...",
      "created_at": "2026-02-14T10:30:00Z"
    }
  ]
}
```

---

## Multi-App Support

The Admin API plugin supports full multi-tenant isolation using the `source_account_id` column.

### Configuration

```bash
# Set the account identifier
export SOURCE_ACCOUNT_ID="tenant-123"

# Or use default
export SOURCE_ACCOUNT_ID="primary"
```

### Isolation Strategy

- All tables include `source_account_id` column
- Indexes created on `source_account_id` for performance
- Queries automatically filter by `source_account_id`
- Audit log includes tenant context

### Example: Multi-Tenant Query

```sql
-- Get admin users for specific tenant
SELECT * FROM np_admin_users
WHERE source_account_id = 'tenant-123';

-- Get audit log for specific tenant
SELECT * FROM np_admin_audit_log
WHERE source_account_id = 'tenant-123'
ORDER BY created_at DESC;
```

---

## Development

### Install Dependencies

```bash
cd plugins/admin-api/ts
npm install
```

### Type Check

```bash
npm run typecheck
```

### Build

```bash
npm run build
```

### Development Mode

```bash
npm run dev
```

Runs with hot reload on port 3214.

### Production Mode

```bash
npm start
```

---

## License

Source-Available
