# Access Controls Plugin - Implementation Summary

## Plugin Details

- **Name**: access-controls
- **Version**: 1.0.0
- **Port**: 3027
- **Category**: security
- **Type**: RBAC + ABAC authorization system

## Architecture

### Core Components

1. **Database Layer** (`database.ts`)
   - Complete CRUD operations for all ACL entities
   - Multi-app support via `source_account_id`
   - Efficient queries with proper indexing
   - Recursive role hierarchy queries

2. **Authorization Engine** (`authz.ts`)
   - RBAC permission checking with role hierarchy
   - ABAC policy evaluation with pattern matching
   - In-memory caching with TTL
   - Condition evaluation with operators ($eq, $ne, $in, $gt, etc.)

3. **HTTP Server** (`server.ts`)
   - RESTful API for all operations
   - Multi-app context resolution
   - Rate limiting and API key authentication
   - Health, readiness, and liveness endpoints

4. **CLI** (`cli.ts`)
   - Complete command-line interface
   - Role, permission, user, and policy management
   - Authorization testing
   - Status and statistics

## Database Schema

### Tables (6)

1. **acl_roles**
   - Hierarchical roles with parent relationships
   - Level tracking for efficient queries
   - System role protection
   - Indexed on: source_account_id, name, parent_role_id, level

2. **acl_permissions**
   - Resource and action definitions
   - Optional conditions (JSONB)
   - Unique constraint on (source_account_id, resource, action)
   - Indexed on: source_account_id, resource, action

3. **acl_role_permissions**
   - Many-to-many mapping
   - Grant/deny flag per permission
   - Per-mapping conditions
   - Cascading deletes
   - Indexed on: role_id, permission_id, granted

4. **acl_user_roles**
   - User-role assignments
   - Optional expiration (expires_at)
   - Scoped roles (scope, scope_id)
   - Granted_by audit trail
   - Unique per (source_account_id, user_id, role_id, scope, scope_id)
   - Indexed on: source_account_id, user_id, role_id, expires_at, scope

5. **acl_policies**
   - ABAC policy definitions
   - Effect: allow or deny
   - Principal types: role, user, group
   - Pattern matching for resources/actions
   - Conditions (JSONB)
   - Priority ordering
   - Enabled/disabled flag
   - Indexed on: source_account_id, principal, resource_pattern, priority

6. **acl_webhook_events**
   - Event log for audit trail
   - Processed flag for idempotency
   - Error tracking

### Key Features

- **UUID Primary Keys**: Generated via gen_random_uuid()
- **Timestamps**: All tables have created_at, some have updated_at
- **JSONB**: For flexible metadata and conditions
- **Constraints**: Unique constraints and foreign keys
- **Cascading**: ON DELETE CASCADE for role_permissions and user_roles

## API Endpoints (36)

### Health & Status
- GET /health - Basic health check
- GET /ready - Database connectivity check
- GET /live - Liveness with stats
- GET /status - Full status with config and cache stats

### Roles (6 endpoints)
- POST /v1/roles - Create role
- GET /v1/roles - List roles (paginated)
- GET /v1/roles/hierarchy - Get role hierarchy tree
- GET /v1/roles/:id - Get role with permissions
- PUT /v1/roles/:id - Update role
- DELETE /v1/roles/:id - Delete role

### Permissions (4 endpoints)
- POST /v1/permissions - Create permission
- GET /v1/permissions - List permissions (paginated)
- GET /v1/permissions/:id - Get permission
- DELETE /v1/permissions/:id - Delete permission

### Role Permissions (2 endpoints)
- POST /v1/roles/:id/permissions - Assign permission to role
- DELETE /v1/roles/:roleId/permissions/:permId - Remove permission

### User Roles (4 endpoints)
- POST /v1/users/:userId/roles - Assign role to user
- GET /v1/users/:userId/roles - List user's roles
- DELETE /v1/users/:userId/roles/:roleId - Remove role from user
- GET /v1/users/:userId/permissions - Get effective permissions

### Authorization (2 endpoints)
- POST /v1/authorize - Single authorization check
- POST /v1/authorize/batch - Batch authorization checks

### Policies (5 endpoints)
- POST /v1/policies - Create ABAC policy
- GET /v1/policies - List policies (paginated)
- GET /v1/policies/:id - Get policy
- PUT /v1/policies/:id - Update policy
- DELETE /v1/policies/:id - Delete policy

### Cache Management (2 endpoints)
- POST /v1/cache/invalidate - Invalidate user or all cache
- GET /v1/cache/stats - Get cache statistics

## CLI Commands (8)

1. **init** - Initialize database schema
2. **server** - Start HTTP server
3. **status** - Show statistics
4. **roles** - Manage roles (list, create, show, delete)
5. **permissions** - Manage permissions (list, create, delete)
6. **users** - Manage user roles (list, assign, remove)
7. **authorize** - Test authorization
8. **policies** - Manage ABAC policies (list, create, delete)

## Authorization Flow

### 1. RBAC Check (Fast Path)
```
1. Get user's effective permissions (cached)
2. Check role hierarchy for matching permission
3. Match resource and action patterns (wildcards supported)
4. Evaluate permission conditions against context
5. If match found → ALLOW
```

### 2. ABAC Check (Policy Evaluation)
```
1. Get applicable policies for user and roles
2. Sort by priority (DESC)
3. For each policy:
   - Match resource pattern
   - Match action pattern
   - Evaluate conditions
   - If match → return policy effect (allow/deny)
4. No match → apply default (deny/allow)
```

### 3. Caching
```
- User permissions cached in memory
- TTL configurable (default 300s)
- Invalidated on role/permission changes
- Cache key: source_account_id:user_id
```

## Pattern Matching

### Wildcards
- `*` - Matches anything
- `posts:*` - Matches all post resources
- `posts:123` - Exact match

### Examples
- Pattern: `posts:*` matches `posts:123`, `posts:456`, etc.
- Pattern: `*` matches anything
- Pattern: `posts` matches only `posts`

## Condition Operators

### Comparison
- `$eq` - Equals
- `$ne` - Not equals
- `$gt` - Greater than
- `$gte` - Greater than or equal
- `$lt` - Less than
- `$lte` - Less than or equal

### Arrays
- `$in` - Value in array
- `$nin` - Value not in array

### Example Conditions
```json
{
  "post_author_id": {"$eq": "@user_id"},
  "comment_count": {"$gt": 100},
  "hour": {"$gte": 9, "$lte": 17},
  "role": {"$in": ["admin", "moderator"]}
}
```

## Configuration

### Environment Variables

**Required:**
- DATABASE_URL or POSTGRES_* variables

**Optional:**
- ACL_PLUGIN_PORT (default: 3027)
- ACL_PLUGIN_HOST (default: 0.0.0.0)
- ACL_CACHE_TTL_SECONDS (default: 300)
- ACL_MAX_ROLE_DEPTH (default: 10)
- ACL_DEFAULT_DENY (default: true)
- ACL_API_KEY (optional)
- ACL_RATE_LIMIT_MAX (default: 200)
- ACL_RATE_LIMIT_WINDOW_MS (default: 60000)
- LOG_LEVEL (default: info)

## Security Features

1. **API Key Authentication**: Optional but recommended
2. **Rate Limiting**: Configurable per-IP rate limiting
3. **Default Deny**: Fail-secure by default
4. **CORS**: Configurable cross-origin support
5. **Input Validation**: Type-safe TypeScript interfaces
6. **SQL Injection**: Parameterized queries
7. **Multi-tenancy**: Isolated data per source_account_id

## Performance Optimizations

1. **Caching**: In-memory permission cache
2. **Indexes**: All common queries indexed
3. **Connection Pooling**: Database connection reuse
4. **Batch Operations**: Batch authorization checks
5. **Recursive CTEs**: Efficient role hierarchy queries
6. **JSONB**: Fast JSON operations in PostgreSQL

## Multi-App Support

- All tables have `source_account_id` column (default: 'primary')
- Scoped database instances per request
- Context resolution from headers/query params
- Isolated ACL per application/tenant

## Testing

### Manual Testing
```bash
# Start server
npm run dev

# In another terminal
# Test role creation
curl -X POST http://localhost:3027/v1/roles \
  -H "Content-Type: application/json" \
  -d '{"name":"test-role"}'

# Test authorization
curl -X POST http://localhost:3027/v1/authorize \
  -H "Content-Type: application/json" \
  -d '{"user_id":"user1","resource":"posts","action":"view"}'
```

### Integration with Application
```javascript
// Authorization check in your app
const checkAccess = async (userId, resource, action, context = {}) => {
  const response = await fetch('http://localhost:3027/v1/authorize', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId, resource, action, context })
  });
  const result = await response.json();
  return result.allowed;
};
```

## Production Checklist

- [ ] Set DATABASE_URL
- [ ] Set ACL_API_KEY
- [ ] Configure rate limits
- [ ] Enable HTTPS
- [ ] Set appropriate cache TTL
- [ ] Monitor cache hit rates
- [ ] Set up database backups
- [ ] Configure logging level
- [ ] Test role hierarchy depth limits
- [ ] Document your permission model
- [ ] Set up monitoring/alerting
- [ ] Audit policies and roles regularly

## Dependencies

- **@nself/plugin-utils**: Shared utilities (logger, database, etc.)
- **fastify**: HTTP server framework
- **@fastify/cors**: CORS support
- **commander**: CLI framework
- **dotenv**: Environment variable loading
- **TypeScript**: Type safety

## File Structure

```
plugins/access-controls/
├── plugin.json               # Plugin manifest
├── README.md                 # User documentation
├── EXAMPLE.md                # Complete usage examples
├── SUMMARY.md                # This file
└── ts/
    ├── package.json          # npm configuration
    ├── tsconfig.json         # TypeScript configuration
    ├── .env.example          # Environment template
    └── src/
        ├── types.ts          # All TypeScript interfaces
        ├── config.ts         # Configuration loading
        ├── database.ts       # Database operations (840 lines)
        ├── authz.ts          # Authorization engine (340 lines)
        ├── server.ts         # HTTP server (540 lines)
        ├── cli.ts            # CLI commands (550 lines)
        └── index.ts          # Module exports
```

## Line Count Summary

- **types.ts**: ~260 lines
- **config.ts**: ~70 lines
- **database.ts**: ~840 lines
- **authz.ts**: ~340 lines
- **server.ts**: ~540 lines
- **cli.ts**: ~550 lines
- **Total**: ~2,600 lines of production code

## Key Implementation Decisions

1. **UUID PKs**: Better for distributed systems
2. **JSONB Conditions**: Flexible without schema changes
3. **In-Memory Cache**: Fast, simple, good for single instance
4. **Deny Overrides Allow**: Security-first approach
5. **Priority System**: Predictable policy evaluation
6. **Pattern Matching**: Flexibility without explosion of permissions
7. **Scoped Roles**: Channel/org-specific permissions
8. **Multi-App via Column**: Simple, efficient isolation
9. **TypeScript**: Type safety and better DX
10. **Fastify**: Fast, low-overhead HTTP server

## Future Enhancements (Not Implemented)

1. **Distributed Cache**: Redis for multi-instance deployment
2. **Audit Logging**: Detailed access logs
3. **Policy Versioning**: Track policy changes over time
4. **Role Templates**: Pre-configured role sets
5. **Permission Discovery**: Auto-discover available permissions
6. **Policy Testing**: Dry-run policy evaluation
7. **Bulk Import/Export**: JSON-based role/permission management
8. **WebSocket Support**: Real-time permission updates
9. **GraphQL API**: Alternative to REST
10. **Advanced Conditions**: Complex boolean logic

## Comparison to Alternatives

### vs. Casbin
- **Simpler**: No learning curve for policy language
- **SQL-based**: Standard PostgreSQL storage
- **REST API**: Built-in HTTP interface
- **Caching**: Integrated caching layer

### vs. Open Policy Agent (OPA)
- **Database-backed**: Persistent storage
- **Role Hierarchy**: Built-in role inheritance
- **No Rego**: Standard JSON/SQL patterns
- **User-friendly CLI**: Easy management

### vs. AWS IAM
- **Self-hosted**: Full control
- **Simpler Model**: Easy to understand
- **No Vendor Lock-in**: Open source
- **Cost**: Free to run

## License

Source-Available (MIT)
