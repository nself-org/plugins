# Migration Guide

Comprehensive guide for database migrations, plugin updates, and data transfers in the nself plugin ecosystem.

**Last Updated**: January 30, 2026
**Version**: 1.0.0

---

## Table of Contents

1. [Database Migration Patterns](#database-migration-patterns)
2. [Plugin Updates](#plugin-updates)
3. [Breaking Changes](#breaking-changes)
4. [Data Backups](#data-backups)
5. [Rollback Procedures](#rollback-procedures)
6. [Zero-Downtime Migrations](#zero-downtime-migrations)
7. [Migration from Other Systems](#migration-from-other-systems)
8. [Version Compatibility Matrix](#version-compatibility-matrix)
9. [Migration Scripts](#migration-scripts)
10. [Testing Migrations](#testing-migrations)

---

## Database Migration Patterns

### Schema Evolution Strategy

The nself plugin ecosystem uses **additive migrations** to ensure backward compatibility:

- **Always add, never remove** columns in production
- Use `ALTER TABLE ADD COLUMN` instead of `DROP COLUMN`
- Mark deprecated columns with `_deprecated` suffix
- Create new tables instead of restructuring existing ones

### Migration File Structure

```
plugins/<name>/migrations/
├── 001_initial_schema.sql
├── 002_add_customer_metadata.sql
├── 003_add_subscription_pause.sql
└── rollback/
    ├── 001_rollback_initial.sql
    ├── 002_rollback_customer_metadata.sql
    └── 003_rollback_subscription_pause.sql
```

### Example: Adding a New Column

**Migration: `002_add_customer_metadata.sql`**

```sql
-- Migration: Add customer metadata columns
-- Version: 1.1.0
-- Date: 2026-01-30
-- Rollback: See rollback/002_rollback_customer_metadata.sql

-- Add new columns with defaults to avoid table locks on large tables
ALTER TABLE stripe_customers
ADD COLUMN IF NOT EXISTS marketing_opt_in BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMP WITH TIME ZONE,
ADD COLUMN IF NOT EXISTS preferences JSONB DEFAULT '{}';

-- Add index for performance
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_stripe_customers_last_activity
  ON stripe_customers(last_activity_at DESC)
  WHERE last_activity_at IS NOT NULL;

-- Backfill data in batches to avoid long-running transactions
DO $$
DECLARE
  batch_size INT := 1000;
  offset_val INT := 0;
  updated_count INT;
BEGIN
  LOOP
    -- Update in batches
    WITH batch AS (
      SELECT id FROM stripe_customers
      WHERE last_activity_at IS NULL
      ORDER BY id
      LIMIT batch_size
      OFFSET offset_val
    )
    UPDATE stripe_customers
    SET last_activity_at = updated_at
    WHERE id IN (SELECT id FROM batch);

    GET DIAGNOSTICS updated_count = ROW_COUNT;
    EXIT WHEN updated_count = 0;

    offset_val := offset_val + batch_size;
    RAISE NOTICE 'Updated % rows (offset: %)', updated_count, offset_val;

    -- Add delay to reduce load
    PERFORM pg_sleep(0.1);
  END LOOP;
END $$;

-- Validate migration
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'stripe_customers'
      AND column_name = 'marketing_opt_in'
  ) THEN
    RAISE EXCEPTION 'Migration failed: marketing_opt_in column not created';
  END IF;
END $$;

-- Migration complete
SELECT 'Migration 002 completed successfully' AS status;
```

**Rollback: `rollback/002_rollback_customer_metadata.sql`**

```sql
-- Rollback: Remove customer metadata columns
-- CAUTION: This will permanently delete data

-- Drop indexes first
DROP INDEX IF EXISTS idx_stripe_customers_last_activity;

-- Drop columns
ALTER TABLE stripe_customers
DROP COLUMN IF EXISTS marketing_opt_in,
DROP COLUMN IF EXISTS last_activity_at,
DROP COLUMN IF EXISTS preferences;

-- Rollback complete
SELECT 'Rollback 002 completed' AS status;
```

### Example: Adding a New Table

**Migration: `003_add_subscription_schedules.sql`**

```sql
-- Migration: Add subscription schedule tracking
-- Version: 1.2.0
-- Date: 2026-01-30

CREATE TABLE IF NOT EXISTS stripe_subscription_schedules (
  id VARCHAR(255) PRIMARY KEY,
  customer_id VARCHAR(255) REFERENCES stripe_customers(id) ON DELETE CASCADE,
  subscription_id VARCHAR(255),
  status VARCHAR(50) NOT NULL,
  phases JSONB DEFAULT '[]',
  current_phase JSONB,
  metadata JSONB DEFAULT '{}',
  released_at TIMESTAMP WITH TIME ZONE,
  completed_at TIMESTAMP WITH TIME ZONE,
  canceled_at TIMESTAMP WITH TIME ZONE,
  created_at TIMESTAMP WITH TIME ZONE NOT NULL,
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
  synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_subscription_schedules_customer
  ON stripe_subscription_schedules(customer_id);

CREATE INDEX IF NOT EXISTS idx_subscription_schedules_subscription
  ON stripe_subscription_schedules(subscription_id)
  WHERE subscription_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_subscription_schedules_status
  ON stripe_subscription_schedules(status);

CREATE INDEX IF NOT EXISTS idx_subscription_schedules_created
  ON stripe_subscription_schedules(created_at DESC);

-- Add foreign key to existing subscriptions table
ALTER TABLE stripe_subscriptions
ADD COLUMN IF NOT EXISTS schedule_id VARCHAR(255)
  REFERENCES stripe_subscription_schedules(id) ON DELETE SET NULL;

-- Migration tracking
INSERT INTO schema_migrations (version, applied_at, description)
VALUES ('003', NOW(), 'Add subscription schedules table');

SELECT 'Migration 003 completed successfully' AS status;
```

### Example: Modifying Data Types

**Migration: `004_extend_metadata_fields.sql`**

```sql
-- Migration: Extend metadata fields for better performance
-- Strategy: Create new columns, migrate data, then swap

-- Step 1: Add new columns with TEXT type
ALTER TABLE stripe_customers
ADD COLUMN IF NOT EXISTS metadata_new TEXT;

-- Step 2: Migrate data in batches
DO $$
DECLARE
  batch_size INT := 5000;
  total_rows INT;
  processed INT := 0;
BEGIN
  SELECT COUNT(*) INTO total_rows FROM stripe_customers;

  WHILE processed < total_rows LOOP
    UPDATE stripe_customers
    SET metadata_new = metadata::TEXT
    WHERE id IN (
      SELECT id FROM stripe_customers
      WHERE metadata_new IS NULL
      ORDER BY id
      LIMIT batch_size
    );

    processed := processed + batch_size;
    RAISE NOTICE 'Migrated % / % rows', LEAST(processed, total_rows), total_rows;
    PERFORM pg_sleep(0.5);
  END LOOP;
END $$;

-- Step 3: Verify all data migrated
DO $$
DECLARE
  null_count INT;
BEGIN
  SELECT COUNT(*) INTO null_count
  FROM stripe_customers
  WHERE metadata IS NOT NULL AND metadata_new IS NULL;

  IF null_count > 0 THEN
    RAISE EXCEPTION 'Migration incomplete: % rows not migrated', null_count;
  END IF;
END $$;

-- Step 4: Swap columns (in a separate transaction after verification)
-- This step should be run manually after validation
-- ALTER TABLE stripe_customers RENAME COLUMN metadata TO metadata_old;
-- ALTER TABLE stripe_customers RENAME COLUMN metadata_new TO metadata;
-- Then drop metadata_old after confirming everything works

SELECT 'Migration 004 Phase 1 completed - Verify before proceeding to Phase 2' AS status;
```

### Handling Large Tables

For tables with millions of rows, use these patterns:

```sql
-- Pattern 1: Add column without default (fast)
ALTER TABLE large_table ADD COLUMN new_field TEXT;

-- Pattern 2: Set default in batches (avoids table rewrite)
DO $$
DECLARE
  batch_size INT := 10000;
BEGIN
  LOOP
    UPDATE large_table
    SET new_field = 'default_value'
    WHERE id IN (
      SELECT id FROM large_table
      WHERE new_field IS NULL
      LIMIT batch_size
    );

    EXIT WHEN NOT FOUND;
    COMMIT; -- For long-running operations
    PERFORM pg_sleep(1);
  END LOOP;
END $$;

-- Pattern 3: Add NOT NULL constraint after backfill
ALTER TABLE large_table
ALTER COLUMN new_field SET NOT NULL;
```

### Creating Indexes Safely

```sql
-- Use CONCURRENTLY to avoid table locks
CREATE INDEX CONCURRENTLY idx_orders_created_at
  ON shopify_orders(created_at DESC);

-- For unique indexes, use CONCURRENTLY as well
CREATE UNIQUE INDEX CONCURRENTLY idx_products_sku
  ON shopify_products(sku)
  WHERE sku IS NOT NULL;

-- Partial indexes for filtered queries
CREATE INDEX CONCURRENTLY idx_orders_unfulfilled
  ON shopify_orders(created_at DESC)
  WHERE fulfillment_status IS NULL OR fulfillment_status = 'unfulfilled';
```

---

## Plugin Updates

### Semantic Versioning

nself plugins follow [Semantic Versioning](https://semver.org/):

- **MAJOR** (1.0.0 → 2.0.0): Breaking changes, incompatible API changes
- **MINOR** (1.0.0 → 1.1.0): New features, backward compatible
- **PATCH** (1.0.0 → 1.0.1): Bug fixes, backward compatible

### Checking for Updates

```bash
# Check all plugins for updates
nself plugin updates

# Output:
# stripe: 1.0.0 → 1.1.0 (minor update available)
# github: 1.0.0 (up to date)
# shopify: 1.0.0 → 1.0.1 (patch available)

# Check specific plugin
nself plugin status stripe
```

### Updating a Plugin

#### Patch/Minor Updates (Safe)

```bash
# Patch updates (1.0.0 → 1.0.1) - Auto-update safe
nself plugin update stripe

# Minor updates (1.0.0 → 1.1.0) - Review changelog
nself plugin update stripe --minor

# The update process:
# 1. Downloads new plugin version
# 2. Backs up current database
# 3. Runs migration scripts
# 4. Updates plugin files
# 5. Verifies installation
```

#### Major Updates (Review Required)

```bash
# Major updates (1.x.x → 2.0.0) require manual intervention
nself plugin update stripe --major

# You'll be prompted:
# ⚠️  WARNING: This is a MAJOR version update with breaking changes.
#
# Breaking changes in v2.0.0:
# - Removed deprecated columns: stripe_customers.old_metadata
# - Changed API endpoint: /api/subscriptions → /api/billing/subscriptions
# - Updated environment variable: STRIPE_API_KEY → STRIPE_SECRET_KEY
#
# Before proceeding:
# 1. Review full changelog: https://github.com/.../CHANGELOG.md
# 2. Update your application code
# 3. Update environment variables
# 4. Backup your database
#
# Continue? (yes/no):
```

### Manual Update Process

```bash
# 1. Check current version
nself plugin status stripe
# Current: 1.0.0

# 2. Backup database
pg_dump $DATABASE_URL > backup_stripe_$(date +%Y%m%d_%H%M%S).sql

# 3. Read changelog
nself plugin changelog stripe --version 1.1.0

# 4. Download update
nself plugin update stripe --version 1.1.0

# 5. Run migrations
cd plugins/stripe/ts
npm run migrate

# 6. Test in development
npm run dev

# 7. Restart production server
pm2 restart stripe-plugin
```

### Rollback After Update

If an update causes issues:

```bash
# Rollback to previous version
nself plugin rollback stripe --to-version 1.0.0

# Restore database backup
psql $DATABASE_URL < backup_stripe_20260130_120000.sql

# Restart with previous version
pm2 restart stripe-plugin
```

---

## Breaking Changes

### Identifying Breaking Changes

Breaking changes are clearly marked in:

1. **CHANGELOG.md** - `### BREAKING CHANGES` section
2. **Plugin version** - Major version bump (1.x → 2.x)
3. **Migration files** - Named with `BREAKING_` prefix

### Example Breaking Change: API Endpoint Rename

**Before (v1.x.x):**
```typescript
// Old endpoint
GET /api/subscriptions
```

**After (v2.0.0):**
```typescript
// New endpoint structure
GET /api/billing/subscriptions
```

**Migration path:**

```typescript
// Option 1: Support both (deprecated route)
// In server.ts v2.0.0
app.get('/api/subscriptions', async (req, reply) => {
  logger.warn('Deprecated endpoint /api/subscriptions used. Update to /api/billing/subscriptions');
  return reply.redirect(301, '/api/billing/subscriptions');
});

app.get('/api/billing/subscriptions', async (req, reply) => {
  // New implementation
});

// Option 2: Hard cutover with clear error message
app.get('/api/subscriptions', async (req, reply) => {
  return reply.code(410).send({
    error: 'Gone',
    message: 'This endpoint has been removed in v2.0.0. Use /api/billing/subscriptions instead.',
    migration_guide: 'https://docs.nself.org/plugins/stripe/migration-v2'
  });
});
```

### Example Breaking Change: Column Removal

**v1.x.x Schema:**
```sql
CREATE TABLE stripe_customers (
  id VARCHAR(255) PRIMARY KEY,
  email VARCHAR(255),
  old_metadata TEXT,  -- Deprecated in v1.5.0
  metadata JSONB,      -- Added in v1.5.0
  ...
);
```

**v2.0.0 Migration:**

```sql
-- Migration: Remove deprecated columns
-- BREAKING CHANGE: old_metadata column removed

-- Step 1: Verify no code uses old_metadata
DO $$
DECLARE
  usage_count INT;
BEGIN
  -- Check application logs for usage
  SELECT COUNT(*) INTO usage_count
  FROM pg_stat_statements
  WHERE query LIKE '%old_metadata%';

  IF usage_count > 0 THEN
    RAISE WARNING 'old_metadata still in use. Found % queries.', usage_count;
  END IF;
END $$;

-- Step 2: Final migration opportunity
UPDATE stripe_customers
SET metadata = to_jsonb(old_metadata::text)
WHERE metadata IS NULL OR metadata = '{}'::jsonb;

-- Step 3: Remove column
ALTER TABLE stripe_customers DROP COLUMN old_metadata;

-- Step 4: Mark as breaking change
INSERT INTO breaking_changes (version, table_name, change, migration_date)
VALUES ('2.0.0', 'stripe_customers', 'Removed old_metadata column', NOW());
```

### Handling Breaking Changes in Application Code

```typescript
// Update your application code before upgrading

// OLD CODE (v1.x - deprecated):
const customers = await fetch('/api/subscriptions');

// NEW CODE (v2.0+):
const customers = await fetch('/api/billing/subscriptions');

// TRANSITION CODE (supports both):
const endpoint = pluginVersion >= '2.0.0'
  ? '/api/billing/subscriptions'
  : '/api/subscriptions';
const customers = await fetch(endpoint);
```

---

## Data Backups

### Automated Backup Strategy

```bash
#!/bin/bash
# backup_plugin_data.sh
# Automated backup script for nself plugins

PLUGIN_NAME=$1
BACKUP_DIR="/var/backups/nself-plugins"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
DATABASE_URL="${DATABASE_URL}"

# Create backup directory
mkdir -p "$BACKUP_DIR/$PLUGIN_NAME"

# Backup database tables
pg_dump "$DATABASE_URL" \
  --table="${PLUGIN_NAME}_*" \
  --format=custom \
  --file="$BACKUP_DIR/$PLUGIN_NAME/db_${TIMESTAMP}.dump"

# Backup configuration
cp -r "plugins/$PLUGIN_NAME/.env" "$BACKUP_DIR/$PLUGIN_NAME/env_${TIMESTAMP}"

# Backup plugin code (for rollback)
tar -czf "$BACKUP_DIR/$PLUGIN_NAME/code_${TIMESTAMP}.tar.gz" \
  "plugins/$PLUGIN_NAME"

# Create metadata
cat > "$BACKUP_DIR/$PLUGIN_NAME/metadata_${TIMESTAMP}.json" <<EOF
{
  "plugin": "$PLUGIN_NAME",
  "version": "$(cat plugins/$PLUGIN_NAME/plugin.json | jq -r '.version')",
  "timestamp": "$TIMESTAMP",
  "database_url": "${DATABASE_URL%%@*}@***",
  "tables_count": $(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_name LIKE '${PLUGIN_NAME}_%'")
}
EOF

# Clean up old backups (keep last 30 days)
find "$BACKUP_DIR/$PLUGIN_NAME" -type f -mtime +30 -delete

echo "✅ Backup completed: $BACKUP_DIR/$PLUGIN_NAME/*_${TIMESTAMP}.*"
```

### Pre-Migration Backup Checklist

Before any migration:

- [ ] **Full database backup** - `pg_dump` with custom format
- [ ] **Plugin code backup** - Current version tarball
- [ ] **Environment backup** - `.env` file copy
- [ ] **Test restore** - Verify backup is valid
- [ ] **Document current state** - Table counts, row counts
- [ ] **Tag current version** - Git tag for rollback point

```bash
# Complete pre-migration backup
./scripts/backup_plugin_data.sh stripe

# Verify backup
pg_restore --list /var/backups/nself-plugins/stripe/db_20260130_120000.dump

# Test restore in isolated environment
createdb test_stripe_restore
pg_restore -d test_stripe_restore /var/backups/nself-plugins/stripe/db_20260130_120000.dump
psql test_stripe_restore -c "SELECT COUNT(*) FROM stripe_customers"
```

### Incremental Backup Strategy

```sql
-- Create backup tracking table
CREATE TABLE IF NOT EXISTS backup_checkpoints (
  id SERIAL PRIMARY KEY,
  plugin_name VARCHAR(100),
  table_name VARCHAR(255),
  checkpoint_type VARCHAR(50), -- 'full' or 'incremental'
  last_synced_at TIMESTAMP WITH TIME ZONE,
  rows_backed_up BIGINT,
  backup_location TEXT,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Incremental backup query
-- Only backup rows modified since last checkpoint
WITH last_checkpoint AS (
  SELECT last_synced_at
  FROM backup_checkpoints
  WHERE plugin_name = 'stripe'
    AND table_name = 'stripe_customers'
    AND checkpoint_type = 'incremental'
  ORDER BY created_at DESC
  LIMIT 1
)
SELECT *
FROM stripe_customers
WHERE synced_at > (SELECT last_synced_at FROM last_checkpoint)
   OR updated_at > (SELECT last_synced_at FROM last_checkpoint);
```

### Point-in-Time Recovery Setup

```bash
# Enable WAL archiving for point-in-time recovery
# Add to postgresql.conf:
wal_level = replica
archive_mode = on
archive_command = 'cp %p /var/lib/postgresql/wal_archive/%f'
max_wal_senders = 3

# Create base backup
pg_basebackup -D /var/backups/postgresql/base -Ft -z -P

# Recover to specific point in time
# Create recovery.conf:
restore_command = 'cp /var/lib/postgresql/wal_archive/%f %p'
recovery_target_time = '2026-01-30 12:00:00'
```

---

## Rollback Procedures

### Quick Rollback Checklist

When a migration fails or causes issues:

1. **Stop the application** - Prevent new data writes
2. **Identify rollback point** - Last known good state
3. **Restore database** - From pre-migration backup
4. **Restore code** - Revert to previous plugin version
5. **Verify data integrity** - Check row counts, test queries
6. **Restart application** - With previous version
7. **Document incident** - What failed, why, lessons learned

### Automated Rollback Script

```bash
#!/bin/bash
# rollback_migration.sh
# Automated rollback for failed migrations

PLUGIN_NAME=$1
BACKUP_TIMESTAMP=$2
BACKUP_DIR="/var/backups/nself-plugins"

set -e  # Exit on error

echo "🔄 Starting rollback for $PLUGIN_NAME..."

# Step 1: Stop the plugin server
echo "1. Stopping $PLUGIN_NAME server..."
pm2 stop "${PLUGIN_NAME}-plugin" || true

# Step 2: Verify backup exists
BACKUP_FILE="$BACKUP_DIR/$PLUGIN_NAME/db_${BACKUP_TIMESTAMP}.dump"
if [ ! -f "$BACKUP_FILE" ]; then
  echo "❌ Error: Backup file not found: $BACKUP_FILE"
  exit 1
fi

# Step 3: Drop existing tables (with confirmation)
echo "2. Dropping current tables..."
psql "$DATABASE_URL" <<SQL
DO \$\$
DECLARE
  table_name text;
BEGIN
  FOR table_name IN
    SELECT tablename
    FROM pg_tables
    WHERE tablename LIKE '${PLUGIN_NAME}_%'
  LOOP
    EXECUTE 'DROP TABLE IF EXISTS ' || table_name || ' CASCADE';
    RAISE NOTICE 'Dropped table %', table_name;
  END LOOP;
END \$\$;
SQL

# Step 4: Restore from backup
echo "3. Restoring database from backup..."
pg_restore -d "$DATABASE_URL" \
  --clean \
  --if-exists \
  --no-owner \
  --no-acl \
  "$BACKUP_FILE"

# Step 5: Restore plugin code
echo "4. Restoring plugin code..."
CODE_BACKUP="$BACKUP_DIR/$PLUGIN_NAME/code_${BACKUP_TIMESTAMP}.tar.gz"
tar -xzf "$CODE_BACKUP" -C "$(dirname "plugins/$PLUGIN_NAME")"

# Step 6: Verify restoration
echo "5. Verifying data restoration..."
TABLE_COUNT=$(psql "$DATABASE_URL" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_name LIKE '${PLUGIN_NAME}_%'")
echo "   Tables restored: $TABLE_COUNT"

# Step 7: Restart server
echo "6. Restarting $PLUGIN_NAME server..."
cd "plugins/$PLUGIN_NAME/ts"
npm run build
pm2 restart "${PLUGIN_NAME}-plugin"

echo "✅ Rollback completed successfully!"
echo ""
echo "Post-rollback checklist:"
echo "  [ ] Verify data integrity"
echo "  [ ] Test critical queries"
echo "  [ ] Check application functionality"
echo "  [ ] Review logs for errors"
echo "  [ ] Document what went wrong"
```

### Manual Rollback Steps

#### Database Rollback

```bash
# List available backups
ls -lh /var/backups/nself-plugins/stripe/

# Choose backup to restore
BACKUP_FILE="db_20260130_120000.dump"

# Create a safety backup of current state (even if broken)
pg_dump $DATABASE_URL --table="stripe_*" \
  > "emergency_backup_$(date +%Y%m%d_%H%M%S).sql"

# Drop all plugin tables
psql $DATABASE_URL <<EOF
DO \$\$
DECLARE r RECORD;
BEGIN
  FOR r IN (SELECT tablename FROM pg_tables WHERE tablename LIKE 'stripe_%') LOOP
    EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE';
  END LOOP;
END \$\$;
EOF

# Restore from backup
pg_restore -d $DATABASE_URL /var/backups/nself-plugins/stripe/$BACKUP_FILE

# Verify restoration
psql $DATABASE_URL -c "SELECT COUNT(*) FROM stripe_customers"
```

#### Code Rollback

```bash
# Option 1: Git rollback (if versioned)
cd plugins/stripe
git log --oneline  # Find commit hash
git reset --hard abc1234
npm run build

# Option 2: From backup tarball
tar -xzf /var/backups/nself-plugins/stripe/code_20260130_120000.tar.gz \
  -C plugins/

# Rebuild
cd plugins/stripe/ts
npm install
npm run build

# Restart
pm2 restart stripe-plugin
```

### Partial Rollback (Single Table)

Sometimes you only need to rollback one table:

```sql
-- Step 1: Export table from backup to temp table
pg_restore -d $DATABASE_URL \
  --table=stripe_customers \
  --schema-only \
  /var/backups/nself-plugins/stripe/db_20260130_120000.dump

CREATE TEMP TABLE stripe_customers_backup AS
TABLE stripe_customers;

-- Step 2: Restore data
pg_restore -d $DATABASE_URL \
  --table=stripe_customers \
  --data-only \
  /var/backups/nself-plugins/stripe/db_20260130_120000.dump

-- Step 3: Verify
SELECT
  'current' AS source,
  COUNT(*) AS row_count,
  MAX(updated_at) AS last_update
FROM stripe_customers
UNION ALL
SELECT
  'backup' AS source,
  COUNT(*) AS row_count,
  MAX(updated_at) AS last_update
FROM stripe_customers_backup;
```

---

## Zero-Downtime Migrations

### Blue-Green Deployment Strategy

Run two parallel plugin instances during migration:

```
┌─────────────────────────────────────────────┐
│           Load Balancer / Nginx             │
└──────────────┬──────────────────────────────┘
               │
       ┌───────┴───────┐
       │               │
┌──────▼─────┐  ┌──────▼─────┐
│   Blue     │  │   Green    │
│ (Current)  │  │   (New)    │
│ v1.0.0     │  │  v1.1.0    │
│ Port 3001  │  │ Port 3011  │
└──────┬─────┘  └──────┬─────┘
       │               │
       └───────┬───────┘
               │
     ┌─────────▼─────────┐
     │   PostgreSQL      │
     │  (Shared State)   │
     └───────────────────┘
```

**Implementation:**

```bash
#!/bin/bash
# blue_green_migration.sh

PLUGIN_NAME="stripe"
CURRENT_PORT=3001
NEW_PORT=3011
NGINX_CONFIG="/etc/nginx/sites-available/${PLUGIN_NAME}"

# Step 1: Deploy new version (green) alongside current (blue)
echo "Deploying green instance..."
cd plugins/$PLUGIN_NAME/ts
git checkout v1.1.0
npm install
npm run build

# Start on different port
PORT=$NEW_PORT npm run start &
GREEN_PID=$!

# Wait for health check
sleep 5
if ! curl -f "http://localhost:$NEW_PORT/health"; then
  echo "❌ Green instance failed to start"
  kill $GREEN_PID
  exit 1
fi

# Step 2: Run database migrations (non-blocking)
npm run migrate

# Step 3: Test green instance
echo "Testing green instance..."
./tests/smoke-test.sh "http://localhost:$NEW_PORT"

if [ $? -ne 0 ]; then
  echo "❌ Green instance tests failed"
  kill $GREEN_PID
  exit 1
fi

# Step 4: Gradually shift traffic (10% intervals)
for weight in 10 20 30 50 70 90 100; do
  echo "Shifting $weight% traffic to green..."

  # Update nginx config
  cat > "$NGINX_CONFIG" <<EOF
upstream ${PLUGIN_NAME}_backend {
  server localhost:$CURRENT_PORT weight=$((100 - weight));
  server localhost:$NEW_PORT weight=$weight;
}
EOF

  nginx -s reload

  # Monitor for errors
  sleep 30
  ERROR_RATE=$(tail -100 /var/log/nginx/access.log | grep " 500 " | wc -l)

  if [ $ERROR_RATE -gt 5 ]; then
    echo "❌ High error rate detected, rolling back..."
    # Shift back to blue
    cat > "$NGINX_CONFIG" <<EOF
upstream ${PLUGIN_NAME}_backend {
  server localhost:$CURRENT_PORT weight=100;
  server localhost:$NEW_PORT weight=0;
}
EOF
    nginx -s reload
    kill $GREEN_PID
    exit 1
  fi
done

# Step 5: Shutdown blue instance
echo "✅ Migration complete, shutting down blue instance..."
pm2 stop "${PLUGIN_NAME}-plugin"
pm2 delete "${PLUGIN_NAME}-plugin"

# Update PM2 to manage green as primary
pm2 start --name "${PLUGIN_NAME}-plugin" npm -- start
pm2 save

echo "✅ Blue-green migration completed successfully!"
```

### Read Replica Strategy

For database-heavy migrations:

```sql
-- Create read replica
CREATE PUBLICATION stripe_publication FOR TABLE
  stripe_customers,
  stripe_subscriptions,
  stripe_invoices;

-- On replica database
CREATE SUBSCRIPTION stripe_subscription
  CONNECTION 'host=primary.db user=replicator dbname=nself'
  PUBLICATION stripe_publication;

-- During migration, writes go to primary, reads can use replica
-- Application code:
const readDb = new Database(process.env.REPLICA_DATABASE_URL);
const writeDb = new Database(process.env.PRIMARY_DATABASE_URL);

// Read from replica (eventual consistency acceptable)
const customers = await readDb.query('SELECT * FROM stripe_customers');

// Write to primary (strong consistency required)
await writeDb.execute(
  'INSERT INTO stripe_customers VALUES (...)',
  params
);
```

### Shadow Mode Migration

Test new schema alongside old without affecting production:

```sql
-- Create shadow tables with new schema
CREATE TABLE stripe_customers_v2 (
  id VARCHAR(255) PRIMARY KEY,
  email VARCHAR(255),
  full_name VARCHAR(255), -- Changed from separate first/last
  metadata JSONB DEFAULT '{}',
  -- New schema
  ...
);

-- Dual-write: Write to both schemas during transition
CREATE OR REPLACE FUNCTION dual_write_customers()
RETURNS TRIGGER AS $$
BEGIN
  -- Write to v2 table
  INSERT INTO stripe_customers_v2 (id, email, full_name, metadata)
  VALUES (
    NEW.id,
    NEW.email,
    CONCAT(NEW.first_name, ' ', NEW.last_name),
    NEW.metadata
  )
  ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    full_name = EXCLUDED.full_name,
    metadata = EXCLUDED.metadata;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER customers_dual_write
AFTER INSERT OR UPDATE ON stripe_customers
FOR EACH ROW EXECUTE FUNCTION dual_write_customers();

-- After validating v2 data matches v1, swap tables
BEGIN;
  ALTER TABLE stripe_customers RENAME TO stripe_customers_v1_old;
  ALTER TABLE stripe_customers_v2 RENAME TO stripe_customers;
  DROP TRIGGER customers_dual_write ON stripe_customers_v1_old;
COMMIT;
```

---

## Migration from Other Systems

### Stripe: From stripe-sync-engine

If you're migrating from [supabase/stripe-sync-engine](https://github.com/supabase/stripe-sync-engine):

**Schema mapping:**

```sql
-- stripe-sync-engine uses "stripe" schema, nself uses public with "stripe_" prefix
-- Migration script:

-- Step 1: Export from stripe-sync-engine
pg_dump -n stripe > stripe_sync_engine_export.sql

-- Step 2: Transform schema
sed -i 's/CREATE TABLE stripe\./CREATE TABLE stripe_/g' stripe_sync_engine_export.sql
sed -i 's/FROM stripe\./FROM stripe_/g' stripe_sync_engine_export.sql
sed -i 's/JOIN stripe\./JOIN stripe_/g' stripe_sync_engine_export.sql

-- Step 3: Import to nself
psql $DATABASE_URL < stripe_sync_engine_export.sql

-- Step 4: Update column mappings (if any schema differences)
ALTER TABLE stripe_customers
ADD COLUMN IF NOT EXISTS synced_at TIMESTAMP WITH TIME ZONE DEFAULT NOW();
```

### Shopify: From Custom Integration

Migrating from a custom Shopify integration:

```typescript
// migration/shopify_import.ts

import { ShopifyDatabase } from '../src/database.js';
import { legacyDb } from './legacy_connection.js';

async function migrateFromLegacy() {
  const db = new ShopifyDatabase();

  // Step 1: Migrate shop data
  console.log('Migrating shop data...');
  const shops = await legacyDb.query(`
    SELECT * FROM legacy_shops
  `);

  for (const shop of shops.rows) {
    await db.execute(`
      INSERT INTO shopify_shops (
        id, name, domain, myshopify_domain, currency, timezone,
        created_at, updated_at, synced_at
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
      ON CONFLICT (id) DO UPDATE SET
        name = EXCLUDED.name,
        updated_at = NOW()
    `, [
      shop.shop_id,
      shop.shop_name,
      shop.custom_domain,
      shop.myshopify_domain,
      shop.currency_code,
      shop.tz,
      shop.created_date
    ]);
  }

  // Step 2: Migrate products
  console.log('Migrating products...');
  const products = await legacyDb.query(`
    SELECT
      p.product_id,
      p.title,
      p.description,
      p.vendor,
      p.product_type,
      p.handle,
      p.status,
      p.created_date,
      p.modified_date,
      JSON_AGG(
        JSON_BUILD_OBJECT(
          'id', i.image_id,
          'src', i.image_url,
          'position', i.position
        )
      ) AS images
    FROM legacy_products p
    LEFT JOIN legacy_product_images i ON p.product_id = i.product_id
    GROUP BY p.product_id
  `);

  for (const product of products.rows) {
    await db.execute(`
      INSERT INTO shopify_products (
        id, title, body_html, vendor, product_type, handle,
        status, images, created_at, updated_at, synced_at
      )
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
      ON CONFLICT (id) DO UPDATE SET
        title = EXCLUDED.title,
        body_html = EXCLUDED.body_html,
        images = EXCLUDED.images,
        updated_at = NOW()
    `, [
      product.product_id,
      product.title,
      product.description,
      product.vendor,
      product.product_type,
      product.handle,
      product.status,
      JSON.stringify(product.images),
      product.created_date,
      product.modified_date
    ]);
  }

  // Step 3: Migrate orders (with progress tracking)
  console.log('Migrating orders...');
  const totalOrders = await legacyDb.query(
    'SELECT COUNT(*) FROM legacy_orders'
  );
  const batchSize = 1000;
  let offset = 0;

  while (offset < totalOrders.rows[0].count) {
    const orders = await legacyDb.query(`
      SELECT * FROM legacy_orders
      ORDER BY order_id
      LIMIT $1 OFFSET $2
    `, [batchSize, offset]);

    for (const order of orders.rows) {
      // Transform and insert order
      await db.upsertOrder(transformLegacyOrder(order));
    }

    offset += batchSize;
    console.log(`Migrated ${offset} / ${totalOrders.rows[0].count} orders`);
  }

  console.log('✅ Migration complete!');
}

function transformLegacyOrder(legacy: any): ShopifyOrderRecord {
  return {
    id: legacy.order_id,
    order_number: legacy.order_number,
    name: legacy.name,
    email: legacy.email,
    customer_id: legacy.customer_id,
    financial_status: legacy.payment_status,
    fulfillment_status: legacy.shipment_status,
    total_price: legacy.total_amount,
    currency: legacy.currency_code,
    created_at: legacy.order_date,
    updated_at: legacy.modified_date,
    // ... map all other fields
  };
}

// Run migration
migrateFromLegacy().catch(console.error);
```

### GitHub: From GitHub Archive

Import historical GitHub data:

```bash
#!/bin/bash
# Import GitHub Archive data
# Download from: https://www.gharchive.org/

REPO="acamarata/nself-plugins"
START_DATE="2024-01-01"
END_DATE="2026-01-30"

# Download archive files
for date in $(seq -f "%Y-%m-%d" $(date -d "$START_DATE" +%s) 86400 $(date -d "$END_DATE" +%s)); do
  for hour in {0..23}; do
    FILE="${date}-${hour}.json.gz"
    URL="https://data.gharchive.org/${FILE}"

    echo "Downloading $FILE..."
    curl -O "$URL"

    # Extract events for our repo
    zcat "$FILE" | \
      jq -c "select(.repo.name == \"$REPO\")" | \
      while read event; do
        # Import to database via API
        curl -X POST http://localhost:3002/api/events/import \
          -H "Content-Type: application/json" \
          -d "$event"
      done

    rm "$FILE"
  done
done

echo "✅ GitHub Archive import complete"
```

---

## Version Compatibility Matrix

### Plugin Version Requirements

| Plugin Version | nself CLI Version | Node.js | PostgreSQL | Dependencies |
|----------------|-------------------|---------|------------|--------------|
| stripe@1.0.0 | ≥0.4.8 | ≥18.0.0 | ≥14.0 | stripe@14.x |
| github@1.0.0 | ≥0.4.8 | ≥18.0.0 | ≥14.0 | @octokit/rest@20.x |
| shopify@1.0.0 | ≥0.4.8 | ≥18.0.0 | ≥14.0 | @shopify/shopify-api@9.x |
| realtime@1.0.0 | ≥0.4.8 | ≥18.0.0 | ≥14.0 | socket.io@4.x, Redis@7.x |
| jobs@1.0.0 | ≥0.4.8 | ≥18.0.0 | ≥14.0 | bullmq@5.x, Redis@7.x |
| notifications@1.0.0 | ≥0.4.8 | ≥18.0.0 | ≥14.0 | resend@3.x, twilio@4.x |

### Cross-Plugin Compatibility

Some plugins work together and have version dependencies:

```
notifications@1.0.0
  └── jobs@1.0.0 (optional, for scheduled notifications)
      └── Redis 7.x (required)

file-processing@1.0.0
  └── jobs@1.0.0 (required, for async processing)
  └── MinIO/S3 (required, for storage)

realtime@1.0.0
  └── notifications@1.0.0 (optional, for push notifications)
  └── Redis 7.x (required for clustering)
```

### API Version Compatibility

#### Stripe API Versions

| Plugin Version | Stripe API Version | Notes |
|----------------|-------------------|-------|
| 1.0.0 | 2024-12-18 | Current default |
| 0.9.x | 2024-10-28 | Compatible with fallback |
| 0.8.x | 2024-06-20 | Deprecated, upgrade recommended |

**Migration between Stripe API versions:**

```typescript
// config.ts
export const STRIPE_API_VERSION = process.env.STRIPE_API_VERSION || '2024-12-18';

// client.ts
const stripe = new Stripe(apiKey, {
  apiVersion: STRIPE_API_VERSION,
  // Enable version upgrade warnings
  maxNetworkRetries: 2,
});

// Handle version-specific fields
if (STRIPE_API_VERSION >= '2024-12-18') {
  // Use new field structure
  customer.tax_ids = response.tax_ids;
} else {
  // Use legacy field
  customer.tax_ids = response.sources?.tax_ids || [];
}
```

#### Shopify API Versions

| Plugin Version | Shopify API Version | Notes |
|----------------|---------------------|-------|
| 1.0.0 | 2024-01 | Current default |
| 0.9.x | 2023-10 | Still supported |
| 0.8.x | 2023-07 | Deprecated, upgrade required |

**Shopify API version upgrade:**

```bash
# Update environment variable
SHOPIFY_API_VERSION=2024-01

# Update @shopify/shopify-api package
npm install @shopify/shopify-api@latest

# Test with new version
npm test

# Deploy
npm run build
pm2 restart shopify-plugin
```

---

## Migration Scripts

### Script Template

```typescript
// migrations/001_add_feature.ts

import { Database } from '@nself/plugin-utils';
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('migration:001');

interface MigrationMetadata {
  version: string;
  description: string;
  breaking: boolean;
  rollbackAvailable: boolean;
}

export const metadata: MigrationMetadata = {
  version: '001',
  description: 'Add customer preferences feature',
  breaking: false,
  rollbackAvailable: true,
};

export async function up(db: Database): Promise<void> {
  logger.info('Starting migration 001...');

  try {
    // Step 1: Add new columns
    await db.execute(`
      ALTER TABLE stripe_customers
      ADD COLUMN IF NOT EXISTS preferences JSONB DEFAULT '{}',
      ADD COLUMN IF NOT EXISTS marketing_opt_in BOOLEAN DEFAULT FALSE,
      ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMP WITH TIME ZONE;
    `);

    // Step 2: Create indexes
    await db.execute(`
      CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_customers_marketing
        ON stripe_customers(marketing_opt_in)
        WHERE marketing_opt_in = TRUE;
    `);

    // Step 3: Backfill data in batches
    let offset = 0;
    const batchSize = 1000;
    let processed = 0;

    while (true) {
      const result = await db.execute(`
        UPDATE stripe_customers
        SET last_activity_at = updated_at
        WHERE id IN (
          SELECT id FROM stripe_customers
          WHERE last_activity_at IS NULL
          ORDER BY id
          LIMIT $1
        )
      `, [batchSize]);

      if (result === 0) break;

      processed += result;
      logger.info(`Backfilled ${processed} rows`);

      offset += batchSize;
      // Small delay to reduce load
      await new Promise(resolve => setTimeout(resolve, 100));
    }

    // Step 4: Verify migration
    const { rows } = await db.query<{ count: number }>(`
      SELECT COUNT(*) as count
      FROM stripe_customers
      WHERE last_activity_at IS NULL
    `);

    if (rows[0].count > 0) {
      throw new Error(`Migration incomplete: ${rows[0].count} rows not backfilled`);
    }

    // Step 5: Record migration
    await db.execute(`
      INSERT INTO schema_migrations (version, description, applied_at)
      VALUES ($1, $2, NOW())
    `, [metadata.version, metadata.description]);

    logger.success('Migration 001 completed successfully');
  } catch (error) {
    logger.error('Migration 001 failed', error);
    throw error;
  }
}

export async function down(db: Database): Promise<void> {
  logger.info('Rolling back migration 001...');

  try {
    // Drop indexes
    await db.execute(`
      DROP INDEX IF EXISTS idx_customers_marketing;
    `);

    // Remove columns
    await db.execute(`
      ALTER TABLE stripe_customers
      DROP COLUMN IF EXISTS preferences,
      DROP COLUMN IF EXISTS marketing_opt_in,
      DROP COLUMN IF EXISTS last_activity_at;
    `);

    // Remove migration record
    await db.execute(`
      DELETE FROM schema_migrations
      WHERE version = $1
    `, [metadata.version]);

    logger.success('Migration 001 rolled back successfully');
  } catch (error) {
    logger.error('Rollback 001 failed', error);
    throw error;
  }
}

// CLI integration
if (require.main === module) {
  const db = new Database();

  const command = process.argv[2];

  (async () => {
    await db.connect();

    try {
      if (command === 'up') {
        await up(db);
      } else if (command === 'down') {
        await down(db);
      } else {
        console.error('Usage: node 001_add_feature.js [up|down]');
        process.exit(1);
      }
    } finally {
      await db.disconnect();
    }
  })().catch(error => {
    console.error(error);
    process.exit(1);
  });
}
```

### Migration Runner

```typescript
// migrations/runner.ts

import { Database } from '@nself/plugin-utils';
import { createLogger } from '@nself/plugin-utils';
import { promises as fs } from 'fs';
import path from 'path';

const logger = createLogger('migration-runner');

interface Migration {
  version: string;
  description: string;
  up: (db: Database) => Promise<void>;
  down: (db: Database) => Promise<void>;
}

export class MigrationRunner {
  private db: Database;
  private migrationsDir: string;

  constructor(db: Database, migrationsDir: string) {
    this.db = db;
    this.migrationsDir = migrationsDir;
  }

  async initialize(): Promise<void> {
    // Create migrations tracking table
    await this.db.execute(`
      CREATE TABLE IF NOT EXISTS schema_migrations (
        version VARCHAR(50) PRIMARY KEY,
        description TEXT,
        applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
        rolled_back_at TIMESTAMP WITH TIME ZONE
      );
    `);
  }

  async getAppliedMigrations(): Promise<Set<string>> {
    const { rows } = await this.db.query<{ version: string }>(`
      SELECT version FROM schema_migrations
      WHERE rolled_back_at IS NULL
      ORDER BY version
    `);

    return new Set(rows.map(r => r.version));
  }

  async getPendingMigrations(): Promise<Migration[]> {
    const applied = await this.getAppliedMigrations();
    const files = await fs.readdir(this.migrationsDir);

    const migrations: Migration[] = [];

    for (const file of files.sort()) {
      if (!file.endsWith('.ts') && !file.endsWith('.js')) continue;

      const version = file.split('_')[0];
      if (applied.has(version)) continue;

      const migrationPath = path.join(this.migrationsDir, file);
      const migration = await import(migrationPath);

      migrations.push({
        version: migration.metadata.version,
        description: migration.metadata.description,
        up: migration.up,
        down: migration.down,
      });
    }

    return migrations;
  }

  async runPending(): Promise<void> {
    const pending = await this.getPendingMigrations();

    if (pending.length === 0) {
      logger.info('No pending migrations');
      return;
    }

    logger.info(`Found ${pending.length} pending migrations`);

    for (const migration of pending) {
      logger.info(`Running migration ${migration.version}: ${migration.description}`);

      try {
        await migration.up(this.db);
        logger.success(`Migration ${migration.version} completed`);
      } catch (error) {
        logger.error(`Migration ${migration.version} failed`, error);
        throw error;
      }
    }

    logger.success('All migrations completed');
  }

  async rollback(version?: string): Promise<void> {
    const applied = await this.getAppliedMigrations();

    if (applied.size === 0) {
      logger.info('No migrations to rollback');
      return;
    }

    // Rollback specific version or last applied
    const targetVersion = version || Array.from(applied).sort().pop()!;

    if (!applied.has(targetVersion)) {
      throw new Error(`Migration ${targetVersion} not applied`);
    }

    // Load migration file
    const files = await fs.readdir(this.migrationsDir);
    const file = files.find(f => f.startsWith(targetVersion));

    if (!file) {
      throw new Error(`Migration file for ${targetVersion} not found`);
    }

    const migrationPath = path.join(this.migrationsDir, file);
    const migration = await import(migrationPath);

    logger.info(`Rolling back migration ${targetVersion}`);

    try {
      await migration.down(this.db);
      logger.success(`Migration ${targetVersion} rolled back`);
    } catch (error) {
      logger.error(`Rollback ${targetVersion} failed`, error);
      throw error;
    }
  }

  async status(): Promise<void> {
    const applied = await this.getAppliedMigrations();
    const pending = await this.getPendingMigrations();

    console.log('\n📊 Migration Status\n');
    console.log(`Applied: ${applied.size}`);
    console.log(`Pending: ${pending.length}`);

    if (applied.size > 0) {
      console.log('\nApplied migrations:');
      for (const version of Array.from(applied).sort()) {
        console.log(`  ✓ ${version}`);
      }
    }

    if (pending.length > 0) {
      console.log('\nPending migrations:');
      for (const migration of pending) {
        console.log(`  ○ ${migration.version} - ${migration.description}`);
      }
    }

    console.log('');
  }
}

// CLI
if (require.main === module) {
  const db = new Database();
  const runner = new MigrationRunner(db, __dirname);

  const command = process.argv[2];

  (async () => {
    await db.connect();
    await runner.initialize();

    try {
      switch (command) {
        case 'status':
          await runner.status();
          break;

        case 'up':
        case 'migrate':
          await runner.runPending();
          break;

        case 'down':
        case 'rollback':
          const version = process.argv[3];
          await runner.rollback(version);
          break;

        default:
          console.error('Usage: node runner.ts [status|migrate|rollback [version]]');
          process.exit(1);
      }
    } finally {
      await db.disconnect();
    }
  })().catch(error => {
    console.error(error);
    process.exit(1);
  });
}
```

---

## Testing Migrations

### Pre-Migration Testing Checklist

Before running migrations in production:

- [ ] **Test on development database** - Verify migration works
- [ ] **Test on staging database** - Test with production-like data
- [ ] **Performance test** - Measure migration duration
- [ ] **Rollback test** - Verify rollback works correctly
- [ ] **Data integrity test** - Check data after migration
- [ ] **Application compatibility test** - Ensure app works with new schema
- [ ] **Load test** - Verify performance under load
- [ ] **Backup created** - Full backup before migration

### Automated Migration Tests

```typescript
// tests/migrations/001_test.ts

import { describe, it, beforeEach, afterEach } from 'node:test';
import assert from 'node:assert';
import { Database } from '@nself/plugin-utils';
import { up, down } from '../../migrations/001_add_feature.js';

describe('Migration 001: Add customer preferences', () => {
  let db: Database;

  beforeEach(async () => {
    db = new Database(process.env.TEST_DATABASE_URL);
    await db.connect();

    // Create test data
    await db.execute(`
      INSERT INTO stripe_customers (id, email, name, created_at, updated_at)
      VALUES ('cus_test1', 'test@example.com', 'Test User', NOW(), NOW())
    `);
  });

  afterEach(async () => {
    // Clean up
    await db.execute('DROP TABLE IF EXISTS stripe_customers CASCADE');
    await db.disconnect();
  });

  it('should add preference columns', async () => {
    await up(db);

    const { rows } = await db.query(`
      SELECT column_name
      FROM information_schema.columns
      WHERE table_name = 'stripe_customers'
        AND column_name IN ('preferences', 'marketing_opt_in', 'last_activity_at')
    `);

    assert.equal(rows.length, 3, 'All columns should be added');
  });

  it('should backfill last_activity_at', async () => {
    await up(db);

    const { rows } = await db.query<{ last_activity_at: Date }>(`
      SELECT last_activity_at
      FROM stripe_customers
      WHERE id = 'cus_test1'
    `);

    assert.ok(rows[0].last_activity_at, 'last_activity_at should be backfilled');
  });

  it('should create marketing index', async () => {
    await up(db);

    const { rows } = await db.query(`
      SELECT indexname
      FROM pg_indexes
      WHERE tablename = 'stripe_customers'
        AND indexname = 'idx_customers_marketing'
    `);

    assert.equal(rows.length, 1, 'Marketing index should be created');
  });

  it('should rollback cleanly', async () => {
    await up(db);
    await down(db);

    const { rows } = await db.query(`
      SELECT column_name
      FROM information_schema.columns
      WHERE table_name = 'stripe_customers'
        AND column_name IN ('preferences', 'marketing_opt_in', 'last_activity_at')
    `);

    assert.equal(rows.length, 0, 'All columns should be removed');
  });

  it('should preserve existing data during migration', async () => {
    await up(db);

    const { rows } = await db.query<{ email: string; name: string }>(`
      SELECT email, name
      FROM stripe_customers
      WHERE id = 'cus_test1'
    `);

    assert.equal(rows[0].email, 'test@example.com');
    assert.equal(rows[0].name, 'Test User');
  });

  it('should handle large datasets efficiently', async () => {
    // Insert 10k test records
    for (let i = 0; i < 10000; i++) {
      await db.execute(`
        INSERT INTO stripe_customers (id, email, name, created_at, updated_at)
        VALUES ($1, $2, $3, NOW(), NOW())
      `, [`cus_test${i}`, `test${i}@example.com`, `Test User ${i}`]);
    }

    const startTime = Date.now();
    await up(db);
    const duration = Date.now() - startTime;

    // Should complete in reasonable time (< 30 seconds for 10k rows)
    assert.ok(duration < 30000, `Migration took ${duration}ms`);

    // Verify all rows migrated
    const { rows } = await db.query<{ count: number }>(`
      SELECT COUNT(*) as count
      FROM stripe_customers
      WHERE last_activity_at IS NOT NULL
    `);

    assert.equal(rows[0].count, 10001); // 10000 + 1 from beforeEach
  });
});

// Run tests
import { run } from 'node:test';
run({ files: [__filename] });
```

### Integration Testing

```bash
#!/bin/bash
# test_migration_integration.sh
# Full integration test for migrations

set -e

TEST_DB="test_migration_$(date +%s)"
TEST_DB_URL="postgresql://localhost:5432/$TEST_DB"

echo "🧪 Starting migration integration test..."

# Step 1: Create test database
echo "1. Creating test database..."
createdb "$TEST_DB"

# Step 2: Initialize schema
echo "2. Initializing schema..."
psql "$TEST_DB_URL" < plugins/stripe/ts/src/schema.sql

# Step 3: Seed test data
echo "3. Seeding test data..."
psql "$TEST_DB_URL" <<EOF
INSERT INTO stripe_customers (id, email, name, created_at, updated_at)
SELECT
  'cus_test' || generate_series,
  'test' || generate_series || '@example.com',
  'Test User ' || generate_series,
  NOW() - (generate_series || ' days')::INTERVAL,
  NOW()
FROM generate_series(1, 1000);
EOF

# Step 4: Run migration
echo "4. Running migration..."
DATABASE_URL="$TEST_DB_URL" node plugins/stripe/migrations/runner.ts migrate

# Step 5: Verify data integrity
echo "5. Verifying data integrity..."
CUSTOMER_COUNT=$(psql "$TEST_DB_URL" -t -c "SELECT COUNT(*) FROM stripe_customers")
if [ "$CUSTOMER_COUNT" -ne 1000 ]; then
  echo "❌ Data integrity check failed: Expected 1000 customers, got $CUSTOMER_COUNT"
  exit 1
fi

# Step 6: Run rollback
echo "6. Testing rollback..."
DATABASE_URL="$TEST_DB_URL" node plugins/stripe/migrations/runner.ts rollback

# Step 7: Verify rollback
echo "7. Verifying rollback..."
COLUMN_COUNT=$(psql "$TEST_DB_URL" -t -c "
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_name = 'stripe_customers'
    AND column_name = 'preferences'
")
if [ "$COLUMN_COUNT" -ne 0 ]; then
  echo "❌ Rollback verification failed"
  exit 1
fi

# Step 8: Cleanup
echo "8. Cleaning up..."
dropdb "$TEST_DB"

echo "✅ All migration integration tests passed!"
```

### Load Testing Migrations

```typescript
// tests/migrations/load_test.ts
// Test migration performance under load

import { Database } from '@nself/plugin-utils';
import { Worker } from 'worker_threads';

async function loadTestMigration() {
  const db = new Database(process.env.LOAD_TEST_DATABASE_URL);
  await db.connect();

  // Create large dataset
  console.log('Creating test dataset (1M rows)...');
  await db.execute(`
    INSERT INTO stripe_customers (id, email, name, created_at, updated_at)
    SELECT
      'cus_' || generate_series,
      'user' || generate_series || '@example.com',
      'User ' || generate_series,
      NOW() - (random() * 365 || ' days')::INTERVAL,
      NOW()
    FROM generate_series(1, 1000000);
  `);

  // Simulate concurrent reads during migration
  const workers: Worker[] = [];

  for (let i = 0; i < 10; i++) {
    workers.push(new Worker('./read_worker.js', {
      workerData: { databaseUrl: process.env.LOAD_TEST_DATABASE_URL }
    }));
  }

  // Run migration
  console.log('Running migration with concurrent load...');
  const startTime = Date.now();

  await db.execute(`
    ALTER TABLE stripe_customers
    ADD COLUMN preferences JSONB DEFAULT '{}';
  `);

  const duration = Date.now() - startTime;

  // Stop workers
  workers.forEach(w => w.terminate());

  console.log(`Migration completed in ${duration}ms with concurrent load`);

  await db.disconnect();
}

loadTestMigration().catch(console.error);
```

---

## Summary

This migration guide covers:

1. ✅ **Database schema evolution** with additive migrations
2. ✅ **Plugin version updates** with semantic versioning
3. ✅ **Breaking change management** with clear upgrade paths
4. ✅ **Comprehensive backup strategies** for safety
5. ✅ **Automated rollback procedures** for quick recovery
6. ✅ **Zero-downtime migrations** with blue-green deployments
7. ✅ **Import tools** for migrating from other systems
8. ✅ **Version compatibility matrix** for all plugins
9. ✅ **Production-ready migration scripts** with examples
10. ✅ **Thorough testing approaches** for confidence

### Quick Reference Commands

```bash
# Check for updates
nself plugin updates

# Update plugin
nself plugin update <name>

# Backup before migration
./scripts/backup_plugin_data.sh <name>

# Run migrations
npm run migrate

# Rollback migration
npm run migrate:rollback

# Test migration
npm run migrate:test

# Check migration status
npm run migrate:status
```

### Need Help?

- **Documentation**: https://github.com/acamarata/nself-plugins/wiki
- **Issues**: https://github.com/acamarata/nself-plugins/issues
- **Discord**: https://discord.gg/nself

---

**Last Updated**: January 30, 2026
**Maintainer**: nself team
