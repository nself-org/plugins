# Data Export Plugin

GDPR-compliant data export, deletion, and import plugin for nself.

## Features

- **User Data Export**: Export user data from all registered plugins
- **Plugin Data Export**: Export specific plugin data
- **Full Backup**: Export all data across all plugins
- **Data Deletion**: GDPR "Right to Erasure" with verification and cooldown
- **Data Import**: Import data from previous exports
- **Plugin Registry**: Register plugins for export/deletion operations
- **Multiple Formats**: JSON, CSV, and ZIP export formats
- **Download Tokens**: Secure, expiring download links
- **Multi-Account Support**: Isolated data per source_account_id

## Installation

```bash
cd plugins/data-export/ts
npm install
npm run build
```

## Configuration

Copy `.env.example` to `.env` and configure:

```bash
cp .env.example .env
```

Required environment variables:
- `POSTGRES_PASSWORD`: Database password

Optional environment variables:
- `EXPORT_PLUGIN_PORT`: Server port (default: 3306)
- `EXPORT_STORAGE_PATH`: Export file storage path (default: /tmp/nself-exports)
- `EXPORT_DOWNLOAD_EXPIRY_HOURS`: Download link expiry (default: 24)
- `EXPORT_DELETION_COOLDOWN_HOURS`: Deletion cooldown period (default: 24)
- `EXPORT_API_KEY`: API key for authentication

## Usage

### Initialize Database

```bash
npm run build
node dist/cli.js init
```

### Start Server

```bash
# Development mode (with auto-reload)
npm run dev

# Production mode
npm start

# Custom port
node dist/cli.js server --port 3306
```

### CLI Commands

#### Export Operations

```bash
# Create export request
node dist/cli.js export create --requester user123 --user target_user --type user_data

# List exports
node dist/cli.js export list

# Show export details
node dist/cli.js export show <export-id>

# Process export (run manually)
node dist/cli.js export process <export-id>
```

#### Deletion Operations

```bash
# Create deletion request
node dist/cli.js delete create --requester user123 --user target_user --reason "User requested"

# List deletions
node dist/cli.js delete list

# Verify deletion with code
node dist/cli.js delete verify <deletion-id> --code 123456

# Process deletion after cooldown
node dist/cli.js delete process <deletion-id>

# Cancel deletion
node dist/cli.js delete cancel <deletion-id>
```

#### Import Operations

```bash
# Create import job
node dist/cli.js import create --requester admin --source /path/to/export.json

# List imports
node dist/cli.js import list

# Process import
node dist/cli.js import process <import-id>
```

#### Plugin Registry

```bash
# Register plugin
node dist/cli.js plugins register --name stripe --tables stripe_customers,stripe_subscriptions

# List registered plugins
node dist/cli.js plugins list

# Update plugin
node dist/cli.js plugins update <plugin-id> --enabled false

# Unregister plugin
node dist/cli.js plugins unregister <plugin-id>
```

#### Statistics

```bash
# Show plugin status
node dist/cli.js status

# Show detailed statistics
node dist/cli.js stats
```

## API Endpoints

### Export Endpoints

- `POST /v1/exports` - Create export request
- `GET /v1/exports` - List export requests
- `GET /v1/exports/:id` - Get export details
- `GET /v1/exports/:id/download?token=<token>` - Download export file
- `DELETE /v1/exports/:id` - Delete export request

### Deletion Endpoints

- `POST /v1/deletions` - Create deletion request
- `GET /v1/deletions` - List deletion requests
- `GET /v1/deletions/:id` - Get deletion details
- `POST /v1/deletions/:id/verify` - Verify deletion with code
- `POST /v1/deletions/:id/cancel` - Cancel deletion request

### Plugin Registry Endpoints

- `POST /v1/plugins` - Register plugin
- `GET /v1/plugins` - List registered plugins
- `PUT /v1/plugins/:id` - Update plugin
- `DELETE /v1/plugins/:id` - Unregister plugin

### Import Endpoints

- `POST /v1/import` - Create import job
- `GET /v1/import/:id` - Get import job status

### Utility Endpoints

- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /live` - Liveness check
- `GET /v1/status` - Plugin status
- `GET /v1/stats` - Statistics

## API Examples

### Create Export Request

```bash
curl -X POST http://localhost:3306/v1/exports \
  -H "Content-Type: application/json" \
  -d '{
    "requestType": "user_data",
    "requesterId": "admin",
    "targetUserId": "user123",
    "format": "json"
  }'
```

### Download Export

```bash
curl http://localhost:3306/v1/exports/<export-id>/download?token=<download-token> \
  -o export.json
```

### Create Deletion Request

```bash
curl -X POST http://localhost:3306/v1/deletions \
  -H "Content-Type: application/json" \
  -d '{
    "requesterId": "user123",
    "targetUserId": "user123",
    "reason": "User requested GDPR deletion"
  }'
```

### Verify Deletion

```bash
curl -X POST http://localhost:3306/v1/deletions/<deletion-id>/verify \
  -H "Content-Type: application/json" \
  -d '{
    "code": "123456"
  }'
```

### Register Plugin

```bash
curl -X POST http://localhost:3306/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{
    "pluginName": "stripe",
    "tables": ["stripe_customers", "stripe_subscriptions"],
    "userIdColumn": "customer_id"
  }'
```

## Database Schema

### Tables

1. **export_requests** - Export request tracking
2. **export_deletion_requests** - Deletion request tracking with verification
3. **export_plugin_registry** - Registered plugins for export/deletion
4. **export_import_jobs** - Import job tracking
5. **export_webhook_events** - Webhook event log

All tables include `source_account_id` for multi-account isolation.

## GDPR Compliance

This plugin implements GDPR requirements:

1. **Right to Access**: Users can export all their data
2. **Right to Erasure**: Users can request deletion with verification
3. **Data Portability**: Exports in standard formats (JSON, CSV)
4. **Verification**: Two-step deletion with verification code
5. **Cooldown Period**: 24-hour cooldown before deletion executes
6. **Audit Trail**: All operations tracked in database

## Security

- API key authentication (optional)
- Rate limiting (50 requests/minute by default)
- Download tokens expire after 24 hours
- Verification codes for deletion
- Cooldown period prevents accidental deletion

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Watch mode
npm run watch

# Type checking
npm run typecheck

# Development server
npm run dev
```

## License

MIT
