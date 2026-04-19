# Plugin Configuration Mapping Guide

## Overview

This document explains how to map environment variables between **nself-tv backend** (or other nself applications) and **nself plugins**. Understanding this mapping is critical for proper plugin configuration.

## The Problem

nself applications (like nself-tv) use **prefixed** environment variables in their backend configuration:

```bash
# Backend .env.dev
FILE_PROCESSING_PLUGIN_ENABLED=true
FILE_PROCESSING_PLUGIN_PORT=3104
MINIO_ENDPOINT=http://localhost:9000
MINIO_BUCKET_RAW=media-raw
```

But plugins expect **unprefixed** environment variables:

```bash
# Plugin .env
DATABASE_URL=postgresql://...
PORT=3104
FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_ENDPOINT=http://localhost:9000
FILE_STORAGE_BUCKET=media-raw
```

**Why?** The backend needs to manage multiple plugins and services, so it uses prefixes to organize configuration. Plugins are standalone services that use generic variable names for portability.

## Naming Conventions

### Backend Variables (Prefixed)

Backend variables follow these patterns:

1. **Plugin Control Variables** (managed by nself CLI):
   - `{PLUGIN_NAME}_PLUGIN_ENABLED` - Enable/disable plugin
   - `{PLUGIN_NAME}_PLUGIN_PORT` - Plugin server port

2. **Plugin-Specific Variables** (passed to plugin):
   - `{PLUGIN_NAME}_{SETTING}` - Plugin configuration
   - `{SERVICE}_{SETTING}` - Shared service configuration (e.g., `MINIO_*`, `REDIS_*`)

Examples:
```bash
FILE_PROCESSING_PLUGIN_ENABLED=true       # Control variable
FILE_PROCESSING_PLUGIN_PORT=3104          # Control variable
FILE_STORAGE_PROVIDER=minio               # Plugin variable
FILE_MAX_SIZE=104857600                   # Plugin variable
MINIO_ENDPOINT=http://localhost:9000      # Shared service variable
```

### Plugin Variables (Unprefixed)

Plugins use generic, unprefixed variables for portability:

```bash
DATABASE_URL=postgresql://...     # Always required
PORT=3104                         # Server port (from backend)
FILE_STORAGE_PROVIDER=minio       # Plugin-specific setting
FILE_STORAGE_ENDPOINT=...         # Maps from MINIO_ENDPOINT
FILE_STORAGE_BUCKET=...           # Maps from MINIO_BUCKET_*
```

## Common Mapping Patterns

### Pattern 1: Direct Mapping (No Transform)

Backend variable name matches plugin variable name:

| Backend Variable | Plugin Variable | Notes |
|-----------------|-----------------|-------|
| `DATABASE_URL` | `DATABASE_URL` | Same name |
| `REDIS_URL` | `REDIS_URL` | Same name |
| `EPG_PLUGIN_PORT` | `PORT` | Backend port → plugin PORT |

### Pattern 2: Service → Generic Mapping

Backend service-specific variables map to plugin generic variables:

| Backend Variable | Plugin Variable | Example |
|-----------------|-----------------|---------|
| `MINIO_ENDPOINT` | `FILE_STORAGE_ENDPOINT` | MinIO URL → generic storage URL |
| `MINIO_ACCESS_KEY` | `FILE_STORAGE_ACCESS_KEY` | MinIO key → generic key |
| `MINIO_BUCKET_RAW` | `FILE_STORAGE_BUCKET` | Bucket name → bucket name |

### Pattern 3: Prefix Stripping

Backend prefixed variables map to unprefixed plugin variables:

| Backend Variable | Plugin Variable | Transform |
|-----------------|-----------------|-----------|
| `SPORTS_PROVIDER` | `SPORTS_PROVIDER` | Same name (plugin also uses prefix) |
| `SPORTS_ESPN_API_KEY` | `SPORTS_ESPN_API_KEY` | Same name |
| `REC_DEFAULT_LEAD_TIME_MINUTES` | `REC_DEFAULT_LEAD_TIME_MINUTES` | Same name |

**Note**: Many plugins retain their prefix in variable names (e.g., `SPORTS_*`, `REC_*`, `EPG_*`) for clarity.

### Pattern 4: Control Variables (Backend Only)

These variables are consumed by the nself CLI and NOT passed to plugins:

| Backend Variable | Purpose | Passed to Plugin? |
|-----------------|---------|------------------|
| `{PLUGIN}_PLUGIN_ENABLED` | Enable/disable plugin | ❌ No |
| `{PLUGIN}_PLUGIN_PORT` | Plugin port (passed as `PORT`) | ✅ Yes (as `PORT`) |

## Configuration Workflow

### Step 1: Identify Backend Variables

From your backend `.env.dev`, identify all variables for a plugin:

```bash
# Example: file-processing plugin
cd ~/Sites/nself-tv/backend
grep "FILE_PROCESSING\|MINIO" .env.dev
```

### Step 2: Read Plugin Documentation

Check the plugin's README.md for:
- Required environment variables
- Optional environment variables
- Configuration mapping table (if available)

```bash
# Example
cat ~/Sites/nself-plugins/plugins/file-processing/README.md
```

### Step 3: Create Plugin .env File

Map backend variables to plugin variables:

```bash
# Plugin .env location
~/.nself/plugins/file-processing/ts/.env
```

Example mapping:
```bash
# From backend .env.dev → Plugin .env
DATABASE_URL=$DATABASE_URL                              # Direct copy
PORT=3104                                               # From FILE_PROCESSING_PLUGIN_PORT
FILE_STORAGE_PROVIDER=minio                             # Literal value
FILE_STORAGE_ENDPOINT=$MINIO_ENDPOINT                   # Service mapping
FILE_STORAGE_BUCKET=$MINIO_BUCKET_RAW                   # Service mapping
FILE_STORAGE_ACCESS_KEY=$MINIO_ACCESS_KEY               # Service mapping
FILE_STORAGE_SECRET_KEY=$MINIO_SECRET_KEY               # Service mapping
```

### Step 4: Use Shell Script for Automation

Create a helper script to automate mapping:

```bash
#!/bin/bash
# generate-plugin-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_NAME="file-processing"
PLUGIN_ENV="$HOME/.nself/plugins/$PLUGIN_NAME/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$FILE_PROCESSING_PLUGIN_PORT
FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_ENDPOINT=$MINIO_ENDPOINT
FILE_STORAGE_BUCKET=$MINIO_BUCKET_RAW
FILE_STORAGE_ACCESS_KEY=$MINIO_ACCESS_KEY
FILE_STORAGE_SECRET_KEY=$MINIO_SECRET_KEY
FILE_MAX_SIZE=${FILE_MAX_SIZE:-104857600}
EOF

echo "Created $PLUGIN_ENV"
```

## Common Pitfalls

### Pitfall 1: Missing Service Credentials

**Problem**: Backend uses MinIO, but plugin doesn't have `FILE_STORAGE_ACCESS_KEY`.

**Solution**: Map service credentials:
```bash
# Backend
MINIO_ACCESS_KEY=minioadmin

# Plugin .env
FILE_STORAGE_ACCESS_KEY=$MINIO_ACCESS_KEY
```

### Pitfall 2: Port Mismatch

**Problem**: Plugin starts on wrong port.

**Solution**: Always map `{PLUGIN}_PLUGIN_PORT` to `PORT`:
```bash
# Backend
FILE_PROCESSING_PLUGIN_PORT=3104

# Plugin .env
PORT=3104
```

### Pitfall 3: Missing DATABASE_URL

**Problem**: Plugin can't connect to database.

**Solution**: ALL plugins require `DATABASE_URL`:
```bash
# Backend (or global)
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself

# Plugin .env
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
```

### Pitfall 4: Provider-Specific Variables Not Set

**Problem**: Plugin supports multiple providers (MinIO, S3, GCS) but doesn't know which to use.

**Solution**: Explicitly set provider:
```bash
# Plugin .env
FILE_STORAGE_PROVIDER=minio  # or s3, gcs, r2, b2, azure
```

### Pitfall 5: URL Construction Errors

**Problem**: Service URLs not properly constructed.

**Solution**: Check plugin config.ts for URL expectations:
- Some expect full URLs: `http://localhost:9000`
- Some expect hostname only: `localhost`
- Some expect hostname:port: `localhost:9000`

## Multi-App Configuration

### What is source_account_id?

All plugins use `source_account_id` to isolate data between applications:

```sql
-- Every plugin table has this column
CREATE TABLE np_fileproc_jobs (
  id UUID PRIMARY KEY,
  source_account_id VARCHAR(255) DEFAULT 'primary',
  -- ... other columns
);
```

### Backend Variable

Set which app(s) a plugin serves:

```bash
# Backend .env.dev
EPG_APP_IDS=nself-tv          # Single app
SPORTS_APP_IDS=nself-tv       # Single app

# Or multiple apps
EPG_APP_IDS=nself-tv,nself-family  # Multiple apps
```

### Plugin Behavior

Plugins use `source_account_id` to:
- Filter data by app
- Isolate tenants
- Support multi-tenancy

```javascript
// Plugin automatically filters by source_account_id
const jobs = await db.query(
  'SELECT * FROM np_fileproc_jobs WHERE source_account_id = $1',
  [sourceAccountId]
);
```

## Service-Specific Mappings

### MinIO / Object Storage

| Backend Variable | Plugin Variable | Notes |
|-----------------|-----------------|-------|
| `MINIO_ENDPOINT` | `FILE_STORAGE_ENDPOINT` | Full URL |
| `MINIO_BUCKET_*` | `FILE_STORAGE_BUCKET` | Choose appropriate bucket |
| `MINIO_ACCESS_KEY` | `FILE_STORAGE_ACCESS_KEY` | Credentials |
| `MINIO_SECRET_KEY` | `FILE_STORAGE_SECRET_KEY` | Credentials |
| `MINIO_REGION` | `FILE_STORAGE_REGION` | Optional, defaults to us-east-1 |

### Redis

| Backend Variable | Plugin Variable | Notes |
|-----------------|-----------------|-------|
| `REDIS_URL` | `REDIS_URL` | Direct mapping |
| `JOBS_REDIS_URL` | `JOBS_REDIS_URL` | Jobs plugin specific |

### PostgreSQL

| Backend Variable | Plugin Variable | Notes |
|-----------------|-----------------|-------|
| `DATABASE_URL` | `DATABASE_URL` | Direct mapping (all plugins) |
| `POSTGRES_HOST` | - | Not used by plugins |
| `POSTGRES_PORT` | - | Not used by plugins |
| `POSTGRES_DB` | - | Not used by plugins |

**Plugins use `DATABASE_URL` only.**

## Plugin-Specific Guides

Each critical plugin has detailed configuration mapping in its README.md:

- [file-processing](plugins/file-processing/README.md#configuration-mapping)
- [devices](plugins/devices/README.md#configuration-mapping)
- [recording](plugins/recording/README.md#configuration-mapping)
- [epg](plugins/epg/README.md#configuration-mapping)
- [stream-gateway](plugins/stream-gateway/README.md#configuration-mapping)
- [sports](plugins/sports/README.md#configuration-mapping)
- [jobs](plugins/jobs/README.md#configuration-mapping)
- [workflows](plugins/workflows/README.md#configuration-mapping)
- [torrent-manager](plugins/torrent-manager/README.md#configuration-mapping)

## Testing Your Configuration

### 1. Validate Plugin .env File

```bash
# Check required variables are set
cd ~/.nself/plugins/file-processing/ts
source .env
echo $DATABASE_URL     # Should be set
echo $PORT             # Should be set
echo $FILE_STORAGE_PROVIDER  # Should be set
```

### 2. Test Plugin Startup

```bash
cd ~/.nself/plugins/file-processing/ts
npm run dev
```

Watch for errors like:
- ❌ `DATABASE_URL is required`
- ❌ `FILE_STORAGE_BUCKET is required`
- ❌ `minio requires FILE_STORAGE_ACCESS_KEY`

### 3. Test Plugin Endpoints

```bash
# Health check
curl http://localhost:3104/health

# Status endpoint
curl http://localhost:3104/api/status
```

## Troubleshooting

### Error: "DATABASE_URL is required"

**Cause**: Plugin .env missing `DATABASE_URL`.

**Fix**:
```bash
# Add to plugin .env
DATABASE_URL=postgresql://postgres:password@localhost:5432/nself
```

### Error: "minio requires FILE_STORAGE_ACCESS_KEY"

**Cause**: Using MinIO provider but credentials not set.

**Fix**:
```bash
# Add to plugin .env
FILE_STORAGE_ACCESS_KEY=$MINIO_ACCESS_KEY
FILE_STORAGE_SECRET_KEY=$MINIO_SECRET_KEY
```

### Error: "Connection refused"

**Cause**: Service URL incorrect or service not running.

**Fix**:
1. Check service is running: `docker ps | grep minio`
2. Check URL format: `http://localhost:9000` (include protocol)
3. Check port matches backend configuration

### Plugin starts on wrong port

**Cause**: `PORT` not set in plugin .env.

**Fix**:
```bash
# Map from backend
PORT=$FILE_PROCESSING_PLUGIN_PORT  # or literal value
PORT=3104
```

## Best Practices

1. **Use Shell Variables**: Don't hardcode values, reference backend variables
   ```bash
   # Good
   FILE_STORAGE_ENDPOINT=$MINIO_ENDPOINT

   # Bad
   FILE_STORAGE_ENDPOINT=http://localhost:9000
   ```

2. **Document Custom Mappings**: If you use non-standard mappings, document them

3. **Version Control**: Keep plugin .env.example in version control, not .env

4. **Automate**: Create scripts to generate plugin .env from backend .env

5. **Validate**: Always test plugin startup after configuration changes

6. **Use Defaults**: Plugins provide sensible defaults, only override when needed

## Need Help?

- Check plugin README.md for specific mapping tables
- Review plugin config.ts for expected variable names
- Check backend .env.dev for available variables
- Test with `npm run dev` and watch for error messages

## Contributing

If you find incorrect mappings or missing documentation:

1. Update plugin README.md with correct mapping
2. Test the configuration
3. Submit PR with documentation fixes
