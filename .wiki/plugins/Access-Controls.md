# Access Controls Plugin

Role-based and attribute-based access control (RBAC + ABAC) for nself applications. Define roles, permissions, policies, and authorize requests with wildcard matching, role hierarchies, and conditional evaluation.

| Property | Value |
|----------|-------|
| **Port** | `3027` |
| **Category** | `authentication` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
# Initialize the database schema
nself plugin run access-controls init

# Start the server
nself plugin run access-controls server

# Or with custom port
nself plugin run access-controls server --port 3027
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
| `ACL_PLUGIN_PORT` | `3027` | Server port |
| `ACL_PLUGIN_HOST` | `0.0.0.0` | Server host |
| `ACL_CACHE_TTL_SECONDS` | `300` | Permission cache TTL (seconds) |
| `ACL_MAX_ROLE_DEPTH` | `10` | Maximum role hierarchy depth |
| `ACL_DEFAULT_DENY` | `true` | Deny by default when no matching permission |
| `ACL_API_KEY` | - | API key for authentication |
| `ACL_RATE_LIMIT_MAX` | `200` | Max requests per window |
| `ACL_RATE_LIMIT_WINDOW_MS` | `60000` | Rate limit window (ms) |

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (creates all tables and indexes) |
| `server` | Start the HTTP API server (`-p`/`--port`, `-h`/`--host`) |
| `status` | Show role/permission/policy counts and statistics |
| `roles` | Manage roles: `list`, `create <name>`, `show <id>`, `delete <id>` |
| `permissions` | Manage permissions: `list`, `create <resource> <action>`, `delete <id>` |
| `users` | Manage user roles: `list`, `assign <userId> <roleId>`, `remove <userId> <roleId>` |
| `authorize` | Check authorization: `<userId> <resource> <action>` (with optional `--context`) |
| `policies` | Manage ABAC policies: `list`, `create`, `delete <id>` |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (verifies DB connection) |
| `GET` | `/live` | Liveness check with stats, uptime, memory |
| `GET` | `/status` | Plugin status with role/permission/policy counts |

### Roles

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/roles` | Create a role (body: `name`, `description?`, `parent_role_id?`, `is_system?`, `metadata?`) |
| `GET` | `/v1/roles` | List all roles |
| `GET` | `/v1/roles/:id` | Get role by ID |
| `PUT` | `/v1/roles/:id` | Update role |
| `DELETE` | `/v1/roles/:id` | Delete role |
| `GET` | `/v1/roles/hierarchy` | Get full role hierarchy tree |

### Permissions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/permissions` | Create permission (body: `resource`, `action`, `description?`, `is_system?`) |
| `GET` | `/v1/permissions` | List all permissions (query: `resource?`, `action?`) |
| `GET` | `/v1/permissions/:id` | Get permission by ID |
| `DELETE` | `/v1/permissions/:id` | Delete permission |

### Role-Permission Assignments

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/roles/:id/permissions` | Assign permissions to role (body: `permission_ids[]`) |
| `GET` | `/v1/roles/:id/permissions` | List permissions for role |
| `DELETE` | `/v1/roles/:id/permissions/:permId` | Remove permission from role |

### User-Role Assignments

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/users/:userId/roles` | Assign role to user (body: `role_id`, `expires_at?`) |
| `GET` | `/v1/users/:userId/roles` | List roles for user |
| `DELETE` | `/v1/users/:userId/roles/:roleId` | Remove role from user |
| `GET` | `/v1/users/:userId/permissions` | Get effective permissions (includes inherited via hierarchy) |

### Authorization

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/authorize` | Authorize a single request (body: `user_id`, `resource`, `action`, `context?`) |
| `POST` | `/v1/authorize/batch` | Authorize multiple requests at once (body: `requests[]`) |

### Policies (ABAC)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/policies` | Create ABAC policy (body: `name`, `resource`, `action`, `effect`, `conditions`, `priority?`) |
| `GET` | `/v1/policies` | List all policies (query: `resource?`, `action?`, `effect?`) |
| `GET` | `/v1/policies/:id` | Get policy by ID |
| `PUT` | `/v1/policies/:id` | Update policy |
| `DELETE` | `/v1/policies/:id` | Delete policy |

### Cache Management

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/cache/invalidate` | Invalidate permission cache (body: `user_id?` for specific user, omit for all) |
| `GET` | `/v1/cache/stats` | Get cache hit/miss statistics |

---

## Authorization Engine

The authorization engine (`authz.ts`) evaluates requests in this order:

1. **Cache lookup** -- checks TTL-based in-memory cache for user permissions
2. **RBAC check** -- resolves effective permissions through role hierarchy (recursive CTE)
3. **Wildcard matching** -- `users:*` matches `users:read`, `users:write`, etc.
4. **ABAC policy evaluation** -- evaluates attribute-based policies sorted by priority

### Condition Operators

ABAC policies support these condition operators in the `context` object:

| Operator | Description | Example |
|----------|-------------|---------|
| `$eq` | Equals | `{ "department": { "$eq": "engineering" } }` |
| `$ne` | Not equals | `{ "role": { "$ne": "guest" } }` |
| `$in` | In array | `{ "region": { "$in": ["us-east", "us-west"] } }` |
| `$nin` | Not in array | `{ "status": { "$nin": ["banned", "suspended"] } }` |
| `$gt` | Greater than | `{ "level": { "$gt": 5 } }` |
| `$gte` | Greater than or equal | `{ "age": { "$gte": 18 } }` |
| `$lt` | Less than | `{ "attempts": { "$lt": 3 } }` |
| `$lte` | Less than or equal | `{ "risk_score": { "$lte": 50 } }` |

### Authorization Response

```json
{
  "authorized": true,
  "user_id": "user-123",
  "resource": "documents",
  "action": "read",
  "matched_permission": "documents:read",
  "matched_via": "rbac",
  "role_path": ["admin", "editor"],
  "cached": false
}
```

---

## Database Schema

### `acl_roles`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Role ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(100)` | Unique role name |
| `description` | `TEXT` | Optional description |
| `parent_role_id` | `UUID` (FK) | Parent role for hierarchy |
| `is_system` | `BOOLEAN` | System-managed flag |
| `metadata` | `JSONB` | Arbitrary metadata |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update timestamp |

### `acl_permissions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Permission ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `resource` | `VARCHAR(255)` | Resource identifier (e.g., `documents`, `users`) |
| `action` | `VARCHAR(100)` | Action identifier (e.g., `read`, `write`, `*`) |
| `description` | `TEXT` | Optional description |
| `is_system` | `BOOLEAN` | System-managed flag |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `acl_role_permissions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `role_id` | `UUID` (FK) | References `acl_roles` |
| `permission_id` | `UUID` (FK) | References `acl_permissions` |
| `granted_at` | `TIMESTAMPTZ` | When permission was granted |

### `acl_user_roles`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Record ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `user_id` | `VARCHAR(255)` | External user identifier |
| `role_id` | `UUID` (FK) | References `acl_roles` |
| `granted_by` | `VARCHAR(255)` | Who granted the role |
| `expires_at` | `TIMESTAMPTZ` | Optional expiration |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |

### `acl_policies`

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Policy ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `name` | `VARCHAR(255)` | Policy name |
| `description` | `TEXT` | Policy description |
| `resource` | `VARCHAR(255)` | Target resource pattern |
| `action` | `VARCHAR(100)` | Target action pattern |
| `effect` | `VARCHAR(10)` | `allow` or `deny` |
| `conditions` | `JSONB` | Condition operators and values |
| `priority` | `INTEGER` | Evaluation priority (higher = first) |
| `enabled` | `BOOLEAN` | Whether policy is active |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update timestamp |

### `acl_webhook_events`

Standard webhook event tracking table with `id`, `source_account_id`, `event_type`, `payload` (JSONB), `processed`, `processed_at`, `error`, `created_at`.

---

## Role Hierarchy

Roles support single-parent inheritance. When evaluating permissions, the engine walks up the role tree using a recursive CTE:

```
admin
  -> editor
    -> viewer
```

A user with the `editor` role inherits all permissions from `viewer`. A user with `admin` inherits from both `editor` and `viewer`.

Maximum hierarchy depth is configurable via `ACL_MAX_ROLE_DEPTH` (default: 10).

---

## Troubleshooting

**Authorization always denied** -- Check `ACL_DEFAULT_DENY` is set as intended, verify the user has roles assigned, verify the role has permissions assigned, and check role hierarchy via `parent_role_id`.

**Cache returning stale results** -- Use `POST /v1/cache/invalidate` to clear cache, or reduce `ACL_CACHE_TTL_SECONDS`.

**Wildcard permissions not matching** -- Wildcard `*` matches any single segment: `users:*` matches `users:read` but not `users:admin:read`.
