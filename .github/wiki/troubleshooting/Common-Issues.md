# Common Issues and Solutions

This guide covers the most frequently encountered issues when working with nself plugins, along with detailed solutions.

---

## Table of Contents

1. [Database Connection Issues](#database-connection-issues)
2. [TypeScript Build Errors](#typescript-build-errors)
3. [Webhook Signature Failures](#webhook-signature-failures)
4. [Port Conflicts](#port-conflicts)
5. [Permission Errors](#permission-errors)
6. [npm Install Failures](#npm-install-failures)
7. [API Rate Limiting](#api-rate-limiting)
8. [Data Sync Issues](#data-sync-issues)
9. [Environment Configuration](#environment-configuration)
10. [Runtime Errors](#runtime-errors)

---

## Database Connection Issues

### Issue: "Connection refused" Error

**Symptoms:**
```
Error: connect ECONNREFUSED 127.0.0.1:5432
    at TCPConnectWrap.afterConnect [as oncomplete] (node:net:1495:16)
```

**Causes:**
1. PostgreSQL is not running
2. PostgreSQL is running on a different port
3. Firewall blocking connections
4. Wrong DATABASE_URL configuration

**Solutions:**

#### 1. Verify PostgreSQL is Running
```bash
# macOS
brew services list | grep postgresql

# Linux
sudo systemctl status postgresql

# Check if port 5432 is open
lsof -i :5432
```

#### 2. Start PostgreSQL
```bash
# macOS
brew services start postgresql@14

# Linux
sudo systemctl start postgresql

# Docker
docker start postgres
# or
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:14
```

#### 3. Verify Connection String
```bash
# Check your DATABASE_URL format
# Correct format:
# postgresql://username:password@hostname:port/database

# Test connection with psql
psql "postgresql://postgres:postgres@localhost:5432/nself"

# If connection works, your DATABASE_URL should be:
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/nself
```

#### 4. Check PostgreSQL Configuration
```bash
# Find PostgreSQL config
psql -c "SHOW config_file"

# Verify listen_addresses
grep listen_addresses /path/to/postgresql.conf

# Should be:
# listen_addresses = 'localhost'  # or '*' for all interfaces
```

---

### Issue: "Database does not exist"

**Symptoms:**
```
Error: database "nself" does not exist
```

**Solution:**
```bash
# Create the database
createdb nself

# Or with psql
psql -c "CREATE DATABASE nself;"

# Verify it exists
psql -l | grep nself

# Run plugin init to create tables
cd plugins/stripe/ts
npm start init
```

---

### Issue: "Password authentication failed"

**Symptoms:**
```
Error: password authentication failed for user "postgres"
```

**Solutions:**

#### 1. Reset PostgreSQL Password
```bash
# macOS - login without password
psql postgres

# Change password
ALTER USER postgres PASSWORD 'newpassword';

# Update your .env file
DATABASE_URL=postgresql://postgres:newpassword@localhost:5432/nself
```

#### 2. Check pg_hba.conf
```bash
# Find pg_hba.conf location
psql -c "SHOW hba_file"

# Edit the file (macOS example)
sudo nano /opt/homebrew/var/postgresql@14/pg_hba.conf

# Change authentication method to md5 or trust:
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             all                                     trust
host    all             all             127.0.0.1/32            md5

# Reload PostgreSQL
brew services restart postgresql@14
```

---

### Issue: "Too many connections"

**Symptoms:**
```
Error: sorry, too many clients already
```

**Solutions:**

#### 1. Check Current Connections
```sql
SELECT count(*) FROM pg_stat_activity;
SELECT max_connections FROM pg_settings;
```

#### 2. Kill Idle Connections
```sql
-- Find idle connections
SELECT pid, usename, application_name, state, state_change
FROM pg_stat_activity
WHERE state = 'idle'
AND state_change < NOW() - INTERVAL '5 minutes';

-- Kill specific connection
SELECT pg_terminate_backend(pid);

-- Kill all idle connections
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE state = 'idle'
AND pid <> pg_backend_pid();
```

#### 3. Increase Connection Limit
```bash
# Edit postgresql.conf
nano /path/to/postgresql.conf

# Increase max_connections
max_connections = 100

# Restart PostgreSQL
brew services restart postgresql@14
```

#### 4. Fix Plugin Connection Pool
```typescript
// Check your database.ts
const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
  max: 10, // Don't set this too high
});

// Always close connections
try {
  const client = await pool.connect();
  // ... use client
  client.release(); // Important!
} catch (error) {
  // ...
}
```

---

## TypeScript Build Errors

### Issue: "Cannot find module './types.js'"

**Symptoms:**
```
error TS2307: Cannot find module './types.js' or its corresponding type declarations.
```

**Cause:**
Using `.ts` extension in imports instead of `.js` with NodeNext module resolution.

**Solution:**
```typescript
// WRONG
import { Customer } from './types';
import { Customer } from './types.ts';

// CORRECT
import { Customer } from './types.js';
```

**Why .js?** TypeScript with NodeNext resolution requires `.js` extensions in import statements, even though the source files are `.ts`. The compiler maps them correctly.

---

### Issue: "Module not found: @nself/plugin-utils"

**Symptoms:**
```
error TS2307: Cannot find module '@nself/plugin-utils'
```

**Cause:**
The shared package is not built or not linked properly.

**Solution:**
```bash
# 1. Build the shared package first
cd /Users/admin/Sites/nself-plugins/shared
npm install
npm run build

# 2. Rebuild the plugin
cd /Users/admin/Sites/nself-plugins/plugins/stripe/ts
rm -rf node_modules package-lock.json
npm install
npm run build

# 3. If still failing, check package.json
cat package.json | grep "@nself/plugin-utils"

# Should show:
# "@nself/plugin-utils": "file:../../../shared"
```

---

### Issue: "Type errors in dist/ files"

**Symptoms:**
```
dist/client.js(45,12): error TS2339: Property 'foo' does not exist on type 'Bar'
```

**Cause:**
Stale build artifacts from previous versions.

**Solution:**
```bash
# Clean build
rm -rf dist/
npm run build

# If using watch mode, restart it
npm run watch

# Nuclear option - clean everything
rm -rf dist/ node_modules/ package-lock.json
npm install
npm run build
```

---

### Issue: "ESM vs CommonJS conflicts"

**Symptoms:**
```
Error [ERR_REQUIRE_ESM]: require() of ES Module not supported
```

**Cause:**
Mixing ES modules with CommonJS.

**Solution:**
```json
// Verify package.json has:
{
  "type": "module",
  "exports": {
    ".": {
      "types": "./dist/index.d.ts",
      "import": "./dist/index.js"
    }
  }
}
```

```typescript
// Use ES module imports only
import { something } from 'package';

// Not CommonJS
const something = require('package'); // WRONG
```

---

## Webhook Signature Failures

### Issue: "Invalid signature" for Stripe Webhooks

**Symptoms:**
```
Error: No signatures found matching the expected signature for payload
```

**Causes:**
1. Wrong webhook secret
2. Body parser modifying request body
3. Clock skew between servers
4. Testing with wrong secret

**Solutions:**

#### 1. Verify Webhook Secret
```bash
# Check your .env file
cat .env | grep STRIPE_WEBHOOK_SECRET

# Should start with whsec_
STRIPE_WEBHOOK_SECRET=whsec_xxxxxxxxxxxxx

# Get the correct secret from Stripe Dashboard:
# https://dashboard.stripe.com/webhooks
```

#### 2. Check Body Parser Configuration
```typescript
// In server.ts, ensure raw body is available
fastify.addContentTypeParser(
  'application/json',
  { parseAs: 'buffer' },
  (req, body, done) => {
    done(null, body);
  }
);

// Then in webhook handler
const rawBody = request.body.toString('utf8');
const signature = request.headers['stripe-signature'];

// Verify with raw body, not parsed JSON
stripe.webhooks.constructEvent(rawBody, signature, webhookSecret);
```

#### 3. Test with Stripe CLI
```bash
# Install Stripe CLI
brew install stripe/stripe-cli/stripe

# Login
stripe login

# Forward webhooks to local server
stripe listen --forward-to http://localhost:3001/webhook

# Use the webhook secret shown by CLI
# Ready! Your webhook signing secret is whsec_xxxxx
# Copy this to your .env as STRIPE_WEBHOOK_SECRET

# Test with event
stripe trigger customer.created
```

#### 4. Clock Skew Issues
```bash
# Check system time
date

# Sync time (macOS)
sudo sntp -sS time.apple.com

# Sync time (Linux)
sudo ntpdate -s time.nist.gov
```

---

### Issue: "Invalid signature" for GitHub Webhooks

**Symptoms:**
```
Error: Signature verification failed
```

**Solutions:**

#### 1. Verify Secret Matches
```bash
# In GitHub: Settings > Webhooks > Edit > Secret
# Copy the exact secret to .env
GITHUB_WEBHOOK_SECRET=your_secret_here
```

#### 2. Check Signature Header
```typescript
// GitHub uses X-Hub-Signature-256
const signature = request.headers['x-hub-signature-256'];

// Format is: sha256=<hash>
if (!signature || !signature.startsWith('sha256=')) {
  throw new Error('Invalid signature header');
}

// Extract hash
const hash = signature.substring(7);
```

#### 3. Use Raw Body
```typescript
// Verify with exact raw body
const crypto = require('crypto');
const hmac = crypto.createHmac('sha256', webhookSecret);
hmac.update(rawBody);
const expectedHash = hmac.digest('hex');

if (!crypto.timingSafeEqual(Buffer.from(hash), Buffer.from(expectedHash))) {
  throw new Error('Signature mismatch');
}
```

---

### Issue: "Invalid signature" for Shopify Webhooks

**Symptoms:**
```
Error: HMAC validation failed
```

**Solutions:**

#### 1. Verify API Secret Key
```bash
# Shopify uses your API Secret Key, not a separate webhook secret
# From Shopify Admin: Apps > Your App > API credentials > API secret key
SHOPIFY_API_SECRET=shpss_xxxxxxxxxxxxx
```

#### 2. Check Header Name
```typescript
// Shopify uses X-Shopify-Hmac-SHA256 (Base64 encoded)
const hmac = request.headers['x-shopify-hmac-sha256'];

// Verify
const crypto = require('crypto');
const hash = crypto
  .createHmac('sha256', apiSecret)
  .update(rawBody)
  .digest('base64');

if (hash !== hmac) {
  throw new Error('Invalid HMAC');
}
```

---

## Port Conflicts

### Issue: "Port already in use"

**Symptoms:**
```
Error: listen EADDRINUSE: address already in use :::3001
```

**Solutions:**

#### 1. Find Process Using Port
```bash
# macOS/Linux
lsof -i :3001

# Example output:
# COMMAND  PID   USER   FD   TYPE  DEVICE  NODE NAME
# node    1234  admin  23u  IPv6  0x...   0t0  TCP *:3001

# Kill the process
kill -9 1234

# Or kill all node processes on that port
lsof -ti :3001 | xargs kill -9
```

#### 2. Change Plugin Port
```bash
# In .env file
PORT=3011  # Use different port

# Or set inline
PORT=3011 npm start server
```

#### 3. Stop All Plugins
```bash
# Find all running plugin processes
ps aux | grep "node.*plugins"

# Kill all Node processes (nuclear option)
killall node

# Better: keep track of PIDs
npm start server &
echo $! > server.pid

# Stop later
kill $(cat server.pid)
```

#### 4. Use Port Manager Script
```bash
# Create scripts/port-manager.sh
#!/bin/bash

PLUGIN=$1
ACTION=$2

case $PLUGIN in
  stripe)  PORT=3001 ;;
  github)  PORT=3002 ;;
  shopify) PORT=3003 ;;
  *) echo "Unknown plugin"; exit 1 ;;
esac

case $ACTION in
  check)
    lsof -i :$PORT
    ;;
  kill)
    lsof -ti :$PORT | xargs kill -9
    ;;
  *)
    echo "Usage: $0 {stripe|github|shopify} {check|kill}"
    exit 1
    ;;
esac
```

---

## Permission Errors

### Issue: "EACCES: permission denied"

**Symptoms:**
```
Error: EACCES: permission denied, open '/path/to/file'
```

**Solutions:**

#### 1. File/Directory Permissions
```bash
# Check permissions
ls -la /path/to/file

# Fix file permissions
chmod 644 /path/to/file

# Fix directory permissions
chmod 755 /path/to/directory

# Fix ownership
sudo chown $USER:$USER /path/to/file
```

#### 2. npm Global Installs
```bash
# If getting permission errors with npm install -g
# Fix npm permissions
mkdir ~/.npm-global
npm config set prefix '~/.npm-global'

# Add to PATH in ~/.zshrc or ~/.bashrc
export PATH=~/.npm-global/bin:$PATH

# Reload shell
source ~/.zshrc
```

#### 3. PostgreSQL Permissions
```sql
-- Grant permissions to user
GRANT ALL PRIVILEGES ON DATABASE nself TO your_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO your_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO your_user;

-- Or create superuser
ALTER USER your_user WITH SUPERUSER;
```

#### 4. Log File Permissions
```bash
# If plugin can't write logs
mkdir -p ~/.nself/logs
chmod 755 ~/.nself/logs

# Or set custom log directory
export LOG_DIR=/tmp/nself-logs
mkdir -p $LOG_DIR
```

---

## npm Install Failures

### Issue: "Cannot find module after npm install"

**Symptoms:**
```
Error: Cannot find module 'fastify'
```

**Solutions:**

#### 1. Clean Install
```bash
# Remove everything and reinstall
rm -rf node_modules package-lock.json
npm install

# Verify installation
npm ls fastify
```

#### 2. Check package.json
```bash
# Ensure dependency is listed
cat package.json | grep fastify

# If missing, add it
npm install fastify --save
```

#### 3. Node Version Issues
```bash
# Check Node version
node --version

# Should be 18+ for nself plugins
# Use nvm to switch versions
nvm use 18
npm install
```

---

### Issue: "npm ERR! peer dependency" Warnings

**Symptoms:**
```
npm WARN ERESOLVE overriding peer dependency
```

**Solutions:**

#### 1. Update Dependencies
```bash
# Update all to latest compatible versions
npm update

# Or use --force to ignore peer deps (risky)
npm install --force
```

#### 2. Use Correct Versions
```json
// Check shared/package.json for correct versions
{
  "peerDependencies": {
    "pg": "^8.11.0"
  }
}

// Plugin should match
{
  "dependencies": {
    "pg": "^8.11.0"
  }
}
```

---

### Issue: "ERESOLVE unable to resolve dependency tree"

**Symptoms:**
```
npm ERR! ERESOLVE unable to resolve dependency tree
```

**Solutions:**

```bash
# Option 1: Use legacy peer deps
npm install --legacy-peer-deps

# Option 2: Force install
npm install --force

# Option 3: Use exact versions (recommended)
# Edit package.json to use exact versions
{
  "dependencies": {
    "fastify": "4.26.0"  // Remove ^
  }
}

npm install
```

---

## API Rate Limiting

### Issue: "429 Too Many Requests"

**Symptoms:**
```
Error: Request failed with status code 429
Rate limit exceeded
```

**Solutions:**

#### 1. Check Rate Limiter Configuration
```typescript
// In client.ts
const rateLimiter = new RateLimiter(100); // Too high!

// Stripe: 100 req/sec
const rateLimiter = new RateLimiter(100);

// GitHub: 5000 req/hour = ~83 req/min = ~1.4 req/sec
const rateLimiter = new RateLimiter(1);

// Shopify: 2 req/sec (standard tier)
const rateLimiter = new RateLimiter(2);
```

#### 2. Implement Exponential Backoff
```typescript
async function requestWithRetry<T>(
  fn: () => Promise<T>,
  maxRetries = 3
): Promise<T> {
  for (let i = 0; i < maxRetries; i++) {
    try {
      return await fn();
    } catch (error: any) {
      if (error.status === 429 && i < maxRetries - 1) {
        const waitTime = Math.pow(2, i) * 1000; // 1s, 2s, 4s
        logger.warn(`Rate limited, waiting ${waitTime}ms`);
        await new Promise(resolve => setTimeout(resolve, waitTime));
      } else {
        throw error;
      }
    }
  }
  throw new Error('Max retries exceeded');
}
```

#### 3. Use Incremental Sync
```bash
# Instead of full sync every time
npm start sync --full

# Use incremental sync (only recent changes)
npm start sync --since "2 hours ago"

# Or use webhooks for real-time updates
npm start server  # Keeps data current via webhooks
```

#### 4. Batch Requests
```typescript
// Instead of individual requests
for (const id of customerIds) {
  await client.getCustomer(id); // Many requests!
}

// Use list endpoints with filters
const customers = await client.listCustomers({
  ids: customerIds,
  limit: 100
});
```

---

## Data Sync Issues

### Issue: "Sync never completes"

**Symptoms:**
- Sync runs for hours without completing
- CPU usage high
- Memory increasing

**Solutions:**

#### 1. Check Sync Progress
```bash
# Enable debug logging
DEBUG=true npm start sync --full

# Monitor database size
psql -c "SELECT pg_size_pretty(pg_database_size('nself'));"

# Check table row counts
psql -c "SELECT schemaname, tablename, n_live_tup
FROM pg_stat_user_tables
ORDER BY n_live_tup DESC;"
```

#### 2. Implement Pagination Limits
```typescript
// In sync.ts
async syncCustomers(): Promise<void> {
  const batchSize = 100;
  let processed = 0;
  let hasMore = true;
  let cursor: string | undefined;

  while (hasMore) {
    const response = await this.client.listCustomers({
      limit: batchSize,
      starting_after: cursor
    });

    await this.database.upsertCustomers(response.data);
    processed += response.data.length;

    logger.info(`Synced ${processed} customers`);

    hasMore = response.has_more;
    cursor = response.next_cursor;

    // Add delay between batches
    await new Promise(resolve => setTimeout(resolve, 100));
  }
}
```

#### 3. Add Timeout Protection
```typescript
// Wrap sync with timeout
async function syncWithTimeout(
  syncFn: () => Promise<void>,
  timeoutMs: number = 3600000 // 1 hour
): Promise<void> {
  const timeout = new Promise<never>((_, reject) => {
    setTimeout(() => reject(new Error('Sync timeout')), timeoutMs);
  });

  await Promise.race([syncFn(), timeout]);
}
```

---

### Issue: "Duplicate key violations"

**Symptoms:**
```
Error: duplicate key value violates unique constraint
```

**Solution:**
```typescript
// Use ON CONFLICT in upserts
await db.execute(
  `INSERT INTO stripe_customers (id, email, name, synced_at)
   VALUES ($1, $2, $3, NOW())
   ON CONFLICT (id) DO UPDATE SET
     email = EXCLUDED.email,
     name = EXCLUDED.name,
     synced_at = NOW()`,
  [customer.id, customer.email, customer.name]
);

// Never use plain INSERT for sync data
// WRONG:
await db.execute(
  `INSERT INTO stripe_customers (id, email, name)
   VALUES ($1, $2, $3)`,
  [customer.id, customer.email, customer.name]
);
```

---

## Environment Configuration

### Issue: "Missing required environment variable"

**Symptoms:**
```
Error: STRIPE_API_KEY is required
```

**Solutions:**

#### 1. Create .env File
```bash
# Copy example
cp .env.example .env

# Edit with real values
nano .env

# Required variables for each plugin:

# Stripe
STRIPE_API_KEY=sk_test_xxxxx
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/nself

# GitHub
GITHUB_TOKEN=ghp_xxxxx
GITHUB_WEBHOOK_SECRET=your_secret
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/nself

# Shopify
SHOPIFY_SHOP_NAME=yourshop.myshopify.com
SHOPIFY_ACCESS_TOKEN=shpat_xxxxx
SHOPIFY_API_SECRET=shpss_xxxxx
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/nself
```

#### 2. Load .env in Development
```bash
# Ensure dotenv is installed
npm install dotenv

# Check config.ts loads it
head -5 src/config.ts
# Should see: import 'dotenv/config';
```

#### 3. Set in Production
```bash
# Export environment variables
export STRIPE_API_KEY=sk_live_xxxxx
export DATABASE_URL=postgresql://...

# Or use systemd service with EnvironmentFile
# /etc/systemd/system/nself-stripe.service
[Service]
EnvironmentFile=/etc/nself/.env
ExecStart=/usr/bin/node /opt/nself/plugins/stripe/ts/dist/index.js server
```

---

## Runtime Errors

### Issue: "Unhandled promise rejection"

**Symptoms:**
```
UnhandledPromiseRejectionWarning: Error: Something went wrong
```

**Solutions:**

#### 1. Add Global Error Handler
```typescript
// In index.ts or server.ts
process.on('unhandledRejection', (reason, promise) => {
  logger.error('Unhandled Rejection:', reason);
  // Don't exit in production - log and continue
});

process.on('uncaughtException', (error) => {
  logger.error('Uncaught Exception:', error);
  process.exit(1); // Exit on uncaught exceptions
});
```

#### 2. Wrap Async Functions
```typescript
// WRONG - unhandled rejection
async function handler(req, res) {
  const data = await fetchData(); // If this fails, unhandled!
  res.send(data);
}

// CORRECT - handled rejection
async function handler(req, res) {
  try {
    const data = await fetchData();
    res.send(data);
  } catch (error) {
    logger.error('Fetch failed:', error);
    res.status(500).send({ error: 'Internal server error' });
  }
}
```

---

### Issue: "Maximum call stack size exceeded"

**Symptoms:**
```
RangeError: Maximum call stack size exceeded
```

**Cause:**
Infinite recursion or very large arrays.

**Solutions:**

#### 1. Fix Recursive Calls
```typescript
// WRONG - infinite recursion
async function sync() {
  await syncData();
  await sync(); // Calls itself forever!
}

// CORRECT - add stopping condition
async function sync(depth = 0, maxDepth = 10) {
  if (depth >= maxDepth) return;
  await syncData();
  await sync(depth + 1, maxDepth);
}
```

#### 2. Chunk Large Arrays
```typescript
// WRONG - stack overflow with large array
const allIds = await getAllIds(); // 100,000 items
await Promise.all(allIds.map(id => process(id))); // Stack overflow!

// CORRECT - process in chunks
async function processInChunks<T>(
  items: T[],
  fn: (item: T) => Promise<void>,
  chunkSize = 100
): Promise<void> {
  for (let i = 0; i < items.length; i += chunkSize) {
    const chunk = items.slice(i, i + chunkSize);
    await Promise.all(chunk.map(fn));
  }
}

const allIds = await getAllIds();
await processInChunks(allIds, id => process(id));
```

---

## Quick Reference

### Common Commands

```bash
# Check if PostgreSQL is running
pg_isready

# Test database connection
psql $DATABASE_URL -c "SELECT 1"

# Check port usage
lsof -i :3001

# View plugin logs
tail -f ~/.nself/logs/stripe.log

# Test webhook signature
curl -X POST http://localhost:3001/webhook \
  -H "Content-Type: application/json" \
  -H "Stripe-Signature: t=..." \
  -d @webhook-payload.json

# Rebuild everything
rm -rf dist node_modules package-lock.json
npm install
npm run build

# Check for stale processes
ps aux | grep node
```

### Environment Variable Checklist

- [ ] DATABASE_URL set and valid
- [ ] API key/token set for service
- [ ] Webhook secret set (if using webhooks)
- [ ] PORT set (if default conflicts)
- [ ] LOG_LEVEL set for debugging
- [ ] NODE_ENV set (development/production)

### Pre-flight Checklist

Before starting a plugin server:

- [ ] PostgreSQL is running (`pg_isready`)
- [ ] Database exists (`psql -l | grep nself`)
- [ ] Tables initialized (`npm start init`)
- [ ] .env file exists with required variables
- [ ] Port is available (`lsof -i :3001`)
- [ ] Plugin builds successfully (`npm run build`)
- [ ] No other instance running (`ps aux | grep node`)

---

## Getting More Help

If you're still stuck after trying these solutions:

1. **Enable Debug Logging**
   ```bash
   DEBUG=true LOG_LEVEL=debug npm start
   ```

2. **Check Plugin Logs**
   ```bash
   tail -f ~/.nself/logs/*.log
   ```

3. **Review Documentation**
   - [Debugging Guide](./Debugging.md)
   - [FAQ](./FAQ.md)
   - Plugin-specific docs in the wiki Plugins section

4. **Search Issues**
   - GitHub Issues: https://github.com/acamarata/nself-plugins/issues

5. **Ask for Help**
   - Create a new issue with:
     - Error message (full stack trace)
     - Steps to reproduce
     - Environment details (OS, Node version, PostgreSQL version)
     - Relevant configuration (redact secrets!)

---

**Last Updated:** 2026-01-30
