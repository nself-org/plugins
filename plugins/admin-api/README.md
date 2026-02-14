# Admin API Plugin

Comprehensive admin dashboard backend for nself applications.

## Features

- **System Health Monitoring** - CPU, memory, disk, database, storage, and service health
- **User Management** - List, ban, unban, and delete users
- **Metrics** - DAU/WAU/MAU, content metrics, playback metrics
- **Audit Logging** - Immutable audit trail for all admin actions
- **Multi-App Support** - Full tenant isolation with `source_account_id`

## Installation

```bash
nself plugin install admin-api
```

## Configuration

### Required Environment Variables

```bash
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
```

### Optional Environment Variables

```bash
ADMIN_API_PORT=3214
ADMIN_JWT_SECRET=your-secret-key
ADMIN_SESSION_TIMEOUT_MINUTES=60
ADMIN_METRICS_COLLECTION_INTERVAL_SECONDS=60
ADMIN_API_KEY=your-api-key
ADMIN_RATE_LIMIT_MAX=100
ADMIN_RATE_LIMIT_WINDOW_MS=60000
```

## API Endpoints

### System Health

```
GET /api/admin/health                  # Overall system health
GET /api/admin/health/database         # Database health
GET /api/admin/health/storage          # Storage health
GET /api/admin/health/queue            # Queue health
GET /api/admin/health/services         # Service status
```

### User Management

```
GET    /api/admin/users                # List users
GET    /api/admin/users/:id            # Get user details
PATCH  /api/admin/users/:id/ban        # Ban user
PATCH  /api/admin/users/:id/unban      # Unban user
DELETE /api/admin/users/:id            # Delete user
```

### Content Management

```
GET    /api/admin/content              # List content
DELETE /api/admin/content/:id          # Delete content
```

### Metrics

```
GET /api/admin/metrics/users           # User metrics (DAU/WAU/MAU)
GET /api/admin/metrics/content         # Content metrics
GET /api/admin/metrics/playback        # Playback metrics
```

### Alerts

```
GET  /api/admin/alerts                 # List alerts
POST /api/admin/alerts/:id/acknowledge # Acknowledge alert
```

### Audit Log

```
GET /api/admin/audit-log               # View audit log
```

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
```

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
```

## Usage

### Start Server

```bash
cd plugins/admin-api/ts
npm install
npm run dev
```

### Check Health

```bash
curl http://localhost:3214/health
```

### View System Health

```bash
curl http://localhost:3214/api/admin/health
```

### List Users

```bash
curl http://localhost:3214/api/admin/users
```

### Ban User

```bash
curl -X PATCH http://localhost:3214/api/admin/users/{user-id}/ban \
  -H "Content-Type: application/json" \
  -d '{"reason": "Terms violation"}'
```

### View Metrics

```bash
curl http://localhost:3214/api/admin/metrics/users
```

### View Audit Log

```bash
curl http://localhost:3214/api/admin/audit-log
```

## Development

```bash
# Install dependencies
npm install

# Type check
npm run typecheck

# Build
npm run build

# Development mode
npm run dev

# Production mode
npm start
```

## License

Source-Available
