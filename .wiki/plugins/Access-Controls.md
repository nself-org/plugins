# Access Controls Plugin

Role-based and attribute-based access control (RBAC + ABAC) with policy engine for nself. Implement fine-grained authorization with hierarchical roles, dynamic permissions, and flexible policy evaluation.

---

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [REST API](#rest-api)
- [Database Schema](#database-schema)
- [RBAC (Role-Based Access Control)](#rbac-role-based-access-control)
- [ABAC (Attribute-Based Access Control)](#abac-attribute-based-access-control)
- [Policy Engine](#policy-engine)
- [Role Hierarchy](#role-hierarchy)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Access Controls plugin provides a complete authorization system combining Role-Based Access Control (RBAC) and Attribute-Based Access Control (ABAC). Define roles, assign permissions, create hierarchies, and write dynamic policies for complex authorization scenarios.

### Key Features

- **RBAC** - Assign permissions to roles and roles to users
- **ABAC** - Dynamic policy evaluation based on user, resource, and environment attributes
- **Role Hierarchy** - Inherit permissions from parent roles
- **Permission Management** - Define granular resource-action permissions
- **Policy Engine** - Evaluate complex authorization rules with JSON-based policies
- **Caching** - High-performance authorization checks with configurable TTL
- **Audit Trail** - Track all authorization decisions and policy changes
- **Default Deny** - Secure by default with explicit allow requirements
- **Multi-Account Support** - Isolate access control data per account
- **REST API** - Programmatic access to all authorization functions

### Synced Resources

| Resource | Description | Table |
|----------|-------------|-------|
| Roles | User roles with hierarchical relationships | `acl_roles` |
| Permissions | Resource-action permission definitions | `acl_permissions` |
| Role Permissions | Role-to-permission assignments | `acl_role_permissions` |
| User Roles | User-to-role assignments | `acl_user_roles` |
| Policies | ABAC policy definitions | `acl_policies` |
| Webhook Events | Authorization event log | `acl_webhook_events` |

---

## Quick Start

```bash
# Install the plugin
nself plugin install access-controls

# Configure environment
echo "DATABASE_URL=postgresql://user:pass@localhost:5432/nself" >> .env
echo "ACL_PLUGIN_PORT=3027" >> .env

# Initialize database schema
nself plugin access-controls init

# Start server
nself plugin access-controls server --port 3027

# Create a role
curl -X POST http://localhost:3027/api/roles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "admin",
    "description": "Administrator role with full access"
  }'

# Create permissions
curl -X POST http://localhost:3027/api/permissions \
  -H "Content-Type: application/json" \
  -d '{
    "resource": "users",
    "action": "create"
  }'

# Assign permission to role
curl -X POST http://localhost:3027/api/roles/admin/permissions \
  -H "Content-Type: application/json" \
  -d '{
    "resource": "users",
    "action": "create"
  }'

# Assign role to user
curl -X POST http://localhost:3027/api/users/user_123/roles \
  -H "Content-Type: application/json" \
  -d '{
    "role": "admin"
  }'

# Check authorization
curl -X POST http://localhost:3027/api/authorize \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user_123",
    "resource": "users",
    "action": "create"
  }'
```

---

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | - | PostgreSQL connection string |
| `ACL_PLUGIN_PORT` | No | `3027` | HTTP server port |
| `ACL_CACHE_TTL_SECONDS` | No | `300` | Cache TTL for authorization decisions (5 min) |
| `ACL_MAX_ROLE_DEPTH` | No | `10` | Maximum role hierarchy depth |
| `ACL_DEFAULT_DENY` | No | `true` | Default deny unless explicitly allowed |
| `ACL_API_KEY` | No | - | API key for authentication (optional) |
| `ACL_RATE_LIMIT_MAX` | No | `1000` | Max requests per window |
| `ACL_RATE_LIMIT_WINDOW_MS` | No | `60000` | Rate limit window in milliseconds |

### Example Configuration

```bash
# .env file
DATABASE_URL=postgresql://localhost:5432/nself
ACL_PLUGIN_PORT=3027
ACL_CACHE_TTL_SECONDS=600
ACL_MAX_ROLE_DEPTH=5
ACL_DEFAULT_DENY=true
ACL_API_KEY=your_acl_api_key_here
```

---

## CLI Commands

### init

Initialize the access control database schema.

```bash
nself plugin access-controls init
```

### server

Start the access control API server.

```bash
# Start with default port
nself plugin access-controls server

# Start with custom port
nself plugin access-controls server --port 3500
```

**Options:**
- `-p, --port <port>` - Server port (default: 3027)
- `-h, --host <host>` - Server host (default: 0.0.0.0)

### authorize

Check if a user has permission to perform an action.

```bash
# Simple authorization check
nself plugin access-controls authorize \
  --user user_123 \
  --resource users \
  --action create

# With resource ID
nself plugin access-controls authorize \
  --user user_123 \
  --resource users \
  --action update \
  --resource-id user_456
```

**Output:**
```
✓ Authorized: user_123 can create users
Reason: User has role 'admin' with permission 'users:create'
```

### roles

Manage roles.

```bash
# List all roles
nself plugin access-controls roles list

# Create role
nself plugin access-controls roles create \
  --name editor \
  --description "Content editor role" \
  --parent viewer

# Get role details
nself plugin access-controls roles get admin

# Update role
nself plugin access-controls roles update editor \
  --description "Updated description"

# Delete role
nself plugin access-controls roles delete editor

# List role hierarchy
nself plugin access-controls roles hierarchy
```

### permissions

Manage permissions.

```bash
# List all permissions
nself plugin access-controls permissions list

# Create permission
nself plugin access-controls permissions create \
  --resource posts \
  --action publish

# Grant permission to role
nself plugin access-controls permissions grant \
  --role editor \
  --resource posts \
  --action publish

# Revoke permission from role
nself plugin access-controls permissions revoke \
  --role editor \
  --resource posts \
  --action delete

# List role permissions
nself plugin access-controls permissions list-by-role editor
```

### policies

Manage ABAC policies.

```bash
# List all policies
nself plugin access-controls policies list

# Create policy
nself plugin access-controls policies create \
  --name business_hours_only \
  --description "Allow access only during business hours" \
  --rule-file policy.json

# Example policy.json:
{
  "effect": "allow",
  "conditions": {
    "hourOfDay": {"gte": 9, "lte": 17},
    "dayOfWeek": {"in": [1,2,3,4,5]}
  }
}

# Update policy
nself plugin access-controls policies update business_hours_only \
  --rule-file updated-policy.json

# Delete policy
nself plugin access-controls policies delete business_hours_only

# Test policy
nself plugin access-controls policies test business_hours_only \
  --context '{"hourOfDay":14,"dayOfWeek":3}'
```

### users

Manage user role assignments.

```bash
# List user's roles
nself plugin access-controls users roles user_123

# Assign role to user
nself plugin access-controls users assign \
  --user user_123 \
  --role editor

# Remove role from user
nself plugin access-controls users unassign \
  --user user_123 \
  --role editor

# List all permissions for user (including inherited)
nself plugin access-controls users permissions user_123
```

### stats

View access control statistics.

```bash
nself plugin access-controls stats
```

**Output:**
```
=== Access Control Statistics ===

Roles: 5
  - admin: 2 users, 15 permissions
  - editor: 8 users, 8 permissions
  - viewer: 25 users, 3 permissions

Permissions: 24
Users with Roles: 35
Policies: 3

Cache:
  Hit Rate: 94.2%
  Entries: 1,234
```

---

## REST API

### Health Check Endpoints

#### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "plugin": "access-controls",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

#### GET /ready

Readiness check with database connectivity.

**Response:**
```json
{
  "ready": true,
  "plugin": "access-controls",
  "timestamp": "2026-02-11T10:00:00.000Z"
}
```

### Authorization

#### POST /api/authorize

Check if user is authorized to perform action on resource.

**Request Body:**
```json
{
  "userId": "user_123",
  "resource": "posts",
  "action": "publish",
  "resourceId": "post_456",
  "context": {
    "ipAddress": "192.168.1.1",
    "userAgent": "Mozilla/5.0...",
    "timestamp": "2026-02-11T10:00:00Z"
  }
}
```

**Response:** `200 OK`
```json
{
  "authorized": true,
  "reason": "User has role 'editor' with permission 'posts:publish'",
  "matchedPolicies": [],
  "cached": false
}
```

Or if denied:
```json
{
  "authorized": false,
  "reason": "No matching permissions or policies",
  "cached": false
}
```

#### POST /api/authorize/batch

Check multiple authorizations in batch.

**Request Body:**
```json
{
  "userId": "user_123",
  "checks": [
    {"resource": "posts", "action": "create"},
    {"resource": "posts", "action": "publish"},
    {"resource": "users", "action": "delete"}
  ]
}
```

**Response:** `200 OK`
```json
{
  "results": [
    {
      "resource": "posts",
      "action": "create",
      "authorized": true,
      "reason": "Role permission"
    },
    {
      "resource": "posts",
      "action": "publish",
      "authorized": true,
      "reason": "Role permission"
    },
    {
      "resource": "users",
      "action": "delete",
      "authorized": false,
      "reason": "No matching permissions"
    }
  ]
}
```

### Role Management

#### POST /api/roles

Create a new role.

**Request Body:**
```json
{
  "name": "editor",
  "description": "Content editor role",
  "parentRole": "viewer",
  "metadata": {
    "department": "content",
    "level": 2
  }
}
```

**Response:** `201 Created`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "editor",
  "description": "Content editor role",
  "parentRole": "viewer",
  "metadata": {
    "department": "content",
    "level": 2
  },
  "createdAt": "2026-02-11T10:00:00.000Z"
}
```

#### GET /api/roles

List all roles.

**Response:** `200 OK`
```json
{
  "roles": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "admin",
      "description": "Administrator role",
      "parentRole": null,
      "userCount": 2,
      "permissionCount": 15,
      "createdAt": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 1
}
```

#### GET /api/roles/:name

Get role details.

**Response:** `200 OK`
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "editor",
  "description": "Content editor role",
  "parentRole": "viewer",
  "permissions": [
    {"resource": "posts", "action": "create"},
    {"resource": "posts", "action": "update"},
    {"resource": "posts", "action": "publish"}
  ],
  "users": ["user_123", "user_456"],
  "metadata": {},
  "createdAt": "2026-02-11T10:00:00.000Z"
}
```

#### PATCH /api/roles/:name

Update a role.

**Request Body:**
```json
{
  "description": "Updated description",
  "parentRole": "member"
}
```

**Response:** `200 OK`

#### DELETE /api/roles/:name

Delete a role.

**Response:** `204 No Content`

#### GET /api/roles/hierarchy

Get role hierarchy tree.

**Response:** `200 OK`
```json
{
  "hierarchy": [
    {
      "name": "admin",
      "children": [
        {
          "name": "editor",
          "children": [
            {
              "name": "viewer",
              "children": []
            }
          ]
        }
      ]
    }
  ]
}
```

### Permission Management

#### POST /api/permissions

Create a permission.

**Request Body:**
```json
{
  "resource": "posts",
  "action": "publish",
  "description": "Publish posts to production"
}
```

**Response:** `201 Created`

#### GET /api/permissions

List all permissions.

**Response:** `200 OK`
```json
{
  "permissions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440000",
      "resource": "posts",
      "action": "create",
      "description": "Create new posts",
      "createdAt": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 1
}
```

#### POST /api/roles/:role/permissions

Grant permission to role.

**Request Body:**
```json
{
  "resource": "posts",
  "action": "publish"
}
```

**Response:** `200 OK`

#### DELETE /api/roles/:role/permissions

Revoke permission from role.

**Request Body:**
```json
{
  "resource": "posts",
  "action": "delete"
}
```

**Response:** `204 No Content`

#### GET /api/roles/:role/permissions

List role permissions (including inherited).

**Response:** `200 OK`
```json
{
  "role": "editor",
  "permissions": [
    {
      "resource": "posts",
      "action": "create",
      "inherited": false
    },
    {
      "resource": "posts",
      "action": "read",
      "inherited": true,
      "inheritedFrom": "viewer"
    }
  ]
}
```

### User Role Assignment

#### POST /api/users/:userId/roles

Assign role to user.

**Request Body:**
```json
{
  "role": "editor",
  "expiresAt": "2026-12-31T23:59:59Z"
}
```

**Response:** `200 OK`

#### DELETE /api/users/:userId/roles/:role

Remove role from user.

**Response:** `204 No Content`

#### GET /api/users/:userId/roles

List user's roles.

**Response:** `200 OK`
```json
{
  "userId": "user_123",
  "roles": [
    {
      "role": "editor",
      "assignedAt": "2026-02-11T10:00:00.000Z",
      "expiresAt": null
    }
  ]
}
```

#### GET /api/users/:userId/permissions

List all permissions for user (including inherited).

**Response:** `200 OK`
```json
{
  "userId": "user_123",
  "permissions": [
    {
      "resource": "posts",
      "action": "create",
      "source": "role:editor"
    },
    {
      "resource": "posts",
      "action": "read",
      "source": "role:viewer (inherited)"
    }
  ],
  "total": 2
}
```

### Policy Management

#### POST /api/policies

Create an ABAC policy.

**Request Body:**
```json
{
  "name": "business_hours_only",
  "description": "Allow access only during business hours",
  "effect": "allow",
  "resource": "*",
  "action": "*",
  "conditions": {
    "hourOfDay": {"gte": 9, "lte": 17},
    "dayOfWeek": {"in": [1, 2, 3, 4, 5]}
  },
  "priority": 100
}
```

**Response:** `201 Created`

#### GET /api/policies

List all policies.

**Response:** `200 OK`
```json
{
  "policies": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440000",
      "name": "business_hours_only",
      "description": "Allow access only during business hours",
      "effect": "allow",
      "resource": "*",
      "action": "*",
      "priority": 100,
      "enabled": true,
      "createdAt": "2026-02-11T10:00:00.000Z"
    }
  ],
  "total": 1
}
```

#### GET /api/policies/:name

Get policy details.

**Response:** `200 OK`
```json
{
  "id": "770e8400-e29b-41d4-a716-446655440000",
  "name": "business_hours_only",
  "description": "Allow access only during business hours",
  "effect": "allow",
  "resource": "*",
  "action": "*",
  "conditions": {
    "hourOfDay": {"gte": 9, "lte": 17},
    "dayOfWeek": {"in": [1, 2, 3, 4, 5]}
  },
  "priority": 100,
  "enabled": true,
  "evaluationCount": 1234,
  "createdAt": "2026-02-11T10:00:00.000Z"
}
```

#### PATCH /api/policies/:name

Update a policy.

**Request Body:**
```json
{
  "enabled": false,
  "priority": 50
}
```

**Response:** `200 OK`

#### DELETE /api/policies/:name

Delete a policy.

**Response:** `204 No Content`

#### POST /api/policies/:name/evaluate

Test policy evaluation.

**Request Body:**
```json
{
  "context": {
    "userId": "user_123",
    "hourOfDay": 14,
    "dayOfWeek": 3,
    "ipAddress": "192.168.1.1"
  }
}
```

**Response:** `200 OK`
```json
{
  "policy": "business_hours_only",
  "matched": true,
  "effect": "allow",
  "reason": "All conditions satisfied"
}
```

---

## Database Schema

### acl_roles

Stores role definitions.

```sql
CREATE TABLE acl_roles (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  parent_role VARCHAR(255),
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX idx_acl_roles_source_account ON acl_roles(source_account_id);
CREATE INDEX idx_acl_roles_name ON acl_roles(name);
CREATE INDEX idx_acl_roles_parent ON acl_roles(parent_role);
```

### acl_permissions

Stores permission definitions.

```sql
CREATE TABLE acl_permissions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  resource VARCHAR(255) NOT NULL,
  action VARCHAR(255) NOT NULL,
  description TEXT,
  metadata JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, resource, action)
);

CREATE INDEX idx_acl_permissions_source_account ON acl_permissions(source_account_id);
CREATE INDEX idx_acl_permissions_resource ON acl_permissions(resource);
CREATE INDEX idx_acl_permissions_action ON acl_permissions(action);
```

### acl_role_permissions

Maps permissions to roles.

```sql
CREATE TABLE acl_role_permissions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  role_id UUID REFERENCES acl_roles(id) ON DELETE CASCADE,
  permission_id UUID REFERENCES acl_permissions(id) ON DELETE CASCADE,
  granted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(role_id, permission_id)
);

CREATE INDEX idx_acl_role_permissions_source_account ON acl_role_permissions(source_account_id);
CREATE INDEX idx_acl_role_permissions_role ON acl_role_permissions(role_id);
CREATE INDEX idx_acl_role_permissions_permission ON acl_role_permissions(permission_id);
```

### acl_user_roles

Maps roles to users.

```sql
CREATE TABLE acl_user_roles (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  user_id VARCHAR(255) NOT NULL,
  role_id UUID REFERENCES acl_roles(id) ON DELETE CASCADE,
  assigned_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  expires_at TIMESTAMP WITH TIME ZONE,
  UNIQUE(source_account_id, user_id, role_id)
);

CREATE INDEX idx_acl_user_roles_source_account ON acl_user_roles(source_account_id);
CREATE INDEX idx_acl_user_roles_user ON acl_user_roles(user_id);
CREATE INDEX idx_acl_user_roles_role ON acl_user_roles(role_id);
CREATE INDEX idx_acl_user_roles_expires ON acl_user_roles(expires_at) WHERE expires_at IS NOT NULL;
```

### acl_policies

Stores ABAC policy definitions.

```sql
CREATE TABLE acl_policies (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  name VARCHAR(255) NOT NULL,
  description TEXT,
  effect VARCHAR(32) NOT NULL,
  resource VARCHAR(255) DEFAULT '*',
  action VARCHAR(255) DEFAULT '*',
  conditions JSONB DEFAULT '{}',
  priority INTEGER DEFAULT 0,
  enabled BOOLEAN DEFAULT TRUE,
  evaluation_count INTEGER DEFAULT 0,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  UNIQUE(source_account_id, name)
);

CREATE INDEX idx_acl_policies_source_account ON acl_policies(source_account_id);
CREATE INDEX idx_acl_policies_name ON acl_policies(name);
CREATE INDEX idx_acl_policies_resource ON acl_policies(resource);
CREATE INDEX idx_acl_policies_action ON acl_policies(action);
CREATE INDEX idx_acl_policies_priority ON acl_policies(priority DESC);
CREATE INDEX idx_acl_policies_enabled ON acl_policies(enabled);
```

### acl_webhook_events

Tracks authorization events for audit.

```sql
CREATE TABLE acl_webhook_events (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  source_account_id VARCHAR(128) DEFAULT 'primary',
  event_type VARCHAR(128) NOT NULL,
  user_id VARCHAR(255),
  resource VARCHAR(255),
  action VARCHAR(255),
  authorized BOOLEAN,
  reason TEXT,
  context JSONB DEFAULT '{}',
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_acl_webhook_events_source_account ON acl_webhook_events(source_account_id);
CREATE INDEX idx_acl_webhook_events_type ON acl_webhook_events(event_type);
CREATE INDEX idx_acl_webhook_events_user ON acl_webhook_events(user_id);
CREATE INDEX idx_acl_webhook_events_created ON acl_webhook_events(created_at DESC);
```

---

## RBAC (Role-Based Access Control)

### Role Hierarchy

Roles can inherit permissions from parent roles:

```
admin (all permissions)
  └── editor (content permissions)
      └── viewer (read-only permissions)
```

When checking authorization, the system traverses up the hierarchy until a matching permission is found.

### Creating Role Hierarchies

```bash
# Create base viewer role
curl -X POST http://localhost:3027/api/roles \
  -d '{"name":"viewer","description":"Read-only access"}'

# Create editor inheriting from viewer
curl -X POST http://localhost:3027/api/roles \
  -d '{"name":"editor","description":"Content editor","parentRole":"viewer"}'

# Create admin inheriting from editor
curl -X POST http://localhost:3027/api/roles \
  -d '{"name":"admin","description":"Administrator","parentRole":"editor"}'
```

### Permission Naming Convention

Use `resource:action` format:
- `posts:create`
- `posts:read`
- `posts:update`
- `posts:delete`
- `users:manage`
- `settings:admin`

---

## ABAC (Attribute-Based Access Control)

### Policy Conditions

Policies support various condition operators:

**Comparison:**
- `eq` - Equal
- `ne` - Not equal
- `gt` - Greater than
- `gte` - Greater than or equal
- `lt` - Less than
- `lte` - Less than or equal

**Set operations:**
- `in` - Value in array
- `notIn` - Value not in array

**String operations:**
- `contains` - String contains substring
- `startsWith` - String starts with
- `endsWith` - String ends with

**Boolean:**
- `exists` - Attribute exists

### Example Policies

**Time-based access:**
```json
{
  "name": "business_hours",
  "effect": "allow",
  "conditions": {
    "hourOfDay": {"gte": 9, "lte": 17},
    "dayOfWeek": {"in": [1,2,3,4,5]}
  }
}
```

**IP-based access:**
```json
{
  "name": "internal_network_only",
  "effect": "allow",
  "conditions": {
    "ipAddress": {"startsWith": "192.168."}
  }
}
```

**Department-based access:**
```json
{
  "name": "hr_documents",
  "effect": "allow",
  "resource": "documents:hr",
  "conditions": {
    "userDepartment": {"eq": "human_resources"}
  }
}
```

---

## Policy Engine

### Evaluation Order

1. **Explicit Deny** - Any deny policy immediately denies access
2. **Role Permissions** - Check RBAC permissions
3. **Allow Policies** - Check ABAC allow policies by priority
4. **Default Deny** - Deny if no explicit allow (if ACL_DEFAULT_DENY=true)

### Policy Priority

Higher priority policies are evaluated first:
- Priority 1000+ - Critical overrides
- Priority 100-999 - Standard policies
- Priority 0-99 - Low priority policies

---

## Role Hierarchy

### Maximum Depth

Role hierarchies are limited by `ACL_MAX_ROLE_DEPTH` (default: 10) to prevent infinite recursion.

### Circular References

The system detects and prevents circular role references:

```
admin → editor → viewer → admin (INVALID)
```

### Permission Inheritance

Child roles inherit all permissions from parent roles:

```
viewer: [posts:read, comments:read]
editor: [posts:create, posts:update] + inherited from viewer
admin: [posts:delete, users:manage] + inherited from editor
```

---

## Examples

### Example 1: Simple RBAC Setup

```bash
# Create roles
curl -X POST http://localhost:3027/api/roles \
  -d '{"name":"admin","description":"Full access"}'

curl -X POST http://localhost:3027/api/roles \
  -d '{"name":"user","description":"Regular user"}'

# Create permissions
curl -X POST http://localhost:3027/api/permissions \
  -d '{"resource":"posts","action":"create"}'

curl -X POST http://localhost:3027/api/permissions \
  -d '{"resource":"posts","action":"delete"}'

# Assign permissions
curl -X POST http://localhost:3027/api/roles/admin/permissions \
  -d '{"resource":"posts","action":"delete"}'

curl -X POST http://localhost:3027/api/roles/user/permissions \
  -d '{"resource":"posts","action":"create"}'

# Assign roles to users
curl -X POST http://localhost:3027/api/users/user_123/roles \
  -d '{"role":"user"}'

# Check authorization
curl -X POST http://localhost:3027/api/authorize \
  -d '{"userId":"user_123","resource":"posts","action":"create"}'
# Result: {"authorized":true}

curl -X POST http://localhost:3027/api/authorize \
  -d '{"userId":"user_123","resource":"posts","action":"delete"}'
# Result: {"authorized":false}
```

### Example 2: Role Hierarchy

```bash
# Create hierarchy: admin -> moderator -> user
curl -X POST http://localhost:3027/api/roles \
  -d '{"name":"user"}'

curl -X POST http://localhost:3027/api/roles \
  -d '{"name":"moderator","parentRole":"user"}'

curl -X POST http://localhost:3027/api/roles \
  -d '{"name":"admin","parentRole":"moderator"}'

# Assign permissions at each level
curl -X POST http://localhost:3027/api/roles/user/permissions \
  -d '{"resource":"posts","action":"read"}'

curl -X POST http://localhost:3027/api/roles/moderator/permissions \
  -d '{"resource":"posts","action":"update"}'

curl -X POST http://localhost:3027/api/roles/admin/permissions \
  -d '{"resource":"users","action":"manage"}'

# User with admin role has all permissions:
# - posts:read (from user)
# - posts:update (from moderator)
# - users:manage (from admin)
```

### Example 3: Time-Based Access Control

```bash
# Create policy for business hours access
curl -X POST http://localhost:3027/api/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "business_hours",
    "description": "Allow access only 9AM-5PM weekdays",
    "effect": "allow",
    "resource": "sensitive_data",
    "action": "*",
    "conditions": {
      "hourOfDay": {"gte": 9, "lte": 17},
      "dayOfWeek": {"in": [1,2,3,4,5]}
    },
    "priority": 100
  }'

# Test during business hours (2PM on Wednesday)
curl -X POST http://localhost:3027/api/authorize \
  -d '{
    "userId":"user_123",
    "resource":"sensitive_data",
    "action":"read",
    "context":{"hourOfDay":14,"dayOfWeek":3}
  }'
# Result: {"authorized":true}

# Test outside business hours (8PM on Wednesday)
curl -X POST http://localhost:3027/api/authorize \
  -d '{
    "userId":"user_123",
    "resource":"sensitive_data",
    "action":"read",
    "context":{"hourOfDay":20,"dayOfWeek":3}
  }'
# Result: {"authorized":false}
```

### Example 4: Multi-Tenant Access

```bash
# Create policy for tenant isolation
curl -X POST http://localhost:3027/api/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "tenant_isolation",
    "description": "Users can only access their own tenant data",
    "effect": "allow",
    "resource": "tenant_data",
    "conditions": {
      "userTenantId": {"eq": "$resourceTenantId"}
    }
  }'

# Check access (same tenant)
curl -X POST http://localhost:3027/api/authorize \
  -d '{
    "userId":"user_123",
    "resource":"tenant_data",
    "action":"read",
    "resourceId":"tenant_abc",
    "context":{
      "userTenantId":"tenant_abc",
      "resourceTenantId":"tenant_abc"
    }
  }'
# Result: {"authorized":true}

# Check access (different tenant)
curl -X POST http://localhost:3027/api/authorize \
  -d '{
    "userId":"user_123",
    "resource":"tenant_data",
    "action":"read",
    "resourceId":"tenant_xyz",
    "context":{
      "userTenantId":"tenant_abc",
      "resourceTenantId":"tenant_xyz"
    }
  }'
# Result: {"authorized":false}
```

### Example 5: Express.js Middleware

```javascript
import axios from 'axios';

async function authorize(userId, resource, action, context = {}) {
  const response = await axios.post('http://localhost:3027/api/authorize', {
    userId,
    resource,
    action,
    context
  });
  return response.data.authorized;
}

// Middleware
async function requirePermission(resource, action) {
  return async (req, res, next) => {
    const userId = req.user.id;

    const authorized = await authorize(userId, resource, action, {
      ipAddress: req.ip,
      userAgent: req.headers['user-agent'],
      hourOfDay: new Date().getHours(),
      dayOfWeek: new Date().getDay()
    });

    if (!authorized) {
      return res.status(403).json({
        error: 'Forbidden',
        message: `You don't have permission to ${action} ${resource}`
      });
    }

    next();
  };
}

// Usage
app.post('/api/posts',
  requirePermission('posts', 'create'),
  async (req, res) => {
    // Create post
  }
);

app.delete('/api/posts/:id',
  requirePermission('posts', 'delete'),
  async (req, res) => {
    // Delete post
  }
);
```

---

## Troubleshooting

### Authorization Always Denied

**Symptom:** All authorization checks return false.

**Solutions:**
```bash
# Check if user has roles
curl http://localhost:3027/api/users/user_123/roles

# Check role permissions
curl http://localhost:3027/api/roles/role_name/permissions

# Check policy configuration
curl http://localhost:3027/api/policies

# Verify default deny setting
echo $ACL_DEFAULT_DENY  # Should be 'true' or 'false'

# Check authorization with detailed response
curl -X POST http://localhost:3027/api/authorize \
  -d '{"userId":"user_123","resource":"posts","action":"create"}' | jq
```

### Role Hierarchy Not Working

**Symptom:** Child roles don't inherit parent permissions.

**Solutions:**
```bash
# Verify parent role relationship
curl http://localhost:3027/api/roles/child_role | jq '.parentRole'

# Check role hierarchy
curl http://localhost:3027/api/roles/hierarchy

# Verify max depth not exceeded
echo $ACL_MAX_ROLE_DEPTH  # Default: 10

# Check for circular references
# System should prevent these automatically
```

### Policy Not Matching

**Symptom:** ABAC policy not granting access as expected.

**Solutions:**
```bash
# Test policy evaluation
curl -X POST http://localhost:3027/api/policies/policy_name/evaluate \
  -d '{"context":{"hourOfDay":14,"dayOfWeek":3}}'

# Check policy conditions syntax
curl http://localhost:3027/api/policies/policy_name

# Verify policy is enabled
curl http://localhost:3027/api/policies/policy_name | jq '.enabled'

# Check policy priority
# Higher priority policies are evaluated first
curl http://localhost:3027/api/policies | jq '.policies | sort_by(.priority)'
```

### Cache Issues

**Symptom:** Authorization decisions not reflecting recent changes.

**Solutions:**
```bash
# Clear cache by restarting server
nself plugin access-controls server

# Reduce cache TTL for debugging
export ACL_CACHE_TTL_SECONDS=10

# Disable caching temporarily
export ACL_CACHE_TTL_SECONDS=0

# Wait for cache to expire (default 5 minutes)
```

### Permission Denied Unexpectedly

**Symptom:** User should have access but is denied.

**Solutions:**
```bash
# Check all user permissions (including inherited)
curl http://localhost:3027/api/users/user_123/permissions

# Verify exact resource and action names match
# "posts:create" ≠ "posts:Create" ≠ "post:create"

# Check for deny policies
curl http://localhost:3027/api/policies | jq '.policies[] | select(.effect=="deny")'

# Review authorization reason
curl -X POST http://localhost:3027/api/authorize \
  -d '{"userId":"user_123","resource":"posts","action":"create"}' | jq '.reason'
```

---

**Need Help?**

- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Documentation: https://github.com/acamarata/nself-plugins/wiki
- Plugin Source: https://github.com/acamarata/nself-plugins/tree/main/plugins/access-controls
