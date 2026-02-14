# Database Password Parsing Fix

**Date**: 2026-02-14
**Git Commit**: `ed9a8e0`
**Status**: ✅ RESOLVED

---

## Problem Summary

Multiple plugins (jobs, devices, recording) were failing with this error:
```
SASL: SCRAM-SERVER-FIRST-MESSAGE: client password must be a string
```

This occurred when using `DATABASE_URL` instead of individual `POSTGRES_*` environment variables.

---

## Root Cause Analysis

### The Bug

The `createDatabase()` function in `shared/src/database.ts` only checked individual `POSTGRES_*` environment variables, ignoring `DATABASE_URL`.

**Before (broken code):**
```typescript
export function createDatabase(config?: Partial<DatabaseConfig>): Database {
  const fullConfig: DatabaseConfig = {
    host: config?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    port: config?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    database: config?.database ?? process.env.POSTGRES_DB ?? 'nself',
    user: config?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    password: config?.password ?? process.env.POSTGRES_PASSWORD ?? '', // ❌ BUG HERE
    ssl: config?.ssl ?? process.env.POSTGRES_SSL === 'true',
    maxConnections: config?.maxConnections ?? parseInt(process.env.POSTGRES_MAX_CONNECTIONS ?? '10', 10),
  };

  return new Database(fullConfig);
}
```

### What Went Wrong

1. **nTV configuration** uses `DATABASE_URL`:
   ```env
   DATABASE_URL=postgresql://postgres:dev_password_change_in_prod@localhost:5432/nself_tv_db
   ENCRYPTION_KEY=Hbc/oQRhuv1k2BgrDiSNwrMWuQP60rEexSfXUg6tfi8=
   PORT=3603
   LOG_LEVEL=info
   ```

2. **No individual POSTGRES_* variables** were set, so:
   - `process.env.POSTGRES_PASSWORD` = `undefined`
   - Falls back to empty string: `''`

3. **PostgreSQL SCRAM authentication** requires a non-empty string password:
   - Empty string `''` is invalid
   - Results in: "client password must be a string" error

### Why Some Plugins Worked

Individual plugins like `jobs`, `devices`, and `recording` have their own `parseDatabaseUrl()` functions in their `config.ts` files. However, they also use the shared `createDatabase()` utility, which didn't parse DATABASE_URL, causing the failure.

---

## The Fix

### Added DATABASE_URL Parsing

**After (fixed code):**
```typescript
/**
 * Parse DATABASE_URL into connection parameters
 */
function parseDatabaseUrl(url: string | undefined): {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
  ssl: boolean;
} | null {
  if (!url) {
    return null;
  }

  try {
    const match = url.match(/^postgres(?:ql)?:\/\/([^:]+):([^@]+)@([^:]+):(\d+)\/([^?]+)(\?.*)?$/);
    if (!match) {
      return null;
    }

    const [, user, password, host, port, database, queryString] = match;
    const ssl = queryString?.includes('sslmode=require') || queryString?.includes('ssl=true') || false;

    return {
      host,
      port: parseInt(port, 10),
      database,
      user,
      password, // ✅ Extracted from URL
      ssl,
    };
  } catch {
    return null;
  }
}

export function createDatabase(config?: Partial<DatabaseConfig>): Database {
  // Try to parse DATABASE_URL first, fall back to individual POSTGRES_* vars
  const dbFromUrl = parseDatabaseUrl(process.env.DATABASE_URL);

  const fullConfig: DatabaseConfig = {
    host: config?.host ?? dbFromUrl?.host ?? process.env.POSTGRES_HOST ?? 'localhost',
    port: config?.port ?? dbFromUrl?.port ?? parseInt(process.env.POSTGRES_PORT ?? '5432', 10),
    database: config?.database ?? dbFromUrl?.database ?? process.env.POSTGRES_DB ?? 'nself',
    user: config?.user ?? dbFromUrl?.user ?? process.env.POSTGRES_USER ?? 'postgres',
    password: config?.password ?? dbFromUrl?.password ?? process.env.POSTGRES_PASSWORD ?? '', // ✅ Now checks DATABASE_URL
    ssl: config?.ssl ?? dbFromUrl?.ssl ?? process.env.POSTGRES_SSL === 'true',
    maxConnections: config?.maxConnections ?? parseInt(process.env.POSTGRES_MAX_CONNECTIONS ?? '10', 10),
  };

  // Validate that we have a password (empty string will cause SCRAM auth errors)
  if (!fullConfig.password) {
    const source = config?.password ? 'config' :
                   dbFromUrl?.password ? 'DATABASE_URL' :
                   process.env.POSTGRES_PASSWORD ? 'POSTGRES_PASSWORD' : 'none';
    logger.error('Database password is empty or undefined', {
      source,
      hasDatabaseUrl: !!process.env.DATABASE_URL,
      hasPostgresPassword: !!process.env.POSTGRES_PASSWORD,
    });
  }

  return new Database(fullConfig);
}
```

### Configuration Priority Order

The fix establishes a clear priority order (highest to lowest):

1. **Explicit config object** (passed to `createDatabase(config)`)
2. **DATABASE_URL** environment variable
3. **Individual POSTGRES_*** environment variables
4. **Default values**

---

## Testing Results

### Test 1: DATABASE_URL Only ✅
```env
DATABASE_URL=postgresql://postgres:dev_password_change_in_prod@localhost:5432/nself_tv_db
```

**Result**: Connection successful, password correctly extracted from URL.

### Test 2: POSTGRES_* Variables Only ✅
```env
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself_tv_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=dev_password_change_in_prod
```

**Result**: Connection successful, maintains backward compatibility.

### Test 3: Both Present (DATABASE_URL Priority) ✅
```env
DATABASE_URL=postgresql://postgres:dev_password_change_in_prod@localhost:5432/nself_tv_db
POSTGRES_HOST=wrong_host
POSTGRES_PORT=9999
POSTGRES_DB=wrong_db
POSTGRES_USER=wrong_user
POSTGRES_PASSWORD=wrong_password
```

**Result**: DATABASE_URL values used correctly, POSTGRES_* values ignored.

---

## Impact

### Affected Plugins

All plugins using `createDatabase()` from `@nself/plugin-utils`:

- ✅ devices
- ✅ jobs
- ✅ recording
- ✅ content-progress
- ✅ workflows
- ✅ tokens
- ✅ torrent-manager
- ✅ metadata-enrichment
- ✅ retro-gaming
- ✅ rom-discovery
- And any future plugins using the shared utility

### What's Fixed

1. Database connections now work with `DATABASE_URL` configuration
2. Password is correctly extracted from connection strings
3. Special characters in passwords (underscores, etc.) work correctly
4. SCRAM authentication succeeds
5. Clear error logging if password is missing

### Backward Compatibility

✅ **Fully maintained** - Existing plugins using `POSTGRES_*` variables continue to work unchanged.

---

## Deployment Instructions for nTV Team

### 1. Update Shared Package

```bash
cd ~/Sites/nself-plugins/shared
pnpm run build
```

### 2. Update Affected Plugins

For each plugin using the shared database utility:

```bash
cd ~/Sites/nself-plugins/plugins/devices/ts
pnpm install  # Picks up updated @nself/plugin-utils
pnpm run build

cd ~/Sites/nself-plugins/plugins/jobs/ts
pnpm install
pnpm run build

cd ~/Sites/nself-plugins/plugins/recording/ts
pnpm install
pnpm run build

# Repeat for other affected plugins
```

### 3. Restart Plugins

```bash
# Stop all affected plugins
pm2 stop devices jobs recording

# Start them again
pm2 start devices jobs recording

# Or use nself commands if available
nself plugin restart devices
nself plugin restart jobs
nself plugin restart recording
```

### 4. Verify

```bash
# Check plugin logs
pm2 logs devices --lines 50
pm2 logs jobs --lines 50
pm2 logs recording --lines 50

# Should see:
# "Database connected {"host":"localhost","database":"nself_tv_db"}"
# NOT: "SCRAM-SERVER-FIRST-MESSAGE" errors
```

---

## Configuration Examples

### Recommended: DATABASE_URL

```env
# Single line, all connection info
DATABASE_URL=postgresql://postgres:your_password@localhost:5432/nself_tv_db

# Optional: Add SSL
DATABASE_URL=postgresql://postgres:your_password@localhost:5432/nself_tv_db?ssl=true
```

### Alternative: Individual Variables

```env
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=nself_tv_db
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your_password
POSTGRES_SSL=true
```

### Both (DATABASE_URL Takes Priority)

```env
# This will be used
DATABASE_URL=postgresql://postgres:primary_password@localhost:5432/nself_tv_db

# These will be ignored (DATABASE_URL takes priority)
POSTGRES_HOST=backup_host
POSTGRES_PASSWORD=backup_password
```

---

## Technical Details

### SCRAM Authentication

PostgreSQL's SCRAM-SHA-256 authentication (default in PostgreSQL 10+) requires:

1. Password must be a **string type** (not undefined, null, or other types)
2. Password can be empty `''` for local dev, but **not recommended**
3. For remote connections, password **must not be empty**

### Regex Pattern

The DATABASE_URL parser uses this pattern:
```regex
/^postgres(?:ql)?:\/\/([^:]+):([^@]+)@([^:]+):(\d+)\/([^?]+)(\?.*)?$/
```

**Matches**:
- `postgresql://user:password@host:port/database`
- `postgres://user:password@host:port/database`
- Optional query string: `?ssl=true&sslmode=require`

**Captures**:
1. `user` - Username
2. `password` - Password (can include special characters except `@`)
3. `host` - Hostname or IP
4. `port` - Port number
5. `database` - Database name
6. `queryString` - Optional query parameters

---

## Debugging

If you encounter password issues in the future:

### Check Environment

```bash
# In plugin directory
node -e "require('dotenv').config(); console.log({
  DATABASE_URL: process.env.DATABASE_URL,
  POSTGRES_PASSWORD: process.env.POSTGRES_PASSWORD
})"
```

### Enable Debug Logging

```env
LOG_LEVEL=debug
```

The shared database module now logs when password is empty:
```
ERROR [database] Database password is empty or undefined {
  source: 'none',
  hasDatabaseUrl: false,
  hasPostgresPassword: false
}
```

### Verify Connection

```bash
# Direct PostgreSQL connection test
docker exec nself-tv_postgres psql -U postgres -d nself_tv_db -c "SELECT 1"

# Or from host
psql "postgresql://postgres:your_password@localhost:5432/nself_tv_db" -c "SELECT 1"
```

---

## Lessons Learned

1. **Always support multiple configuration methods** - Some users prefer DATABASE_URL, others prefer individual variables
2. **Parse DATABASE_URL early** - Many tools and platforms (Heroku, Railway, Render) use DATABASE_URL as the standard
3. **Validate credentials before use** - Catch empty passwords early with clear error messages
4. **Maintain backward compatibility** - Don't break existing configurations when adding new features
5. **Test all configuration scenarios** - URL-only, vars-only, both present, neither present

---

## Questions?

If you encounter any issues with this fix:

1. Check the commit: `git show ed9a8e0`
2. Review shared package build: `cd shared && pnpm run build`
3. Verify environment variables are loaded
4. Check plugin logs for password validation errors
5. Contact the nself-plugins team with specific error messages

---

**Status**: ✅ Fix deployed and tested
**Commit**: `ed9a8e0`
**Files Changed**: `shared/src/database.ts`
**Lines Changed**: +36 additions, -6 deletions
