# Frequently Asked Questions (FAQ)

Comprehensive answers to common questions about nself plugins.

---

## Table of Contents

### Installation & Setup
1. [What are the system requirements?](#what-are-the-system-requirements)
2. [Do I need to install all plugins?](#do-i-need-to-install-all-plugins)
3. [Can I use this with an existing PostgreSQL database?](#can-i-use-this-with-an-existing-postgresql-database)
4. [How do I get API keys for each service?](#how-do-i-get-api-keys-for-each-service)
5. [Can I run multiple plugins simultaneously?](#can-i-run-multiple-plugins-simultaneously)

### Configuration
6. [Where should I store my credentials?](#where-should-i-store-my-credentials)
7. [How do I configure different environments?](#how-do-i-configure-different-environments)
8. [Can I use a remote PostgreSQL database?](#can-i-use-a-remote-postgresql-database)
9. [What ports do the plugins use?](#what-ports-do-the-plugins-use)
10. [How do I enable debug logging?](#how-do-i-enable-debug-logging)

### Usage
11. [How often should I run sync?](#how-often-should-i-run-sync)
12. [What's the difference between full sync and incremental sync?](#whats-the-difference-between-full-sync-and-incremental-sync)
13. [Do I need to run the server for sync to work?](#do-i-need-to-run-the-server-for-sync-to-work)
14. [How do I query synced data?](#how-do-i-query-synced-data)
15. [Can I sync data from multiple accounts?](#can-i-sync-data-from-multiple-accounts)

### Webhooks
16. [What are webhooks and do I need them?](#what-are-webhooks-and-do-i-need-them)
17. [How do I set up webhooks in production?](#how-do-i-set-up-webhooks-in-production)
18. [Can I test webhooks locally?](#can-i-test-webhooks-locally)
19. [What happens if webhook delivery fails?](#what-happens-if-webhook-delivery-fails)
20. [How do I verify webhook signatures?](#how-do-i-verify-webhook-signatures)

### Data & Performance
21. [How much data can the plugins handle?](#how-much-data-can-the-plugins-handle)
22. [Will syncing affect my API rate limits?](#will-syncing-affect-my-api-rate-limits)
23. [How do I handle deleted records?](#how-do-i-handle-deleted-records)
24. [Can I customize which data gets synced?](#can-i-customize-which-data-gets-synced)
25. [How do I back up synced data?](#how-do-i-back-up-synced-data)

### Development
26. [How do I contribute a new plugin?](#how-do-i-contribute-a-new-plugin)
27. [Can I modify an existing plugin?](#can-i-modify-an-existing-plugin)
28. [How do I test my changes?](#how-do-i-test-my-changes)
29. [What's the release process?](#whats-the-release-process)

### Troubleshooting
30. [Why is my sync taking so long?](#why-is-my-sync-taking-so-long)
31. [I'm getting rate limited, what should I do?](#im-getting-rate-limited-what-should-i-do)
32. [How do I reset everything and start fresh?](#how-do-i-reset-everything-and-start-fresh)

---

## Installation & Setup

### What are the system requirements?

**Minimum Requirements:**
- **Node.js**: v18.0.0 or higher
- **PostgreSQL**: v12 or higher (v14+ recommended)
- **npm**: v8 or higher
- **Operating System**: macOS, Linux, or Windows (WSL2)
- **RAM**: 2GB minimum, 4GB+ recommended
- **Disk Space**: 1GB for code, additional space for data

**Recommended Setup:**
- **Node.js**: v20 LTS
- **PostgreSQL**: v14 or v15
- **RAM**: 8GB+ for large syncs
- **Disk Space**: Calculate based on data volume:
  - Stripe with 10k customers: ~500MB
  - GitHub with 100 repos: ~1GB
  - Shopify with 5k products: ~750MB

**Development Tools:**
- TypeScript 5.0+
- Git
- Code editor (VS Code recommended)
- Command-line tools: `curl`, `jq`, `psql`

**Verification:**
```bash
# Check versions
node --version   # Should be v18+
npm --version    # Should be v8+
psql --version   # Should be 12+

# Check PostgreSQL is running
pg_isready

# Check available disk space
df -h
```

---

### Do I need to install all plugins?

**No, you only install the plugins you need.**

Each plugin is independent and can be installed/run separately:

```bash
# Install only Stripe plugin
cd plugins/stripe/ts
npm install
npm run build

# Don't need GitHub or Shopify
```

**However, the shared utilities are required:**

```bash
# Always build shared first
cd shared
npm install
npm run build

# Then install your plugin
cd ../plugins/stripe/ts
npm install
```

**Plugin Dependencies:**
- Each plugin depends on `@nself/plugin-utils` (the shared package)
- No plugins depend on each other
- You can run multiple plugins in parallel (they share the same database)

---

### Can I use this with an existing PostgreSQL database?

**Yes, with considerations:**

**Option 1: Separate Database (Recommended)**
```bash
# Create a dedicated database
createdb nself

# Point plugins to it
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
```

**Option 2: Same Database, Separate Schema**
```sql
-- Create nself schema
CREATE SCHEMA nself;

-- Grant permissions
GRANT ALL ON SCHEMA nself TO your_user;

-- Plugin tables will be in public schema by default
-- You can modify database.ts to use nself schema:
CREATE TABLE nself.stripe_customers (...);
```

**Option 3: Same Database, Same Schema**
- Plugin table names are prefixed (e.g., `stripe_customers`)
- Low risk of collision with existing tables
- Ensure your user has CREATE TABLE permission
- Consider using separate schema for cleaner separation

**Backup First:**
```bash
# Always backup before adding plugins
pg_dump your_database > backup.sql
```

---

### How do I get API keys for each service?

**Stripe:**
1. Go to https://dashboard.stripe.com/apikeys
2. Copy "Secret key" (starts with `sk_test_` or `sk_live_`)
3. For webhooks: https://dashboard.stripe.com/webhooks
4. Create endpoint, copy "Signing secret" (starts with `whsec_`)

**GitHub:**
1. Go to https://github.com/settings/tokens
2. Click "Generate new token (classic)"
3. Select scopes:
   - `repo` (for private repos)
   - `read:org` (for organizations)
   - `read:user` (for user data)
4. Copy token (starts with `ghp_`)
5. For webhooks: Repository Settings > Webhooks > Add webhook

**Shopify:**
1. Shopify Admin > Apps > Develop apps
2. Create app or select existing
3. Configure Admin API scopes (read/write as needed)
4. Install app to your store
5. Reveal "Admin API access token" (starts with `shpat_`)
6. Copy "API secret key" for webhooks (starts with `shpss_`)
7. For webhooks: Settings > Notifications > Webhooks

**Security Best Practices:**
- Use test/sandbox keys for development
- Never commit keys to git
- Store in `.env` file (gitignored)
- Rotate keys periodically
- Use least-privilege scopes

---

### Can I run multiple plugins simultaneously?

**Yes, each plugin runs on a different port:**

```bash
# Terminal 1 - Stripe
cd plugins/stripe/ts
PORT=3001 npm start server

# Terminal 2 - GitHub
cd plugins/github/ts
PORT=3002 npm start server

# Terminal 3 - Shopify
cd plugins/shopify/ts
PORT=3003 npm start server
```

**All plugins share the same database:**
```bash
# All use the same DATABASE_URL
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/nself
```

**Process Management:**

Using `pm2` (recommended for production):
```bash
# Install pm2
npm install -g pm2

# Create ecosystem.config.js
module.exports = {
  apps: [
    {
      name: 'stripe',
      cwd: './plugins/stripe/ts',
      script: 'npm',
      args: 'start server',
      env: { PORT: 3001 }
    },
    {
      name: 'github',
      cwd: './plugins/github/ts',
      script: 'npm',
      args: 'start server',
      env: { PORT: 3002 }
    },
    {
      name: 'shopify',
      cwd: './plugins/shopify/ts',
      script: 'npm',
      args: 'start server',
      env: { PORT: 3003 }
    }
  ]
};

# Start all
pm2 start ecosystem.config.js

# Monitor
pm2 monit

# Stop all
pm2 stop all
```

---

## Configuration

### Where should I store my credentials?

**Development:**
```bash
# Create .env in plugin directory
cd plugins/stripe/ts
cat > .env <<EOF
STRIPE_API_KEY=sk_test_xxxxx
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
DATABASE_URL=postgresql://postgres:postgres@localhost:5432/nself
PORT=3001
LOG_LEVEL=debug
EOF

# .env is gitignored by default
```

**Production:**

**Option 1: Environment Variables**
```bash
# Export in shell
export STRIPE_API_KEY=sk_live_xxxxx
export DATABASE_URL=postgresql://...

# Or in systemd service
[Service]
Environment="STRIPE_API_KEY=sk_live_xxxxx"
Environment="DATABASE_URL=postgresql://..."
```

**Option 2: Environment File**
```bash
# Store in secure location
sudo mkdir -p /etc/nself
sudo chmod 700 /etc/nself

# Create env file
sudo tee /etc/nself/stripe.env > /dev/null <<EOF
STRIPE_API_KEY=sk_live_xxxxx
STRIPE_WEBHOOK_SECRET=whsec_xxxxx
DATABASE_URL=postgresql://...
EOF

sudo chmod 600 /etc/nself/stripe.env

# Load in service
[Service]
EnvironmentFile=/etc/nself/stripe.env
```

**Option 3: Secrets Manager**
```bash
# AWS Secrets Manager
aws secretsmanager get-secret-value \
  --secret-id nself/stripe/api-key \
  --query SecretString \
  --output text

# Use in startup script
export STRIPE_API_KEY=$(aws secretsmanager get-secret-value ...)
npm start server
```

**Never:**
- Commit to git
- Share in chat/email
- Hard-code in source files
- Store unencrypted on shared drives

---

### How do I configure different environments?

**Using Multiple .env Files:**

```bash
# Development
.env.development

# Staging
.env.staging

# Production
.env.production

# Load specific env
NODE_ENV=production node -r dotenv/config dist/index.js server
```

**Using Environment-Specific Configs:**

```typescript
// src/config.ts
const environment = process.env.NODE_ENV || 'development';

const config = {
  development: {
    stripe: {
      apiKey: process.env.STRIPE_TEST_KEY,
      webhookSecret: process.env.STRIPE_TEST_WEBHOOK_SECRET,
    },
    database: {
      url: 'postgresql://localhost:5432/nself_dev',
    },
    server: {
      port: 3001,
      host: 'localhost',
    },
  },
  production: {
    stripe: {
      apiKey: process.env.STRIPE_LIVE_KEY,
      webhookSecret: process.env.STRIPE_LIVE_WEBHOOK_SECRET,
    },
    database: {
      url: process.env.DATABASE_URL,
    },
    server: {
      port: parseInt(process.env.PORT || '3001'),
      host: '0.0.0.0',
    },
  },
};

export default config[environment];
```

**Best Practice: Use git branches**
```bash
# Development branch uses .env.development
git checkout development
npm start server  # Uses dev config

# Production branch uses .env.production
git checkout production
npm start server  # Uses prod config
```

---

### Can I use a remote PostgreSQL database?

**Yes, absolutely.**

**Connection String Format:**
```bash
DATABASE_URL=postgresql://username:password@hostname:port/database?sslmode=require
```

**Examples:**

**AWS RDS:**
```bash
DATABASE_URL=postgresql://admin:password@nself-db.xxxxx.us-east-1.rds.amazonaws.com:5432/nself?sslmode=require
```

**Heroku Postgres:**
```bash
DATABASE_URL=postgres://user:pass@ec2-xxx.compute-1.amazonaws.com:5432/db?sslmode=require
```

**DigitalOcean Managed Database:**
```bash
DATABASE_URL=postgresql://doadmin:pass@db-postgresql-nyc3-xxxxx.ondigitalocean.com:25060/nself?sslmode=require
```

**Supabase:**
```bash
DATABASE_URL=postgresql://postgres:password@db.xxxxx.supabase.co:5432/postgres
```

**SSL Configuration:**

If you need custom SSL settings:
```typescript
// Modify database.ts
import { Pool } from 'pg';

const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
  ssl: {
    rejectUnauthorized: false, // For self-signed certs
    ca: fs.readFileSync('/path/to/ca.crt').toString(),
  },
});
```

**Performance Considerations:**
- Higher latency for remote databases
- Sync operations will be slower
- Consider running plugin server close to database (same region)
- Use connection pooling (default in pg)

---

### What ports do the plugins use?

**Default Ports:**
- Stripe: `3001`
- GitHub: `3002`
- Shopify: `3003`

**Custom Ports:**
```bash
# Set via environment variable
PORT=8080 npm start server

# Or in .env
PORT=8080
```

**Port Assignment in Code:**
```typescript
// src/server.ts
const port = parseInt(process.env.PORT || '3001');
```

**Firewall Configuration:**

If running in production with webhooks:
```bash
# Allow inbound on plugin port
sudo ufw allow 3001/tcp

# Or use reverse proxy (recommended)
# Nginx listens on 443, forwards to plugin on 3001
```

**Reverse Proxy Example (Nginx):**
```nginx
server {
  listen 443 ssl;
  server_name stripe-webhook.yourdomain.com;

  location / {
    proxy_pass http://localhost:3001;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
  }
}
```

---

### How do I enable debug logging?

**Method 1: Environment Variable**
```bash
LOG_LEVEL=debug npm start server

# Or in .env
LOG_LEVEL=debug
```

**Method 2: DEBUG Flag**
```bash
DEBUG=true npm start server
```

**Log Levels:**
- `error` - Only errors (default in production)
- `warn` - Warnings and errors
- `info` - Info, warnings, errors (default in development)
- `debug` - Everything including debug messages

**Output:**
```bash
# Console output
2026-01-30T10:30:45.123Z [stripe:sync] DEBUG Fetching customers page 1
2026-01-30T10:30:45.234Z [stripe:sync] DEBUG API response: 100 customers
2026-01-30T10:30:45.345Z [stripe:sync] INFO Synced 100 customers

# File logging (if configured)
tail -f ~/.nself/logs/stripe.log
```

**Custom Logger Configuration:**
```typescript
// src/config.ts
import { createLogger } from '@nself/plugin-utils';

const logger = createLogger('stripe:custom', {
  level: process.env.LOG_LEVEL || 'info',
  file: process.env.LOG_FILE,
  json: process.env.LOG_FORMAT === 'json',
});
```

---

## Usage

### How often should I run sync?

**Depends on your use case:**

**Option 1: Webhooks Only (Recommended)**
```bash
# Start server once, webhooks keep data current
npm start server

# Optional: Initial sync to backfill historical data
npm start sync --full

# No scheduled syncs needed - webhooks handle everything!
```

**Option 2: Scheduled Syncs**
```bash
# Cron job for hourly incremental sync
0 * * * * cd /path/to/plugin && npm start sync --since "1 hour ago"

# Daily full sync (for safety/completeness)
0 2 * * * cd /path/to/plugin && npm start sync --full

# Use with or without webhooks
```

**Option 3: On-Demand**
```bash
# Sync whenever needed
npm start sync --full

# Good for development or infrequent updates
```

**Recommendations by Data Volume:**

| Data Volume | Strategy |
|-------------|----------|
| < 1k records | Full sync every 15-60 minutes |
| 1k-10k records | Incremental hourly + full daily |
| 10k-100k records | Webhooks + daily incremental |
| 100k+ records | Webhooks only + weekly full |

---

### What's the difference between full sync and incremental sync?

**Full Sync:**
```bash
npm start sync --full
```
- Fetches ALL data from the service
- Updates every record in database
- Slow for large datasets
- Use for: initial setup, data recovery, completeness verification

**Incremental Sync:**
```bash
npm start sync --since "2 hours ago"
```
- Fetches only records created/updated since timestamp
- Much faster
- May miss deletions (unless service provides deleted events)
- Use for: keeping data current between full syncs

**Examples:**

```typescript
// Full sync implementation
async fullSync(): Promise<void> {
  await this.syncCustomers();      // All customers
  await this.syncProducts();       // All products
  await this.syncOrders();         // All orders
  // ... all resources
}

// Incremental sync implementation
async incrementalSync(since: Date): Promise<void> {
  await this.syncCustomers({ created_since: since });
  await this.syncProducts({ updated_since: since });
  await this.syncOrders({ created_since: since });
}
```

**Best Practice: Combined Strategy**
```bash
# Daily incremental syncs
0 */6 * * * npm start sync --since "6 hours ago"

# Weekly full sync
0 3 * * 0 npm start sync --full

# Plus webhooks for real-time
npm start server  # Keep running
```

---

### Do I need to run the server for sync to work?

**No, sync and server are independent:**

**Sync Only (CLI):**
```bash
# Run sync without server
npm start sync --full

# Just syncs data and exits
```

**Server Only (Webhooks):**
```bash
# Run server without explicit sync
npm start server

# Handles webhooks, has API endpoints
# Can trigger sync via API: POST /api/sync
```

**Both:**
```bash
# Terminal 1: Server for webhooks
npm start server

# Terminal 2: Scheduled syncs
npm start sync --since "1 hour ago"
```

**When to use each:**

| Use Case | Command | Why |
|----------|---------|-----|
| Initial data load | `sync --full` | One-time backfill |
| Scheduled updates | `sync --since ...` | Cron job |
| Real-time updates | `server` | Webhooks |
| Development | `server` | Test webhooks & API |
| Production | Both | Redundancy & completeness |

---

### How do I query synced data?

**Option 1: Direct SQL**
```bash
# psql
psql $DATABASE_URL

# Query
SELECT * FROM stripe_customers WHERE email LIKE '%@example.com';

# Joins
SELECT c.email, COUNT(o.id) as order_count
FROM stripe_customers c
LEFT JOIN stripe_orders o ON o.customer_id = c.id
GROUP BY c.email
ORDER BY order_count DESC;
```

**Option 2: Plugin REST API**
```bash
# List customers
curl http://localhost:3001/api/customers

# Get specific customer
curl http://localhost:3001/api/customers/cus_xxxxx

# Filter by query params (if plugin supports)
curl http://localhost:3001/api/customers?email=user@example.com
```

**Option 3: Node.js**
```javascript
import pg from 'pg';

const pool = new pg.Pool({
  connectionString: process.env.DATABASE_URL
});

// Query
const result = await pool.query(
  'SELECT * FROM stripe_customers WHERE email = $1',
  ['user@example.com']
);

console.log(result.rows);
```

**Option 4: Python**
```python
import psycopg2

conn = psycopg2.connect(os.environ['DATABASE_URL'])
cur = conn.cursor()

cur.execute('SELECT * FROM stripe_customers WHERE email = %s', ('user@example.com',))
customers = cur.fetchall()

print(customers)
```

**Option 5: Using Views**
```sql
-- Plugins provide analytics views
SELECT * FROM stripe_mrr;  -- Monthly recurring revenue
SELECT * FROM stripe_active_subscriptions;
SELECT * FROM github_repo_stats;
```

---

### Can I sync data from multiple accounts?

**Yes, but requires code modifications:**

**Current Limitation:**
- Each plugin instance syncs one account
- Tables don't have account_id column by default

**Solution 1: Run Multiple Instances**
```bash
# Account 1
cd plugins/stripe/ts
cp .env .env.account1
# Edit .env.account1 with account 1 credentials
DATABASE_URL=postgresql://localhost/nself_account1 npm start server

# Account 2
cp .env .env.account2
# Edit .env.account2 with account 2 credentials
DATABASE_URL=postgresql://localhost/nself_account2 npm start server
```

**Solution 2: Modify Schema**
```sql
-- Add account_id to all tables
ALTER TABLE stripe_customers ADD COLUMN account_id VARCHAR(255);
CREATE INDEX idx_stripe_customers_account ON stripe_customers(account_id);

-- Update upsert queries to include account_id
INSERT INTO stripe_customers (id, account_id, email, ...)
VALUES ($1, $2, $3, ...)
ON CONFLICT (id, account_id) DO UPDATE ...
```

```typescript
// Modify database.ts
async upsertCustomer(customer: Customer, accountId: string) {
  await this.db.execute(
    `INSERT INTO stripe_customers (id, account_id, email, ...)
     VALUES ($1, $2, $3, ...)
     ON CONFLICT (id, account_id) DO UPDATE ...`,
    [customer.id, accountId, customer.email, ...]
  );
}
```

**Solution 3: Table Prefixes**
```typescript
// Modify database.ts to use account-specific table names
const tableName = `stripe_customers_${accountId}`;

await this.db.execute(
  `INSERT INTO ${tableName} (id, email, ...)
   VALUES ($1, $2, ...)
   ON CONFLICT (id) DO UPDATE ...`,
  [customer.id, customer.email, ...]
);
```

**Recommended: Separate databases** for complete isolation.

---

## Webhooks

### What are webhooks and do I need them?

**What are webhooks?**
- HTTP callbacks from services when events occur
- Near real-time data updates (seconds vs hours)
- Push model vs pull (sync) model

**Example Flow:**
```
1. Customer updates email in Stripe
2. Stripe sends webhook to your server: POST /webhook
3. Plugin receives event, updates database
4. Data is current within seconds
```

**Do you need them?**

**Yes, if:**
- You need real-time data (e.g., dashboards, notifications)
- High-frequency updates (e.g., e-commerce orders)
- Triggering actions based on events
- Reducing API calls / staying under rate limits

**No, if:**
- Batch analytics only (daily reports)
- Historical data analysis
- Infrequent updates
- Development/testing only

**Hybrid Approach (Recommended):**
```bash
# Use webhooks for real-time
npm start server  # Always running

# Plus periodic full sync for safety
0 3 * * * npm start sync --full  # Daily at 3 AM
```

This ensures:
- Real-time updates via webhooks
- Data completeness via full sync (catches any missed webhooks)

---

### How do I set up webhooks in production?

**Prerequisites:**
1. Public server with static IP or domain
2. HTTPS endpoint (required by most services)
3. Firewall allows inbound on webhook port

**Step-by-Step:**

#### 1. Set Up Server

```bash
# Ensure server is running
npm start server

# Verify it responds
curl http://localhost:3001/webhook
```

#### 2. Configure Reverse Proxy (Nginx)

```nginx
# /etc/nginx/sites-available/stripe-webhook
server {
  listen 80;
  server_name stripe.yourdomain.com;

  # Redirect to HTTPS
  return 301 https://$server_name$request_uri;
}

server {
  listen 443 ssl http2;
  server_name stripe.yourdomain.com;

  # SSL certificates (Let's Encrypt)
  ssl_certificate /etc/letsencrypt/live/stripe.yourdomain.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/stripe.yourdomain.com/privkey.pem;

  location /webhook {
    proxy_pass http://localhost:3001/webhook;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # Important: Don't buffer request body
    proxy_request_buffering off;
  }
}
```

```bash
# Enable site
sudo ln -s /etc/nginx/sites-available/stripe-webhook /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx

# Get SSL cert
sudo certbot --nginx -d stripe.yourdomain.com
```

#### 3. Configure Webhook in Service

**Stripe:**
1. Dashboard > Webhooks > Add endpoint
2. URL: `https://stripe.yourdomain.com/webhook`
3. Events: Select all or specific events
4. Copy webhook signing secret
5. Add to .env: `STRIPE_WEBHOOK_SECRET=whsec_xxxxx`

**GitHub:**
1. Repository Settings > Webhooks > Add webhook
2. Payload URL: `https://github.yourdomain.com/webhook`
3. Content type: `application/json`
4. Secret: Generate random string
5. Events: Select events
6. Add to .env: `GITHUB_WEBHOOK_SECRET=your_secret`

**Shopify:**
1. Admin > Settings > Notifications > Webhooks
2. Create webhook
3. URL: `https://shopify.yourdomain.com/webhook`
4. Format: JSON
5. Add to .env: `SHOPIFY_API_SECRET=shpss_xxxxx`

#### 4. Test Webhook

```bash
# Send test event from service dashboard
# Watch logs
tail -f ~/.nself/logs/stripe.log

# Should see:
# Webhook received: customer.created
# Processing customer.created event
# Customer cus_xxxxx created
```

#### 5. Monitor

```bash
# Check webhook events table
psql $DATABASE_URL -c "SELECT * FROM stripe_webhook_events ORDER BY created_at DESC LIMIT 10;"

# Check for errors
psql $DATABASE_URL -c "SELECT * FROM stripe_webhook_events WHERE error IS NOT NULL;"
```

---

### Can I test webhooks locally?

**Yes, using tunneling tools:**

#### Option 1: ngrok (Recommended)

```bash
# Install
brew install ngrok

# Start tunnel
ngrok http 3001

# Output:
# Forwarding https://abc123.ngrok.io -> http://localhost:3001

# Use https://abc123.ngrok.io/webhook in service config
```

#### Option 2: Stripe CLI (for Stripe only)

```bash
# Install
brew install stripe/stripe-cli/stripe

# Login
stripe login

# Forward webhooks
stripe listen --forward-to http://localhost:3001/webhook

# Output:
# Ready! Your webhook signing secret is whsec_xxxxx

# Add to .env
STRIPE_WEBHOOK_SECRET=whsec_xxxxx

# Trigger test events
stripe trigger customer.created
stripe trigger payment_intent.succeeded
```

#### Option 3: localhost.run (No installation)

```bash
# Start tunnel
ssh -R 80:localhost:3001 localhost.run

# Outputs public URL
# Use in webhook configuration
```

#### Option 4: Manual Testing with curl

```bash
# Create test payload
cat > test-webhook.json <<EOF
{
  "id": "evt_test",
  "type": "customer.created",
  "data": {
    "object": {
      "id": "cus_test",
      "email": "test@example.com"
    }
  }
}
EOF

# Generate signature (Stripe example)
timestamp=$(date +%s)
payload="$timestamp.$(cat test-webhook.json)"
signature=$(echo -n "$payload" | openssl dgst -sha256 -hmac "whsec_test" | cut -d' ' -f2)

# Send request
curl -X POST http://localhost:3001/webhook \
  -H "Content-Type: application/json" \
  -H "Stripe-Signature: t=$timestamp,v1=$signature" \
  -d @test-webhook.json
```

---

### What happens if webhook delivery fails?

**Service Retry Behavior:**

**Stripe:**
- Retries for 3 days
- Exponential backoff
- Disables endpoint after repeated failures
- Dashboard shows delivery attempts

**GitHub:**
- Retries several times over a few hours
- Exponential backoff
- Provides redelivery button in UI
- 7-day webhook history

**Shopify:**
- Retries for 48 hours
- Configurable retry policy
- Manual redelivery option

**Plugin Behavior:**

```typescript
// Webhook handler returns 200 even if processing fails
async handleWebhook(request, reply) {
  try {
    // Verify signature
    const event = verifyWebhook(request);

    // Store raw event
    await this.db.insertWebhookEvent(event);

    // Return 200 immediately
    reply.status(200).send({ received: true });

    // Process asynchronously
    this.processEvent(event).catch(error => {
      logger.error('Event processing failed:', error);
      // Event is in database, can retry later
    });

  } catch (error) {
    // Only return error for invalid signature
    logger.error('Webhook verification failed:', error);
    reply.status(400).send({ error: 'Invalid signature' });
  }
}
```

**Recovery Options:**

```bash
# 1. Query unprocessed events
psql $DATABASE_URL -c "
  SELECT * FROM stripe_webhook_events
  WHERE processed_at IS NULL
  ORDER BY created_at;
"

# 2. Retry failed events
npm start retry-webhooks

# 3. Manual sync to catch up
npm start sync --since "2 days ago"

# 4. Redeliver from service dashboard
# Stripe: Dashboard > Webhooks > Event > Redeliver
```

---

### How do I verify webhook signatures?

Each service has a different signature scheme:

#### Stripe

```typescript
import Stripe from 'stripe';

function verifyStripeWebhook(
  rawBody: string,
  signature: string,
  secret: string
): Stripe.Event {
  try {
    return Stripe.webhooks.constructEvent(rawBody, signature, secret);
  } catch (error) {
    throw new Error('Invalid signature');
  }
}

// In webhook handler
const rawBody = request.body.toString('utf8');
const signature = request.headers['stripe-signature'];
const event = verifyStripeWebhook(rawBody, signature, webhookSecret);
```

#### GitHub

```typescript
import crypto from 'crypto';

function verifyGitHubWebhook(
  rawBody: string,
  signature: string,
  secret: string
): boolean {
  // Signature format: sha256=<hash>
  const hash = signature.substring(7);

  const hmac = crypto.createHmac('sha256', secret);
  hmac.update(rawBody);
  const expectedHash = hmac.digest('hex');

  return crypto.timingSafeEqual(
    Buffer.from(hash),
    Buffer.from(expectedHash)
  );
}

// In webhook handler
const rawBody = request.body.toString('utf8');
const signature = request.headers['x-hub-signature-256'];
if (!verifyGitHubWebhook(rawBody, signature, webhookSecret)) {
  throw new Error('Invalid signature');
}
```

#### Shopify

```typescript
import crypto from 'crypto';

function verifyShopifyWebhook(
  rawBody: string,
  hmacHeader: string,
  secret: string
): boolean {
  const hash = crypto
    .createHmac('sha256', secret)
    .update(rawBody)
    .digest('base64');

  return crypto.timingSafeEqual(
    Buffer.from(hash),
    Buffer.from(hmacHeader)
  );
}

// In webhook handler
const rawBody = request.body.toString('utf8');
const hmac = request.headers['x-shopify-hmac-sha256'];
if (!verifyShopifyWebhook(rawBody, hmac, apiSecret)) {
  throw new Error('Invalid HMAC');
}
```

**Common Mistakes:**

1. **Using parsed JSON instead of raw body**
   ```typescript
   // WRONG
   const body = JSON.stringify(request.body);
   verify(body, signature, secret);

   // CORRECT
   const rawBody = request.body.toString('utf8');
   verify(rawBody, signature, secret);
   ```

2. **Wrong secret**
   ```bash
   # Stripe: Use webhook secret (whsec_), not API key (sk_)
   STRIPE_WEBHOOK_SECRET=whsec_xxxxx  # Correct

   # Shopify: Use API secret key (shpss_), not access token (shpat_)
   SHOPIFY_API_SECRET=shpss_xxxxx  # Correct
   ```

3. **Clock skew**
   ```bash
   # Stripe checks timestamp is within 5 minutes
   # Ensure server time is synchronized
   sudo ntpdate -s time.nist.gov
   ```

---

## Data & Performance

### How much data can the plugins handle?

**Tested Limits:**

| Plugin | Records | Database Size | Sync Time | Notes |
|--------|---------|---------------|-----------|-------|
| Stripe | 100k customers | ~2GB | 2-3 hours | Full sync |
| GitHub | 1k repos | ~500MB | 30 minutes | With full history |
| Shopify | 50k products | ~1.5GB | 1-2 hours | With variants |

**Scalability Factors:**

1. **PostgreSQL Performance**
   - Proper indexing critical
   - Regular VACUUM
   - Connection pooling
   - Consider read replicas for queries

2. **API Rate Limits**
   - Stripe: 100 req/sec (can request increase)
   - GitHub: 5000 req/hour
   - Shopify: 2-4 req/sec (tier-dependent)

3. **Network/Latency**
   - Local database: Fast
   - Remote database: Slower sync
   - Geographic proximity matters

**Optimization Tips:**

```sql
-- 1. Add indexes for common queries
CREATE INDEX idx_stripe_customers_email ON stripe_customers(email);
CREATE INDEX idx_stripe_orders_created ON stripe_orders(created_at DESC);

-- 2. Partition large tables (100M+ rows)
CREATE TABLE stripe_events_2026_01 PARTITION OF stripe_events
  FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

-- 3. Regular maintenance
VACUUM ANALYZE stripe_customers;
REINDEX TABLE stripe_customers;

-- 4. Monitor table sizes
SELECT schemaname, tablename, pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename))
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

**When to scale:**

- **Vertical**: More RAM, faster CPU, SSD
- **Horizontal**: Read replicas, sharding
- **Caching**: Redis for frequently accessed data
- **Archive**: Move old data to separate tables/database

---

### Will syncing affect my API rate limits?

**Yes, syncing consumes API quota.**

**Rate Limit Impact:**

**Stripe:**
- Full sync of 10k customers = ~100 API calls = 1 second of quota
- With products, subscriptions, etc. = more calls
- Usually not an issue due to high limits

**GitHub:**
- Full sync of 100 repos = ~100 API calls
- With issues, PRs, commits = 1000s of calls
- Can exhaust hourly limit (5000)

**Shopify:**
- Full sync of 5k products = 50 API calls (100 per page)
- Variants, inventory = additional calls
- May hit rate limit with large catalogs

**Mitigation Strategies:**

#### 1. Use Rate Limiter

```typescript
// Set to service's limit
const rateLimiter = new RateLimiter(
  service === 'stripe' ? 100 :
  service === 'github' ? 1 :
  2  // shopify
);

await rateLimiter.acquire();
await apiCall();
```

#### 2. Incremental Sync

```bash
# Instead of full sync
npm start sync --full

# Sync only recent changes
npm start sync --since "1 hour ago"
```

#### 3. Webhooks

```bash
# Real-time updates without API calls
npm start server
```

#### 4. Schedule During Off-Peak

```bash
# Run full sync at night
0 3 * * * npm start sync --full
```

#### 5. Monitor Usage

```bash
# Check GitHub rate limit
curl -H "Authorization: token $GITHUB_TOKEN" \
  https://api.github.com/rate_limit

# Stripe (in response headers)
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1643587200
```

**Best Practice:**
```bash
# Initial setup: Full sync
npm start sync --full

# Ongoing: Webhooks + periodic incremental
npm start server  # Always running
0 */6 * * * npm start sync --since "6 hours ago"  # Every 6 hours
```

---

### How do I handle deleted records?

**Challenge:**
- Most APIs don't provide "list deleted items" endpoint
- You don't know what was deleted unless service tells you

**Solutions:**

#### 1. Webhook Events (Best)

```typescript
// Stripe sends deleted events
async handleCustomerDeleted(event: Stripe.Event) {
  const customerId = event.data.object.id;

  // Option A: Soft delete
  await this.db.execute(
    `UPDATE stripe_customers SET deleted = true, deleted_at = NOW() WHERE id = $1`,
    [customerId]
  );

  // Option B: Hard delete
  await this.db.execute(
    `DELETE FROM stripe_customers WHERE id = $1`,
    [customerId]
  );
}
```

#### 2. Track Last Seen

```sql
-- Add last_seen column
ALTER TABLE stripe_customers ADD COLUMN last_seen_at TIMESTAMP DEFAULT NOW();

-- Update on every sync
UPDATE stripe_customers SET last_seen_at = NOW() WHERE id = $1;

-- Find stale records (not seen in 7 days)
SELECT * FROM stripe_customers
WHERE last_seen_at < NOW() - INTERVAL '7 days';

-- Mark as deleted
UPDATE stripe_customers SET deleted = true
WHERE last_seen_at < NOW() - INTERVAL '7 days';
```

#### 3. Compare Full List

```typescript
// Get all IDs from service
const apiIds = await client.listAllIds();

// Get all IDs from database
const dbIds = await db.query('SELECT id FROM stripe_customers');

// Find deleted
const deleted = dbIds.filter(id => !apiIds.includes(id));

// Mark as deleted
for (const id of deleted) {
  await db.execute(
    `UPDATE stripe_customers SET deleted = true WHERE id = $1`,
    [id]
  );
}
```

**Recommended Schema:**

```sql
CREATE TABLE stripe_customers (
  id VARCHAR(255) PRIMARY KEY,
  email VARCHAR(255),
  name VARCHAR(255),
  created_at TIMESTAMP,
  updated_at TIMESTAMP,
  deleted BOOLEAN DEFAULT false,
  deleted_at TIMESTAMP,
  synced_at TIMESTAMP DEFAULT NOW(),
  last_seen_at TIMESTAMP DEFAULT NOW()
);

-- Query active customers
SELECT * FROM stripe_customers WHERE deleted = false;
```

---

### Can I customize which data gets synced?

**Yes, by modifying sync.ts:**

#### Example: Skip Certain Resources

```typescript
// src/sync.ts
async fullSync(): Promise<void> {
  logger.info('Starting full sync');

  // Sync only customers and orders, skip products
  await this.syncCustomers();
  // await this.syncProducts();  // Commented out
  await this.syncOrders();

  logger.info('Full sync complete');
}
```

#### Example: Filter Records

```typescript
// Sync only active customers
async syncCustomers(): Promise<void> {
  const customers = await this.client.listCustomers({
    deleted: false  // Only active
  });

  // Filter further
  const filtered = customers.filter(c =>
    c.email && !c.email.includes('test')  // Skip test accounts
  );

  await this.database.upsertCustomers(filtered);
}
```

#### Example: Limit Date Range

```typescript
// Sync only recent orders
async syncOrders(): Promise<void> {
  const thirtyDaysAgo = new Date();
  thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);

  const orders = await this.client.listOrders({
    created_after: thirtyDaysAgo
  });

  await this.database.upsertOrders(orders);
}
```

#### Example: Selective Fields

```typescript
// Sync only essential fields
private mapCustomer(api: APICustomer): CustomerRecord {
  return {
    id: api.id,
    email: api.email,
    name: api.name,
    // Skip metadata, address, etc.
    created_at: new Date(api.created * 1000),
    updated_at: new Date(api.updated * 1000),
  };
}
```

---

### How do I back up synced data?

#### Option 1: PostgreSQL Dump

```bash
# Full database backup
pg_dump $DATABASE_URL > nself-backup-$(date +%Y%m%d).sql

# Compressed
pg_dump $DATABASE_URL | gzip > nself-backup-$(date +%Y%m%d).sql.gz

# Specific tables only
pg_dump $DATABASE_URL -t 'stripe_*' > stripe-backup.sql

# Automated daily backups
cat > /usr/local/bin/backup-nself.sh <<'EOF'
#!/bin/bash
DATE=$(date +%Y%m%d)
BACKUP_DIR=/backups/nself
mkdir -p $BACKUP_DIR

pg_dump $DATABASE_URL | gzip > $BACKUP_DIR/nself-$DATE.sql.gz

# Keep only last 30 days
find $BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete
EOF

chmod +x /usr/local/bin/backup-nself.sh

# Cron: Daily at 4 AM
0 4 * * * /usr/local/bin/backup-nself.sh
```

#### Option 2: Continuous Archiving (WAL)

```bash
# Enable WAL archiving in postgresql.conf
wal_level = replica
archive_mode = on
archive_command = 'cp %p /backups/wal/%f'

# Base backup
pg_basebackup -D /backups/base -Ft -z -P

# Point-in-time recovery possible
```

#### Option 3: Managed Database Backups

```bash
# AWS RDS: Automated backups enabled by default
# Retention: 1-35 days
# Manual snapshots: Indefinite

# Restore from snapshot via AWS Console or CLI
aws rds restore-db-instance-from-db-snapshot \
  --db-instance-identifier nself-restored \
  --db-snapshot-identifier nself-snapshot-20260130
```

#### Option 4: Export to CSV

```bash
# Export specific tables
psql $DATABASE_URL -c "COPY stripe_customers TO STDOUT CSV HEADER" > customers.csv

# All tables
for table in $(psql $DATABASE_URL -t -c "SELECT tablename FROM pg_tables WHERE schemaname='public'"); do
  psql $DATABASE_URL -c "COPY $table TO STDOUT CSV HEADER" > $table.csv
done
```

#### Option 5: Replicate to S3

```bash
# Export and upload
pg_dump $DATABASE_URL | gzip | aws s3 cp - s3://mybucket/backups/nself-$(date +%Y%m%d).sql.gz

# Automated via cron
0 5 * * * pg_dump $DATABASE_URL | gzip | aws s3 cp - s3://mybucket/backups/nself-$(date +\%Y\%m\%d).sql.gz
```

---

## Development

### How do I contribute a new plugin?

See [CONTRIBUTING.md](../CONTRIBUTING.md) for detailed guide.

**Quick Overview:**

1. **Choose a Service**
   - Has a REST API
   - Supports webhooks
   - Has predictable data model

2. **Study Existing Plugins**
   - Copy structure from stripe/github/shopify
   - Follow same file organization

3. **Create Plugin Structure**
   ```bash
   mkdir -p plugins/newservice/ts/src
   cd plugins/newservice/ts
   ```

4. **Implement Required Files**
   - types.ts (API + DB types)
   - client.ts (API wrapper)
   - database.ts (schema + CRUD)
   - sync.ts (sync logic)
   - webhooks.ts (event handlers)
   - server.ts (HTTP server)
   - cli.ts (CLI commands)

5. **Add to Registry**
   - Create plugin.json
   - Update registry.json

6. **Documentation**
   - Create `plugins/ServiceName.md`
   - Update `Home.md`

7. **Test**
   ```bash
   npm install
   npm run build
   npm start init
   npm start sync --full
   npm start server
   ```

8. **Submit PR**
   - Fork repo
   - Create branch
   - Push changes
   - Open PR with description

---

### Can I modify an existing plugin?

**Yes, and encouraged!**

**Common Modifications:**

#### 1. Add New Resource/Table

```typescript
// 1. Add types to types.ts
export interface Refund {
  id: string;
  amount: number;
  status: string;
  created: number;
}

export interface RefundRecord {
  id: string;
  amount: number;
  status: string;
  created_at: Date;
  synced_at: Date;
}

// 2. Add client method to client.ts
async listRefunds(): Promise<Refund[]> {
  await this.rateLimiter.acquire();
  const response = await this.http.get('/refunds');
  return response.data;
}

// 3. Add table to database.ts
async initSchema(): Promise<void> {
  await this.db.execute(`
    CREATE TABLE IF NOT EXISTS stripe_refunds (
      id VARCHAR(255) PRIMARY KEY,
      amount INTEGER NOT NULL,
      status VARCHAR(50),
      created_at TIMESTAMP,
      synced_at TIMESTAMP DEFAULT NOW()
    );
  `);
}

async upsertRefund(refund: RefundRecord): Promise<void> {
  await this.db.execute(
    `INSERT INTO stripe_refunds (id, amount, status, created_at, synced_at)
     VALUES ($1, $2, $3, $4, NOW())
     ON CONFLICT (id) DO UPDATE SET
       amount = EXCLUDED.amount,
       status = EXCLUDED.status,
       synced_at = NOW()`,
    [refund.id, refund.amount, refund.status, refund.created_at]
  );
}

// 4. Add sync method to sync.ts
async syncRefunds(): Promise<void> {
  const refunds = await this.client.listRefunds();
  for (const refund of refunds) {
    await this.database.upsertRefund(this.mapRefund(refund));
  }
}

// 5. Update fullSync
async fullSync(): Promise<void> {
  await this.syncCustomers();
  await this.syncRefunds();  // Add this
  // ...
}

// 6. Add webhook handler to webhooks.ts
case 'refund.created':
  await this.handleRefundCreated(event);
  break;

// 7. Update registry.json
"tables": ["stripe_customers", "stripe_refunds", ...]
```

#### 2. Add Custom Analytics View

```typescript
// In database.ts
async initSchema(): Promise<void> {
  // ... existing tables ...

  // Add custom view
  await this.db.execute(`
    CREATE OR REPLACE VIEW stripe_refund_stats AS
    SELECT
      DATE_TRUNC('month', created_at) as month,
      COUNT(*) as refund_count,
      SUM(amount) as total_refunded
    FROM stripe_refunds
    GROUP BY DATE_TRUNC('month', created_at)
    ORDER BY month DESC;
  `);
}
```

#### 3. Add Custom CLI Command

```typescript
// In cli.ts
program
  .command('export-customers')
  .description('Export customers to CSV')
  .action(async () => {
    const customers = await db.query('SELECT * FROM stripe_customers');
    console.log('id,email,name,created_at');
    customers.rows.forEach(c => {
      console.log(`${c.id},${c.email},${c.name},${c.created_at}`);
    });
  });
```

**After modifying:**
```bash
# Rebuild
npm run build

# Update database schema
npm start init

# Test
npm start sync --full
```

---

### How do I test my changes?

**Local Testing:**

```bash
# 1. Build
npm run build

# 2. Type check
npm run typecheck

# 3. Initialize database
npm start init

# 4. Test sync
DEBUG=true npm start sync --full

# 5. Test server
npm start server &

# 6. Test endpoints
curl http://localhost:3001/api/customers
curl http://localhost:3001/api/status

# 7. Test webhook
curl -X POST http://localhost:3001/webhook \
  -H "Content-Type: application/json" \
  -d '{"type": "test.event", "data": {}}'
```

**Database Testing:**

```sql
-- Check table was created
\dt stripe_*

-- Check data was synced
SELECT COUNT(*) FROM stripe_customers;
SELECT * FROM stripe_customers LIMIT 5;

-- Check indexes
\di stripe_*

-- Check views
\dv stripe_*
SELECT * FROM stripe_mrr;
```

**Integration Testing:**

```bash
# Use test API keys
STRIPE_API_KEY=sk_test_xxxxx npm start sync --full

# Verify test data appears
psql $DATABASE_URL -c "SELECT * FROM stripe_customers WHERE email LIKE '%@test.com'"
```

**Webhook Testing:**

```bash
# Use Stripe CLI
stripe listen --forward-to http://localhost:3001/webhook
stripe trigger customer.created

# Check database
psql $DATABASE_URL -c "SELECT * FROM stripe_webhook_events ORDER BY created_at DESC LIMIT 5"
```

**Automated Tests (Future):**

```typescript
// tests/sync.test.ts
import { describe, it, expect } from 'vitest';
import { SyncService } from '../src/sync.js';

describe('SyncService', () => {
  it('should sync customers', async () => {
    const sync = new SyncService();
    await sync.syncCustomers();

    const result = await db.query('SELECT COUNT(*) FROM stripe_customers');
    expect(result.rows[0].count).toBeGreaterThan(0);
  });
});
```

---

### What's the release process?

See the [Contributing Guide](../CONTRIBUTING.md) for the release process.

**Summary:**

1. **Make changes** in feature branch
2. **Test locally**
3. **Create PR** to main
4. **PR validated** by GitHub Actions
5. **Merge PR**
6. **Update versions** in plugin.json and registry.json
7. **Create tag**: `git tag -a v1.0.1 -m "Release v1.0.1"`
8. **Push tag**: `git push origin v1.0.1`
9. **GitHub Actions** automatically:
   - Updates registry.json timestamp/checksums
   - Notifies Cloudflare Worker
   - Creates GitHub Release

Users get updates via:
```bash
nself plugin updates
nself plugin install stripe
```

---

## Troubleshooting

### Why is my sync taking so long?

**Diagnosis:**

```bash
# Enable debug logging to see progress
DEBUG=true LOG_LEVEL=debug npm start sync --full

# Monitor database size growth
watch -n 5 'psql $DATABASE_URL -c "SELECT pg_size_pretty(pg_database_size(current_database()))"'

# Check active connections
psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity"

# Monitor API calls
# Look for log output like:
# [stripe:sync] DEBUG Fetched 100 customers (page 1)
# [stripe:sync] DEBUG Fetched 100 customers (page 2)
# ...
```

**Common Causes:**

1. **Large dataset**
   - 100k records can take hours
   - Solution: Use incremental sync

2. **Rate limiting**
   - RateLimiter too slow
   - Solution: Increase if within service limits

3. **Slow database**
   - Remote database with high latency
   - Solution: Run plugin server near database

4. **Missing indexes**
   - Upserts are slow without indexes
   - Solution: Ensure primary keys and indexes exist

5. **Network issues**
   - Slow API responses
   - Solution: Check network, use local database

**Optimizations:**

```typescript
// 1. Increase rate limit (if service allows)
const rateLimiter = new RateLimiter(200);  // was 100

// 2. Batch database operations
async upsertMany(records: Record[]): Promise<void> {
  // Instead of individual upserts
  const values = records.map(r => `('${r.id}', '${r.email}')`).join(',');
  await this.db.execute(`
    INSERT INTO stripe_customers (id, email)
    VALUES ${values}
    ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email
  `);
}

// 3. Parallelize independent syncs
async fullSync(): Promise<void> {
  await Promise.all([
    this.syncCustomers(),
    this.syncProducts(),
    this.syncOrders(),
  ]);
}

// 4. Use incremental sync
npm start sync --since "1 day ago"
```

---

### I'm getting rate limited, what should I do?

**Immediate Fix:**

```bash
# Stop all sync processes
pkill -f "npm start sync"

# Wait for rate limit to reset
# Check reset time in error message or API response headers

# Reduce rate limiter
# Edit src/client.ts
const rateLimiter = new RateLimiter(10);  // Reduced from 100

# Rebuild
npm run build

# Resume with incremental sync
npm start sync --since "1 hour ago"
```

**Long-term Solutions:**

#### 1. Use Webhooks Instead
```bash
# Real-time updates without API calls
npm start server
```

#### 2. Reduce Sync Frequency
```bash
# Instead of hourly
0 * * * * npm start sync

# Use daily
0 3 * * * npm start sync
```

#### 3. Incremental Sync Only
```bash
# Don't run full sync
# npm start sync --full

# Only incremental
npm start sync --since "6 hours ago"
```

#### 4. Request Limit Increase
- Stripe: Contact support for higher limits
- GitHub: Use GitHub App for higher limits (15k/hour)
- Shopify: Upgrade to Shopify Plus (40 req/sec)

#### 5. Implement Exponential Backoff
```typescript
async function requestWithBackoff<T>(
  fn: () => Promise<T>,
  maxRetries = 5
): Promise<T> {
  for (let i = 0; i < maxRetries; i++) {
    try {
      return await fn();
    } catch (error: any) {
      if (error.status === 429) {
        const waitTime = Math.pow(2, i) * 1000;
        logger.warn(`Rate limited, waiting ${waitTime}ms (attempt ${i + 1})`);
        await new Promise(resolve => setTimeout(resolve, waitTime));
      } else {
        throw error;
      }
    }
  }
  throw new Error('Max retries exceeded');
}
```

---

### How do I reset everything and start fresh?

**Complete Reset:**

```bash
# 1. Stop all plugin processes
pkill -f "npm start"

# 2. Drop all plugin tables
psql $DATABASE_URL <<EOF
DROP TABLE IF EXISTS stripe_customers CASCADE;
DROP TABLE IF EXISTS stripe_products CASCADE;
DROP TABLE IF EXISTS stripe_orders CASCADE;
-- ... all tables
DROP TABLE IF EXISTS stripe_webhook_events CASCADE;

-- Or drop entire schema if using separate schema
DROP SCHEMA IF EXISTS nself CASCADE;
CREATE SCHEMA nself;
EOF

# 3. Clean build artifacts
cd plugins/stripe/ts
rm -rf dist/ node_modules/ package-lock.json

# 4. Rebuild
npm install
npm run build

# 5. Initialize fresh
npm start init

# 6. Sync data
npm start sync --full

# 7. Start server
npm start server
```

**Reset Single Plugin:**

```bash
# Drop only Stripe tables
psql $DATABASE_URL -c "
  DO \$\$ DECLARE
    r RECORD;
  BEGIN
    FOR r IN (SELECT tablename FROM pg_tables WHERE tablename LIKE 'stripe_%') LOOP
      EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE';
    END LOOP;
  END \$\$;
"

# Re-initialize
cd plugins/stripe/ts
npm start init
npm start sync --full
```

**Reset Database Only (Keep Code):**

```bash
# Truncate all tables (keeps schema)
psql $DATABASE_URL <<EOF
TRUNCATE stripe_customers CASCADE;
TRUNCATE stripe_products CASCADE;
-- ... all tables
EOF

# Re-sync
npm start sync --full
```

**Nuclear Option:**

```bash
# Drop entire database and recreate
dropdb nself
createdb nself

# Rebuild all plugins
cd shared && npm install && npm run build
cd ../plugins/stripe/ts && npm install && npm run build && npm start init
cd ../plugins/github/ts && npm install && npm run build && npm start init
cd ../plugins/shopify/ts && npm install && npm run build && npm start init

# Sync all
cd ../stripe/ts && npm start sync --full
cd ../github/ts && npm start sync --full
cd ../shopify/ts && npm start sync --full
```

---

## Still Need Help?

### Documentation
- [Common Issues](./Common-Issues.md)
- [Debugging Guide](./Debugging.md)
- [Plugin-specific docs](../plugins/)

### Support
- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- Discussions: https://github.com/acamarata/nself-plugins/discussions

### Reporting Bugs
Include:
- Error message and full stack trace
- Steps to reproduce
- Environment (OS, Node version, PostgreSQL version)
- Plugin version (`cat plugin.json | grep version`)
- Configuration (redact secrets!)

---

**Last Updated:** 2026-01-30
