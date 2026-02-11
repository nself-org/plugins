# Access Controls Plugin

Production-ready RBAC (Role-Based Access Control) + ABAC (Attribute-Based Access Control) authorization system for nself.

## Features

- **Role Hierarchy**: Parent-child role relationships with permission inheritance
- **RBAC**: Traditional role-based access control with fine-grained permissions
- **ABAC**: Policy-based access control with conditions and patterns
- **Scoped Roles**: Assign roles with specific scopes (e.g., channel moderator)
- **Pattern Matching**: Wildcard support for resources and actions (`posts:*`, `*`)
- **Caching**: In-memory permission caching with configurable TTL
- **Multi-App Support**: Isolated ACL per `source_account_id`
- **REST API**: Complete HTTP API for all operations
- **CLI**: Full command-line interface

## Installation

```bash
cd plugins/access-controls/ts
npm install
npm run build
```

## Configuration

Copy `.env.example` to `.env` and configure:

```bash
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
ACL_PLUGIN_PORT=3027
ACL_CACHE_TTL_SECONDS=300
ACL_MAX_ROLE_DEPTH=10
ACL_DEFAULT_DENY=true
```

## Quick Start

### 1. Initialize Database

```bash
npm run build
node dist/cli.js init
```

### 2. Start Server

```bash
node dist/cli.js server
# Server running on http://0.0.0.0:3027
```

### 3. Create Role and Permission

```bash
# Create a role
node dist/cli.js roles create admin --display-name "Administrator" --description "Full system access"

# Create permission
node dist/cli.js permissions create --resource "posts" --action "delete" --description "Delete posts"

# Assign permission to role (via API)
curl -X POST http://localhost:3027/v1/roles/{role_id}/permissions \
  -H "Content-Type: application/json" \
  -d '{"permission_id": "{permission_id}"}'
```

### 4. Assign Role to User

```bash
node dist/cli.js users user123 assign --role admin
```

### 5. Check Authorization

```bash
node dist/cli.js authorize user123 posts delete
# Authorization Result: YES/NO
```

## API Endpoints

### Roles

- `POST /v1/roles` - Create role
- `GET /v1/roles` - List roles
- `GET /v1/roles/hierarchy` - Get role hierarchy
- `GET /v1/roles/:id` - Get role with permissions
- `PUT /v1/roles/:id` - Update role
- `DELETE /v1/roles/:id` - Delete role

### Permissions

- `POST /v1/permissions` - Create permission
- `GET /v1/permissions` - List permissions
- `GET /v1/permissions/:id` - Get permission
- `DELETE /v1/permissions/:id` - Delete permission

### Role Permissions

- `POST /v1/roles/:id/permissions` - Assign permission to role
- `DELETE /v1/roles/:roleId/permissions/:permId` - Remove permission

### User Roles

- `POST /v1/users/:userId/roles` - Assign role to user
- `GET /v1/users/:userId/roles` - List user's roles
- `DELETE /v1/users/:userId/roles/:roleId` - Remove role from user
- `GET /v1/users/:userId/permissions` - Get effective permissions

### Authorization

- `POST /v1/authorize` - Check authorization
- `POST /v1/authorize/batch` - Batch authorization check

### Policies (ABAC)

- `POST /v1/policies` - Create policy
- `GET /v1/policies` - List policies
- `GET /v1/policies/:id` - Get policy
- `PUT /v1/policies/:id` - Update policy
- `DELETE /v1/policies/:id` - Delete policy

### Health & Status

- `GET /health` - Basic health check
- `GET /ready` - Readiness check (DB connectivity)
- `GET /live` - Liveness check with stats
- `GET /status` - Full status with configuration

## Authorization Example

```bash
# Check if user can delete posts
curl -X POST http://localhost:3027/v1/authorize \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user123",
    "resource": "posts",
    "action": "delete",
    "context": {"post_owner": "user123"}
  }'

# Response:
{
  "allowed": true,
  "reason": "RBAC permission granted",
  "matched_permissions": ["perm-uuid"],
  "cached": true
}
```

## Policy Example

Create a policy that allows users to delete only their own posts:

```bash
curl -X POST http://localhost:3027/v1/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "own-posts-only",
    "effect": "allow",
    "principal_type": "user",
    "principal_value": "*",
    "resource_pattern": "posts:*",
    "action_pattern": "delete",
    "conditions": {
      "post_owner": {"$eq": "@user_id"}
    },
    "priority": 10
  }'
```

## CLI Commands

```bash
# Initialize
nself-acl init

# Server
nself-acl server --port 3027

# Status
nself-acl status

# Roles
nself-acl roles list
nself-acl roles create <name> [options]
nself-acl roles show <name>
nself-acl roles delete <name>

# Permissions
nself-acl permissions list
nself-acl permissions create --resource <resource> --action <action>
nself-acl permissions delete --resource <resource> --action <action>

# Users
nself-acl users <user_id> list
nself-acl users <user_id> assign --role <role>
nself-acl users <user_id> remove --role <role>

# Authorize
nself-acl authorize <user_id> <resource> <action> [--context <json>]

# Policies
nself-acl policies list
nself-acl policies create --name <name> --effect <allow|deny> --type <role|user|group> --value <value> --resource <pattern> --action <pattern>
nself-acl policies delete --name <name>
```

## Database Schema

### Tables

1. **acl_roles** - Roles with hierarchy
2. **acl_permissions** - Permission definitions
3. **acl_role_permissions** - Role-permission mappings
4. **acl_user_roles** - User-role assignments
5. **acl_policies** - ABAC policies
6. **acl_webhook_events** - Event log

### Indexes

All tables have appropriate indexes on:
- `source_account_id`
- Foreign keys
- Common query columns (user_id, role_id, resource, action, etc.)

## Pattern Matching

Supports wildcards in resources and actions:

- `posts:*` - Matches all post resources
- `*` - Matches any resource/action
- `posts:123` - Exact match

## Conditions

Policies support condition operators:

- `$eq` - Equals
- `$ne` - Not equals
- `$in` - In array
- `$nin` - Not in array
- `$gt` - Greater than
- `$gte` - Greater than or equal
- `$lt` - Less than
- `$lte` - Less than or equal

## Cache Management

```bash
# Invalidate specific user
curl -X POST http://localhost:3027/v1/cache/invalidate \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user123"}'

# Clear all cache
curl -X POST http://localhost:3027/v1/cache/invalidate \
  -H "Content-Type: application/json" \
  -d '{}'

# Get cache stats
curl http://localhost:3027/v1/cache/stats
```

## License

MIT
