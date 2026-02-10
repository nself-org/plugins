# Plugin Development Guide

This guide covers how to create custom nself plugins.

## Plugin Structure

Every plugin follows this structure:

```
plugins/<name>/
├── plugin.json           # Plugin metadata (required)
├── install.sh            # Installation script (required)
├── uninstall.sh          # Uninstallation script (required)
├── schema/
│   ├── tables.sql        # Database schema (required)
│   └── migrations/       # Schema migrations (optional)
├── webhooks/
│   ├── handler.sh        # Webhook dispatcher (optional)
│   └── events/           # Event handlers (optional)
├── actions/
│   └── *.sh              # CLI actions (optional)
└── templates/
    └── *.template        # Config templates (optional)
```

## Creating a Plugin

### 1. Create Plugin Directory

```bash
mkdir -p plugins/my-service/{schema,webhooks/events,actions,templates}
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
  "category": "productivity",
  "tags": ["my-service", "integration"],

  "tables": [
    "myservice_items",
    "myservice_webhooks"
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
  }
}
```

### 3. Create Database Schema

Create `schema/tables.sql`:

```sql
CREATE TABLE IF NOT EXISTS myservice_items (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    data JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE,
    synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS myservice_webhooks (
    id VARCHAR(255) PRIMARY KEY,
    type VARCHAR(100) NOT NULL,
    data JSONB NOT NULL,
    processed BOOLEAN DEFAULT FALSE,
    received_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_myservice_items_name ON myservice_items(name);
```

### 4. Create Install Script

Create `install.sh`:

```bash
#!/bin/bash
set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"
source "${SHARED_DIR}/schema-sync.sh"

install_plugin() {
    plugin_info "Installing my-service plugin..."

    # Check environment
    if ! plugin_check_env "myservice" "MYSERVICE_API_KEY"; then
        plugin_warn "MYSERVICE_API_KEY not set"
    fi

    # Apply schema
    plugin_db_exec_file "${PLUGIN_DIR}/schema/tables.sql"

    plugin_success "Plugin installed!"
}

install_plugin
```

### 5. Create Uninstall Script

Create `uninstall.sh`:

```bash
#!/bin/bash
set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

uninstall_plugin() {
    local keep_data="${1:-false}"

    plugin_info "Uninstalling my-service plugin..."

    if [[ "$keep_data" != "true" ]]; then
        plugin_db_query "DROP TABLE IF EXISTS myservice_webhooks CASCADE;"
        plugin_db_query "DROP TABLE IF EXISTS myservice_items CASCADE;"
    fi

    plugin_success "Plugin uninstalled!"
}

uninstall_plugin "${1:-false}"
```

## Adding Actions

Actions are CLI commands users can run.

### Create an Action

Create `actions/sync.sh`:

```bash
#!/bin/bash
set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

MYSERVICE_API_KEY="${MYSERVICE_API_KEY:-}"
MYSERVICE_API_BASE="https://api.myservice.com/v1"

api_request() {
    local endpoint="$1"
    curl -s \
        -H "Authorization: Bearer ${MYSERVICE_API_KEY}" \
        "${MYSERVICE_API_BASE}/${endpoint}"
}

sync_items() {
    plugin_info "Syncing items..."

    local response
    response=$(api_request "items")

    # Process and insert items
    # ...

    plugin_success "Sync complete!"
}

case "${1:-sync}" in
    sync) sync_items ;;
    -h|--help) echo "Usage: nself plugin my-service sync" ;;
    *) echo "Unknown command: $1" ;;
esac
```

## Adding Webhook Support

### 1. Create Webhook Handler

Create `webhooks/handler.sh`:

```bash
#!/bin/bash
set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

process_webhook() {
    local payload="$1"

    # Extract event type
    local event_type
    event_type=$(plugin_json_get "$payload" "type")

    plugin_info "Received: $event_type"

    # Record event
    local event_id
    event_id=$(plugin_json_get "$payload" "id")

    plugin_db_query "
        INSERT INTO myservice_webhooks (id, type, data)
        VALUES ('$event_id', '$event_type', '$payload'::jsonb);
    "

    # Dispatch to event handler
    local handler="${PLUGIN_DIR}/webhooks/events/${event_type}.sh"
    if [[ -f "$handler" ]]; then
        bash "$handler" "$payload"
    fi
}

process_webhook "$1"
```

### 2. Create Event Handlers

Create `webhooks/events/item_created.sh`:

```bash
#!/bin/bash
set -euo pipefail

PLUGIN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SHARED_DIR="$(dirname "$PLUGIN_DIR")/../shared"

source "${SHARED_DIR}/plugin-utils.sh"

handle_item_created() {
    local payload="$1"

    local item_id name
    item_id=$(plugin_json_get "$payload" "item_id")
    name=$(plugin_json_get "$payload" "name")

    plugin_db_query "
        INSERT INTO myservice_items (id, name, synced_at)
        VALUES ('$item_id', '$name', NOW())
        ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, synced_at = NOW();
    "

    plugin_success "Created item: $item_id"
}

handle_item_created "$@"
```

## Shared Utilities

Use the shared utilities for common operations:

### Database Operations

```bash
# Execute query
plugin_db_query "SELECT * FROM table;"

# Execute SQL file
plugin_db_exec_file "/path/to/file.sql"

# Check if table exists
if plugin_table_exists "myservice_items"; then
    echo "Table exists"
fi
```

### HTTP Requests

```bash
plugin_http_get "https://api.example.com/endpoint"
plugin_http_post "https://api.example.com/endpoint" '{"data":"value"}'
```

### Logging

```bash
plugin_debug "Debug message"
plugin_info "Info message"
plugin_warn "Warning message"
plugin_error "Error message"
plugin_success "Success message"
```

### Caching

```bash
# Get cached value (TTL in seconds)
value=$(plugin_cache_get "my-plugin" "key" 3600)

# Set cached value
plugin_cache_set "my-plugin" "key" "value"

# Clear cache
plugin_cache_clear "my-plugin"
```

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
