# Access Controls Plugin - Complete Example

This document demonstrates a complete workflow using the access-controls plugin.

## Scenario

We'll build an access control system for a blog platform with:
- Multiple user roles (admin, editor, author, reader)
- Fine-grained permissions
- Content ownership policies
- Scoped permissions (per-channel moderators)

## Setup

```bash
# 1. Install and build
cd plugins/access-controls/ts
npm install
npm run build

# 2. Configure database
export DATABASE_URL="postgresql://postgres:password@localhost:5432/nself"

# 3. Initialize schema
node dist/cli.js init
```

## Part 1: Create Role Hierarchy

```bash
# Create base roles
node dist/cli.js roles create reader --display-name "Reader" --description "Can view published content"

node dist/cli.js roles create author --display-name "Author" --description "Can create and edit own posts" --parent reader

node dist/cli.js roles create editor --display-name "Editor" --description "Can edit all posts" --parent author

node dist/cli.js roles create admin --display-name "Administrator" --description "Full system access" --parent editor

# View hierarchy
node dist/cli.js roles list
```

## Part 2: Define Permissions

```bash
# Reader permissions
node dist/cli.js permissions create --resource "posts" --action "view" --description "View published posts"
node dist/cli.js permissions create --resource "comments" --action "view" --description "View comments"
node dist/cli.js permissions create --resource "comments" --action "create" --description "Create comments"

# Author permissions
node dist/cli.js permissions create --resource "posts" --action "create" --description "Create new posts"
node dist/cli.js permissions create --resource "posts" --action "edit:own" --description "Edit own posts"
node dist/cli.js permissions create --resource "posts" --action "delete:own" --description "Delete own posts"

# Editor permissions
node dist/cli.js permissions create --resource "posts" --action "edit:any" --description "Edit any post"
node dist/cli.js permissions create --resource "posts" --action "publish" --description "Publish posts"

# Admin permissions
node dist/cli.js permissions create --resource "posts" --action "delete:any" --description "Delete any post"
node dist/cli.js permissions create --resource "users" --action "manage" --description "Manage users"
node dist/cli.js permissions create --resource "settings" --action "manage" --description "Manage settings"

# View all permissions
node dist/cli.js permissions list
```

## Part 3: Assign Permissions to Roles (via API)

Start the server:
```bash
node dist/cli.js server
```

In another terminal:

```bash
# Get role IDs first
curl http://localhost:3027/v1/roles | jq

# Assume we got these IDs:
READER_ID="uuid-reader"
AUTHOR_ID="uuid-author"
EDITOR_ID="uuid-editor"
ADMIN_ID="uuid-admin"

# Get permission IDs
curl http://localhost:3027/v1/permissions | jq

# Assign reader permissions to reader role
curl -X POST http://localhost:3027/v1/roles/$READER_ID/permissions \
  -H "Content-Type: application/json" \
  -d '{"permission_id": "uuid-posts-view"}'

curl -X POST http://localhost:3027/v1/roles/$READER_ID/permissions \
  -H "Content-Type: application/json" \
  -d '{"permission_id": "uuid-comments-view"}'

curl -X POST http://localhost:3027/v1/roles/$READER_ID/permissions \
  -H "Content-Type: application/json" \
  -d '{"permission_id": "uuid-comments-create"}'

# Assign author permissions to author role
curl -X POST http://localhost:3027/v1/roles/$AUTHOR_ID/permissions \
  -H "Content-Type: application/json" \
  -d '{"permission_id": "uuid-posts-create"}'

curl -X POST http://localhost:3027/v1/roles/$AUTHOR_ID/permissions \
  -H "Content-Type: application/json" \
  -d '{"permission_id": "uuid-posts-edit-own"}'

# ... (continue for editor and admin)
```

## Part 4: Assign Roles to Users

```bash
# Assign reader role to user123
node dist/cli.js users user123 assign --role reader

# Assign author role to user456
node dist/cli.js users user456 assign --role author

# Assign editor role to user789 (inherits author and reader permissions)
node dist/cli.js users user789 assign --role editor

# Assign admin role to admin001
node dist/cli.js users admin001 assign --role admin

# View user's roles and permissions
node dist/cli.js users user456 list
```

## Part 5: Create ABAC Policies

Policies override RBAC permissions. Let's add content ownership policies:

```bash
# Policy: Authors can only edit their own posts
curl -X POST http://localhost:3027/v1/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "author-own-posts-only",
    "description": "Authors can only edit posts they created",
    "effect": "allow",
    "principal_type": "role",
    "principal_value": "author",
    "resource_pattern": "posts:*",
    "action_pattern": "edit",
    "conditions": {
      "post_author_id": {"$eq": "@user_id"}
    },
    "priority": 10
  }'

# Policy: Deny deleting posts with > 100 comments
curl -X POST http://localhost:3027/v1/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "protect-popular-posts",
    "description": "Cannot delete posts with many comments",
    "effect": "deny",
    "principal_type": "role",
    "principal_value": "*",
    "resource_pattern": "posts:*",
    "action_pattern": "delete",
    "conditions": {
      "comment_count": {"$gt": 100}
    },
    "priority": 100
  }'

# Policy: Time-based access (no posting after midnight)
curl -X POST http://localhost:3027/v1/policies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "no-posting-after-midnight",
    "description": "Cannot create posts after midnight",
    "effect": "deny",
    "principal_type": "role",
    "principal_value": "author",
    "resource_pattern": "posts",
    "action_pattern": "create",
    "conditions": {
      "hour": {"$gte": 0, "$lt": 6}
    },
    "priority": 50
  }'

# View policies
node dist/cli.js policies list
```

## Part 6: Authorization Checks

```bash
# Check if user456 (author) can view posts
node dist/cli.js authorize user456 posts view
# Expected: YES (inherited from reader role)

# Check if user456 can create posts
node dist/cli.js authorize user456 posts create
# Expected: YES (author permission)

# Check if user456 can edit their own post
node dist/cli.js authorize user456 posts:123 edit --context '{"post_author_id":"user456"}'
# Expected: YES (ownership policy matches)

# Check if user456 can edit someone else's post
node dist/cli.js authorize user456 posts:123 edit --context '{"post_author_id":"user789"}'
# Expected: NO (ownership policy doesn't match)

# Check if admin can delete a popular post
node dist/cli.js authorize admin001 posts:456 delete --context '{"comment_count":150}'
# Expected: NO (deny policy overrides even admin permissions)

# Check if author can post at 2 AM
node dist/cli.js authorize user456 posts create --context '{"hour":2}'
# Expected: NO (time-based policy denies)
```

## Part 7: Batch Authorization

```bash
curl -X POST http://localhost:3027/v1/authorize/batch \
  -H "Content-Type: application/json" \
  -d '{
    "requests": [
      {"user_id": "user456", "resource": "posts", "action": "view"},
      {"user_id": "user456", "resource": "posts", "action": "create"},
      {"user_id": "user456", "resource": "posts", "action": "delete:any"},
      {"user_id": "admin001", "resource": "users", "action": "manage"}
    ]
  }'

# Response shows authorization result for each request
```

## Part 8: Scoped Roles

Assign channel-specific moderator role:

```bash
# Create moderator role
node dist/cli.js roles create moderator --display-name "Moderator" --description "Channel moderator"

# Assign permissions to moderator
curl -X POST http://localhost:3027/v1/roles/$MODERATOR_ID/permissions \
  -H "Content-Type: application/json" \
  -d '{"permission_id": "uuid-posts-edit-any"}'

curl -X POST http://localhost:3027/v1/roles/$MODERATOR_ID/permissions \
  -H "Content-Type: application/json" \
  -d '{"permission_id": "uuid-comments-delete"}'

# Assign scoped role (user789 is moderator ONLY in channel "gaming")
curl -X POST http://localhost:3027/v1/users/user789/roles \
  -H "Content-Type: application/json" \
  -d '{
    "role_id": "'$MODERATOR_ID'",
    "scope": "channel",
    "scope_id": "gaming"
  }'

# Now user789 has moderator permissions only in gaming channel
# Application code must pass scope context when checking authorization
node dist/cli.js authorize user789 posts:in-gaming edit --context '{"channel":"gaming"}'
# Would need custom policy to enforce scope
```

## Part 9: Cache Management

```bash
# Get cache stats
curl http://localhost:3027/v1/cache/stats

# Invalidate specific user's cache after role change
curl -X POST http://localhost:3027/v1/cache/invalidate \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user456"}'

# Clear all cache
curl -X POST http://localhost:3027/v1/cache/invalidate \
  -H "Content-Type: application/json" \
  -d '{}'
```

## Part 10: Real-world Integration

In your application code:

```javascript
// Express.js middleware example
const axios = require('axios');

async function authorize(userId, resource, action, context = {}) {
  const response = await axios.post('http://localhost:3027/v1/authorize', {
    user_id: userId,
    resource,
    action,
    context
  });

  return response.data.allowed;
}

// Middleware
app.use(async (req, res, next) => {
  const userId = req.user.id;
  const resource = req.path.split('/')[1]; // e.g., "posts"
  const action = req.method === 'GET' ? 'view' : req.method.toLowerCase();

  const allowed = await authorize(userId, resource, action, {
    post_author_id: req.body?.author_id,
    hour: new Date().getHours(),
    // ... other context
  });

  if (!allowed) {
    return res.status(403).json({ error: 'Forbidden' });
  }

  next();
});

// Or check in route handler
app.delete('/posts/:id', async (req, res) => {
  const post = await getPost(req.params.id);

  const allowed = await authorize(req.user.id, `posts:${post.id}`, 'delete', {
    post_author_id: post.author_id,
    comment_count: post.comment_count,
  });

  if (!allowed) {
    return res.status(403).json({ error: 'Cannot delete this post' });
  }

  await deletePost(post.id);
  res.json({ success: true });
});
```

## Key Features Demonstrated

1. **Role Hierarchy**: Roles inherit permissions from parents
2. **RBAC**: Traditional permission-based access control
3. **ABAC**: Dynamic policies with conditions
4. **Pattern Matching**: Wildcards in resources/actions
5. **Context-based Decisions**: Ownership, time, counts, etc.
6. **Priority System**: Deny policies can override allow
7. **Scoped Roles**: Role applies only in specific scope
8. **Caching**: Fast repeated checks with cache invalidation
9. **Batch Operations**: Check multiple permissions at once
10. **Multi-app Support**: Isolated ACL per source_account_id

## Performance Tips

1. **Cache TTL**: Set appropriate `ACL_CACHE_TTL_SECONDS` (default 300)
2. **Batch Checks**: Use batch authorization for multiple checks
3. **Policy Priority**: Higher priority policies evaluated first
4. **Index Usage**: All queries use indexed columns
5. **Connection Pooling**: Database connections are pooled
6. **Minimal Context**: Pass only necessary context data

## Security Best Practices

1. **Default Deny**: Keep `ACL_DEFAULT_DENY=true` (default)
2. **API Key**: Set `ACL_API_KEY` in production
3. **Rate Limiting**: Configure `ACL_RATE_LIMIT_MAX`
4. **Audit Logs**: Monitor webhook events table
5. **HTTPS**: Use TLS in production
6. **Principle of Least Privilege**: Grant minimal necessary permissions
7. **Regular Reviews**: Audit roles and policies periodically
8. **Deny Policies**: Use deny for critical restrictions
