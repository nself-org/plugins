# Plugin Development Guide

This guide covers how to create custom nself plugins.

## 🚨 READ FIRST: Mandatory Standards

**BEFORE developing a plugin, you MUST read and follow [Standards](Standards.md).**

Key requirements:

- ✅ ALL tables must use `np_` prefix
- ✅ ALL plugins must use `source_account_id` for multi-app isolation
- ✅ Use one of 13 official categories only
- ✅ Plugin names must be lowercase-with-hyphens

**Failure to comply will result in automated build failures.**

---

## Plugin Structure

Every plugin follows this structure:

```
plugins/<name>/
├── plugin.json           # Plugin metadata (required)
├── cmd/                  # Go service or CLI entry point (required)
├── migrations/           # Embedded SQL migrations, applied at boot (required for plugins with tables)
├── schema/
│   └── tables.sql        # Database schema reference
├── Dockerfile
└── docker-compose.plugin.yml
```

A plugin ships a binary. The CLI never runs per-plugin `install.sh`, `uninstall.sh`, `actions/*.sh` or
`webhooks/**/*.sh` scripts, and the repository carries none. Install, upgrade and removal go through
`nself plugin install|upgrade|remove`; migrations run when the service starts. Start from
`nself plugin new` and follow [Plugin Development](Plugin-Development.md).

## Creating a Plugin

### 1. Create Plugin Directory

```bash
mkdir -p plugins/my-service/{cmd,migrations,schema}
```

### 2. Create plugin.json

The manifest file describes your plugin:

```json
{
  "name": "my-service",
  "version": "1.0.0",
  "description": "Integration with My Service",
  "author": "Your Name",
  "license": "MIT",
  "minNselfVersion": "0.4.8",
  "category": "integrations",
  "tags": ["my-service", "integration"],

  "tables": [
    "np_myservice_items",
    "np_myservice_webhooks"
  ],

  "webhooks": {
    "item.created": "Sync new items",
    "item.updated": "Update item data"
  },

  "actions": {
    "sync": "Sync all data",
    "items": "Manage items"
  },

  "envVars": {
    "required": ["MYSERVICE_API_KEY"],
    "optional": ["MYSERVICE_WEBHOOK_SECRET"]
  },

  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "pkStrategy": "uuid",
    "defaultValue": "primary"
  },

  "config": {
    "port": 4000,
    "webhookPath": "/webhooks/my-service"
  }
}
```

**Key Points**:

- ⚠️ Tables MUST start with `np_` prefix
- ⚠️ Category must be one of 13 official categories
- ⚠️ `multiApp.isolationColumn` MUST be `source_account_id`

### 3. Create Database Schema

Create `schema/tables.sql`:

```sql
-- ⚠️ IMPORTANT: ALL tables MUST use np_ prefix and include source_account_id

CREATE TABLE IF NOT EXISTS np_myservice_items (
    id UUID PRIMARY KEY,
    source_account_id VARCHAR(255) NOT NULL,  -- REQUIRED for multi-app
    name VARCHAR(255) NOT NULL,
    data JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS np_myservice_webhooks (
    id UUID PRIMARY KEY,
    source_account_id VARCHAR(255) NOT NULL,  -- REQUIRED for multi-app
    type VARCHAR(100) NOT NULL,
    data JSONB NOT NULL,
    processed BOOLEAN DEFAULT FALSE,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for multi-tenant queries
CREATE INDEX IF NOT EXISTS idx_np_myservice_items_account
    ON np_myservice_items(source_account_id);

CREATE INDEX IF NOT EXISTS idx_np_myservice_items_name
    ON np_myservice_items(name);

CREATE INDEX IF NOT EXISTS idx_np_myservice_webhooks_account
    ON np_myservice_webhooks(source_account_id);
```

**Schema Requirements**:

- ⚠️ Table name MUST start with `np_`
- ⚠️ MUST include `source_account_id VARCHAR(255) NOT NULL`
- ⚠️ MUST add index on `source_account_id`
- Use UUID for primary keys (recommended)
- Include `created_at` and `synced_at` timestamps

## Testing Your Plugin

### 1. Install Locally

```bash
nself plugin install ./plugins/my-service
```

### 2. Test Actions

```bash
nself plugin my-service sync
nself plugin my-service items list
```

### 3. Test Webhooks

```bash
# Send test webhook
curl -X POST http://localhost/webhooks/my-service \
    -H "Content-Type: application/json" \
    -d '{"type":"item.created","item_id":"123","name":"Test"}'
```

## Publishing

1. Update version in `plugin.json`
2. Create git tag: `git tag my-service-v1.0.0`
3. Submit PR to nself-plugins repository

## Best Practices

1. **Idempotent syncs** - Running sync twice should produce same result
2. **Upsert data** - Use ON CONFLICT for database inserts
3. **Handle errors gracefully** - Log errors, don't crash
4. **Validate input** - Check required fields exist
5. **Use transactions** - For multi-table operations
6. **Rate limit awareness** - Respect API rate limits
7. **Incremental sync** - Support syncing only changes

## Examples

See the [Stripe plugin](../plugins/stripe/) for a complete implementation example.

## CI

Which checks run on a pull request is described in [CI scope](CI-Scope.md).
