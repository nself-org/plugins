# Quick Start Guide

Welcome to nself plugins! This guide will walk you through installing your first plugin, syncing data, and setting up real-time webhook updates.

**Time to complete**: 15-30 minutes

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [First Plugin Setup (Stripe)](#first-plugin-setup-stripe)
4. [Running Your First Sync](#running-your-first-sync)
5. [Setting Up Webhooks](#setting-up-webhooks)
6. [Querying Your Data](#querying-your-data)
7. [Common Issues](#common-issues)
8. [Next Steps](#next-steps)

---

## Prerequisites

Before getting started, ensure you have the following installed and configured:

### 1. nself CLI (v0.4.8+)

Install the nself CLI if you haven't already:

```bash
# Install via curl
curl -fsSL https://nself.org/install.sh | bash

# Verify installation
nself --version
```

Expected output: `nself version 0.4.8` or higher

### 2. PostgreSQL (v14+)

You need a running PostgreSQL database. Choose one of these options:

**Option A: Docker (Recommended for local development)**

```bash
# Start PostgreSQL with Docker
docker run -d \
  --name nself-postgres \
  -e POSTGRES_USER=nself \
  -e POSTGRES_PASSWORD=nself \
  -e POSTGRES_DB=nself \
  -p 5432:5432 \
  postgres:16-alpine

# Verify it's running
docker ps | grep nself-postgres
```

**Option B: Local Installation**

```bash
# macOS
brew install postgresql@16
brew services start postgresql@16

# Ubuntu/Debian
sudo apt update
sudo apt install postgresql-16
sudo systemctl start postgresql

# Verify
psql --version
```

**Option C: Existing PostgreSQL**

If you already have PostgreSQL running, just note your connection details.

### 3. Node.js (v20+)

nself plugins are built with TypeScript and require Node.js:

```bash
# Check if installed
node --version
npm --version

# Install if needed (using nvm - recommended)
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash
nvm install 20
nvm use 20
```

### 4. Service Account (Stripe for this guide)

For this guide, you'll need a Stripe account:

1. Sign up at [stripe.com](https://stripe.com) (free for testing)
2. Get your API keys from [Dashboard > Developers > API Keys](https://dashboard.stripe.com/test/apikeys)

**Note**: Use test mode keys (`sk_test_...`) for this tutorial.

---

## Installation

### Step 1: Create Your Project Directory

```bash
# Create a directory for your nself project
mkdir my-nself-project
cd my-nself-project

# Initialize a basic project structure
mkdir -p logs config
```

### Step 2: Set Up Environment Variables

Create a `.env` file in your project directory:

```bash
# Create .env file
cat > .env << 'EOF'
# PostgreSQL Connection
DATABASE_URL=postgresql://nself:nself@localhost:5432/nself

# Stripe Configuration
STRIPE_API_KEY=sk_test_YOUR_KEY_HERE
STRIPE_WEBHOOK_SECRET=whsec_YOUR_SECRET_HERE

# Optional: Logging
LOG_LEVEL=info
EOF
```

**Important**: Replace `sk_test_YOUR_KEY_HERE` with your actual Stripe test key.

**Security Note**: Never commit your `.env` file to version control:

```bash
# Add to .gitignore
echo ".env" >> .gitignore
echo ".env.*" >> .gitignore
echo "!.env.example" >> .gitignore
```

### Step 3: List Available Plugins

Before installing, let's see what's available:

```bash
nself plugin list
```

Expected output:
```
Available Plugins:

Billing:
  stripe (v1.0.0) - Stripe billing data sync with webhook handling

DevOps:
  github (v1.0.0) - GitHub repository and workflow data sync

E-Commerce:
  shopify (v1.0.0) - Shopify store data sync

Authentication:
  idme (v1.0.0) - ID.me OAuth authentication

Infrastructure:
  realtime (v1.0.0) - Socket.io real-time server
  notifications (v1.0.0) - Multi-channel notifications
  file-processing (v1.0.0) - File processing with thumbnails
  jobs (v1.0.0) - BullMQ background job queue
```

---

## First Plugin Setup (Stripe)

Now let's install and configure the Stripe plugin.

### Step 1: Install the Plugin

```bash
# Install Stripe plugin
nself plugin install stripe
```

This will:
- Download the plugin from the registry
- Install Node.js dependencies
- Prepare the plugin for initialization

Expected output:
```
Installing plugin: stripe v1.0.0...
Downloading from https://plugins.nself.org/...
Installing dependencies...
Plugin installed successfully!

Next steps:
  1. Configure environment variables in .env
  2. Run: nself plugin stripe init
  3. Run: nself plugin stripe sync
```

### Step 2: Verify Installation

```bash
# Check installed plugins
nself plugin list --installed
```

Expected output:
```
Installed Plugins:

stripe (v1.0.0)
  Status: Configured
  Tables: 21
  Webhooks: 70+
  Location: ~/.nself/plugins/stripe
```

### Step 3: Initialize Database Schema

The plugin needs to create database tables:

```bash
nself plugin stripe init
```

This command will:
1. Connect to your PostgreSQL database
2. Create 21 tables for Stripe data
3. Create indexes for efficient queries
4. Create 6 analytics views
5. Set up webhook event tracking

Expected output:
```
Initializing Stripe plugin...

Creating database schema...
  ✓ Created table: stripe_customers
  ✓ Created table: stripe_products
  ✓ Created table: stripe_prices
  ✓ Created table: stripe_subscriptions
  ✓ Created table: stripe_subscription_items
  ✓ Created table: stripe_invoices
  ✓ Created table: stripe_invoice_items
  ✓ Created table: stripe_payment_intents
  ✓ Created table: stripe_payment_methods
  ✓ Created table: stripe_charges
  ✓ Created table: stripe_refunds
  ✓ Created table: stripe_disputes
  ✓ Created table: stripe_balance_transactions
  ✓ Created table: stripe_payouts
  ✓ Created table: stripe_coupons
  ✓ Created table: stripe_promotion_codes
  ✓ Created table: stripe_tax_rates
  ✓ Created table: stripe_setup_intents
  ✓ Created table: stripe_checkout_sessions
  ✓ Created table: stripe_events
  ✓ Created table: stripe_webhook_events

Creating indexes...
  ✓ Created 45 indexes

Creating analytics views...
  ✓ stripe_mrr_by_month
  ✓ stripe_customer_lifetime_value
  ✓ stripe_subscription_metrics
  ✓ stripe_revenue_by_product
  ✓ stripe_churn_analysis
  ✓ stripe_payment_success_rate

Database initialization complete!
```

### Step 4: Verify Configuration

Check that everything is set up correctly:

```bash
nself plugin stripe status
```

Expected output:
```
Stripe Plugin Status

Configuration:
  ✓ API Key: Configured (sk_test_*****abc123)
  ✓ Webhook Secret: Configured
  ✓ Database: Connected
  ✓ API Version: 2024-12-18.acacia

Database:
  Tables: 21/21 initialized
  Views: 6/6 created
  Indexes: 45/45 created

Server:
  Status: Stopped
  Port: 3001 (when started)
  Webhook Path: /webhook

Last Sync:
  Status: Never synced
  Records: 0
```

---

## Running Your First Sync

Now let's sync your Stripe data to the database.

### Step 1: Understand Sync Types

nself plugins support two sync modes:

1. **Full Sync**: Fetches all data from Stripe (use for first sync)
2. **Incremental Sync**: Only fetches data that changed since last sync

### Step 2: Run Full Sync

```bash
# Start full sync
nself plugin stripe sync
```

This will fetch all your Stripe data. Depending on your account size, this could take 1-10 minutes.

Expected output:
```
Starting Stripe data sync...

Syncing customers...
  ✓ Fetched 245 customers
  ✓ Synced 245 records

Syncing products...
  ✓ Fetched 12 products
  ✓ Synced 12 records

Syncing prices...
  ✓ Fetched 28 prices
  ✓ Synced 28 records

Syncing subscriptions...
  ✓ Fetched 87 subscriptions
  ✓ Synced 87 records

Syncing invoices...
  ✓ Fetched 432 invoices
  ✓ Synced 432 records

Syncing payment intents...
  ✓ Fetched 156 payment intents
  ✓ Synced 156 records

Syncing payment methods...
  ✓ Fetched 198 payment methods
  ✓ Synced 198 records

Syncing charges...
  ✓ Fetched 523 charges
  ✓ Synced 523 records

Syncing refunds...
  ✓ Fetched 23 refunds
  ✓ Synced 23 records

[... additional resources ...]

Sync completed successfully!

Summary:
  Duration: 4m 32s
  Resources synced: 21
  Total records: 1,847
  Database size: 12.4 MB
```

### Step 3: Verify Synced Data

Check your database directly:

```bash
# Connect to PostgreSQL
psql $DATABASE_URL

# Count customers
SELECT COUNT(*) FROM stripe_customers;

# View recent subscriptions
SELECT id, customer_id, status, current_period_end
FROM stripe_subscriptions
ORDER BY created_at DESC
LIMIT 5;

# Exit psql
\q
```

Or use the plugin CLI:

```bash
# View customer count
nself plugin stripe customers count

# List recent customers
nself plugin stripe customers list --limit 5

# View subscription stats
nself plugin stripe subscriptions stats
```

### Step 4: Schedule Incremental Syncs

For ongoing syncs, use incremental mode:

```bash
# Sync only changed data (much faster)
nself plugin stripe sync --incremental
```

To automate this, add to your crontab:

```bash
# Edit crontab
crontab -e

# Add this line to sync every hour
0 * * * * cd /path/to/my-nself-project && nself plugin stripe sync --incremental
```

---

## Setting Up Webhooks

Webhooks provide real-time updates when data changes in Stripe. This keeps your database in sync without polling.

### Step 1: Start the Webhook Server

```bash
# Start the plugin server in the background
nself plugin stripe server --port 3001 &

# Or use a process manager like PM2
npm install -g pm2
pm2 start "nself plugin stripe server --port 3001" --name stripe-server
```

Expected output:
```
Starting Stripe plugin server...

Server configuration:
  Port: 3001
  Host: 0.0.0.0
  Webhook path: /webhook

Routes initialized:
  POST /webhook - Stripe webhook handler
  POST /api/sync - Manual sync trigger
  GET  /api/status - Server status
  GET  /api/customers - List customers
  GET  /api/subscriptions - List subscriptions
  GET  /api/invoices - List invoices
  [... more endpoints ...]

Server listening on http://0.0.0.0:3001
Ready to receive webhooks!
```

### Step 2: Expose Your Local Server (Development)

For local development, you need to expose your server to the internet. Use one of these tools:

**Option A: ngrok (Recommended)**

```bash
# Install ngrok
brew install ngrok

# Expose port 3001
ngrok http 3001
```

You'll get a URL like: `https://abc123.ngrok.io`

**Option B: localtunnel**

```bash
# Install
npm install -g localtunnel

# Expose port 3001
lt --port 3001
```

**Option C: cloudflared (Cloudflare Tunnel)**

```bash
# Install
brew install cloudflared

# Create tunnel
cloudflared tunnel --url http://localhost:3001
```

### Step 3: Configure Stripe Webhook

1. Go to [Stripe Dashboard > Webhooks](https://dashboard.stripe.com/test/webhooks)
2. Click **"Add endpoint"**
3. Enter your webhook URL: `https://abc123.ngrok.io/webhook`
4. Select **"Select all events"** (or choose specific events)
5. Click **"Add endpoint"**
6. Copy the **Signing secret** (starts with `whsec_`)
7. Add to your `.env` file:

```bash
STRIPE_WEBHOOK_SECRET=whsec_YOUR_SIGNING_SECRET_HERE
```

8. Restart the server to pick up the new secret:

```bash
# If running in background
pkill -f "stripe server"
nself plugin stripe server --port 3001 &

# If using PM2
pm2 restart stripe-server
```

### Step 4: Test Webhook Delivery

1. In Stripe Dashboard, click **"Send test webhook"**
2. Select an event type (e.g., `customer.created`)
3. Click **"Send test webhook"**

Check your server logs:

```bash
# View logs (if using PM2)
pm2 logs stripe-server

# Or check database
psql $DATABASE_URL
SELECT * FROM stripe_webhook_events ORDER BY created_at DESC LIMIT 1;
```

Expected log output:
```
[2026-01-30 10:15:23] INFO: Webhook received: customer.created
[2026-01-30 10:15:23] INFO: Signature verified successfully
[2026-01-30 10:15:23] INFO: Processing event: evt_test_123
[2026-01-30 10:15:23] INFO: Customer created: cus_test_456
[2026-01-30 10:15:23] INFO: Synced customer to database
[2026-01-30 10:15:23] INFO: Event processed successfully
```

### Step 5: Production Webhook Setup

For production, deploy your server to a permanent location:

```bash
# Example: Deploy to a VPS or cloud provider
# Your webhook URL would be something like:
https://your-domain.com/webhook

# Or use a subdomain:
https://webhooks.your-domain.com/stripe
```

Update your Stripe webhook endpoint with the production URL.

---

## Querying Your Data

Now that your data is synced, let's explore different ways to query it.

### Method 1: Direct SQL

The most powerful way to query your data:

```bash
# Connect to database
psql $DATABASE_URL
```

**Example Queries:**

```sql
-- Get customer count
SELECT COUNT(*) FROM stripe_customers;

-- Find high-value customers
SELECT
    id,
    email,
    name,
    metadata->>'lifetime_value' as ltv
FROM stripe_customers
WHERE (metadata->>'lifetime_value')::numeric > 1000
ORDER BY (metadata->>'lifetime_value')::numeric DESC
LIMIT 10;

-- Monthly recurring revenue
SELECT
    DATE_TRUNC('month', created_at) as month,
    COUNT(*) as subscription_count,
    SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END) as active_count,
    SUM((metadata->>'amount')::numeric / 100.0) as mrr
FROM stripe_subscriptions
GROUP BY month
ORDER BY month DESC;

-- Recent failed payments
SELECT
    pi.id,
    pi.amount / 100.0 as amount,
    pi.currency,
    pi.status,
    pi.last_payment_error_message,
    c.email as customer_email,
    pi.created_at
FROM stripe_payment_intents pi
JOIN stripe_customers c ON pi.customer_id = c.id
WHERE pi.status = 'requires_payment_method'
ORDER BY pi.created_at DESC
LIMIT 10;

-- Churn analysis
SELECT
    DATE_TRUNC('month', canceled_at) as month,
    COUNT(*) as churned_subscriptions,
    SUM((metadata->>'amount')::numeric / 100.0) as lost_mrr
FROM stripe_subscriptions
WHERE canceled_at IS NOT NULL
GROUP BY month
ORDER BY month DESC;
```

### Method 2: REST API

The plugin server provides a REST API:

```bash
# Get server status
curl http://localhost:3001/api/status

# List customers (paginated)
curl http://localhost:3001/api/customers?limit=10&offset=0

# Get specific customer
curl http://localhost:3001/api/customers/cus_test_123

# List active subscriptions
curl http://localhost:3001/api/subscriptions?status=active

# Get recent invoices
curl http://localhost:3001/api/invoices?limit=20

# Trigger manual sync
curl -X POST http://localhost:3001/api/sync

# Get webhook stats
curl http://localhost:3001/api/webhooks/stats
```

**Example with jq (for pretty output):**

```bash
# Install jq if needed
brew install jq

# Pretty-print customers
curl -s http://localhost:3001/api/customers | jq '.'

# Extract specific fields
curl -s http://localhost:3001/api/customers | jq '.data[] | {id, email, name}'

# Count total customers
curl -s http://localhost:3001/api/customers | jq '.total'
```

### Method 3: Plugin CLI

The plugin provides CLI commands for common queries:

```bash
# Customer commands
nself plugin stripe customers list
nself plugin stripe customers list --limit 10
nself plugin stripe customers get cus_test_123
nself plugin stripe customers search --email "user@example.com"

# Subscription commands
nself plugin stripe subscriptions list
nself plugin stripe subscriptions list --status active
nself plugin stripe subscriptions stats

# Invoice commands
nself plugin stripe invoices list
nself plugin stripe invoices list --status paid
nself plugin stripe invoices get inv_test_123

# Payment commands
nself plugin stripe payments list
nself plugin stripe payments failed

# Analytics commands
nself plugin stripe analytics mrr
nself plugin stripe analytics churn
nself plugin stripe analytics ltv
```

### Method 4: Analytics Views

The plugin creates pre-built SQL views for common metrics:

```sql
-- Monthly recurring revenue by month
SELECT * FROM stripe_mrr_by_month;

-- Customer lifetime value
SELECT * FROM stripe_customer_lifetime_value
WHERE lifetime_value > 1000
ORDER BY lifetime_value DESC;

-- Subscription metrics
SELECT * FROM stripe_subscription_metrics;

-- Revenue by product
SELECT * FROM stripe_revenue_by_product;

-- Churn analysis
SELECT * FROM stripe_churn_analysis;

-- Payment success rate
SELECT * FROM stripe_payment_success_rate;
```

---

## Common Issues

Here are solutions to common problems you might encounter.

### Issue 1: Database Connection Failed

**Error:**
```
Error: Connection to database failed
Could not connect to PostgreSQL at localhost:5432
```

**Solutions:**

1. **Check PostgreSQL is running:**
   ```bash
   # Docker
   docker ps | grep postgres

   # Local install
   brew services list | grep postgresql
   # or
   sudo systemctl status postgresql
   ```

2. **Verify connection string:**
   ```bash
   # Test connection
   psql $DATABASE_URL

   # If that fails, try connecting with details:
   psql -h localhost -U nself -d nself
   ```

3. **Check firewall:**
   ```bash
   # Make sure port 5432 is accessible
   nc -zv localhost 5432
   ```

4. **Check DATABASE_URL format:**
   ```bash
   # Should be: postgresql://user:password@host:port/database
   echo $DATABASE_URL
   ```

### Issue 2: Invalid API Key

**Error:**
```
Error: Invalid API key provided
Stripe returned: No such API key: sk_test_invalid
```

**Solutions:**

1. **Verify API key in .env:**
   ```bash
   # Check your .env file
   grep STRIPE_API_KEY .env
   ```

2. **Get fresh key from Stripe:**
   - Go to [Dashboard > Developers > API Keys](https://dashboard.stripe.com/test/apikeys)
   - Copy the "Secret key" (starts with `sk_test_` or `sk_live_`)
   - Update `.env` file

3. **Restart the plugin:**
   ```bash
   # If server is running, restart it
   pkill -f "stripe server"
   nself plugin stripe server --port 3001 &
   ```

### Issue 3: Webhook Signature Verification Failed

**Error:**
```
Error: Webhook signature verification failed
No valid signature found for expected signature
```

**Solutions:**

1. **Check webhook secret:**
   ```bash
   # Verify it's set
   grep STRIPE_WEBHOOK_SECRET .env
   ```

2. **Get correct secret from Stripe:**
   - Go to [Dashboard > Webhooks](https://dashboard.stripe.com/test/webhooks)
   - Click on your endpoint
   - Click "Reveal" next to "Signing secret"
   - Copy the secret (starts with `whsec_`)
   - Update `.env` file

3. **Restart server to pick up changes:**
   ```bash
   pkill -f "stripe server"
   nself plugin stripe server --port 3001 &
   ```

### Issue 4: Port Already in Use

**Error:**
```
Error: Port 3001 is already in use
```

**Solutions:**

1. **Find what's using the port:**
   ```bash
   lsof -i :3001
   ```

2. **Kill the process:**
   ```bash
   # Get PID from lsof output, then:
   kill -9 <PID>
   ```

3. **Or use a different port:**
   ```bash
   nself plugin stripe server --port 3002
   ```

### Issue 5: Sync Taking Too Long

**Problem:** Initial sync is taking hours for large Stripe accounts.

**Solutions:**

1. **Use incremental sync after first sync:**
   ```bash
   nself plugin stripe sync --incremental
   ```

2. **Sync specific resources only:**
   ```bash
   nself plugin stripe sync --resources customers,subscriptions
   ```

3. **Increase rate limit (if you have higher limits):**
   ```bash
   # Edit plugin config
   # In plugins/stripe/ts/src/config.ts
   # Increase rateLimitPerSecond value
   ```

4. **Run sync during off-hours:**
   ```bash
   # Schedule via cron for nighttime
   0 2 * * * cd /path/to/project && nself plugin stripe sync
   ```

### Issue 6: Missing Tables After Init

**Problem:** `stripe_customers` table doesn't exist after running `init`.

**Solutions:**

1. **Check database connection during init:**
   ```bash
   nself plugin stripe init --verbose
   ```

2. **Verify tables were created:**
   ```bash
   psql $DATABASE_URL -c "\dt stripe_*"
   ```

3. **Re-run initialization:**
   ```bash
   nself plugin stripe init --force
   ```

4. **Check PostgreSQL permissions:**
   ```bash
   # Make sure user can create tables
   psql $DATABASE_URL -c "CREATE TABLE test_table (id INT);"
   psql $DATABASE_URL -c "DROP TABLE test_table;"
   ```

### Issue 7: Environment Variables Not Loading

**Problem:** `.env` file exists but variables aren't being read.

**Solutions:**

1. **Check file location:**
   ```bash
   # .env should be in project root
   ls -la .env
   ```

2. **Verify file format:**
   ```bash
   # Should not have spaces around =
   # GOOD: STRIPE_API_KEY=sk_test_123
   # BAD:  STRIPE_API_KEY = sk_test_123
   cat .env
   ```

3. **Load manually and test:**
   ```bash
   # Load env vars
   export $(cat .env | xargs)

   # Test
   echo $STRIPE_API_KEY
   ```

4. **Use .env in specific directory:**
   ```bash
   # Run from project directory
   cd /path/to/my-nself-project
   nself plugin stripe status
   ```

---

## Next Steps

Congratulations! You've successfully installed your first nself plugin, synced data, and set up webhooks.

### Learn More

**Plugin Documentation:**
- [Stripe Plugin Guide](../plugins/Stripe.md) - Complete Stripe plugin reference
- [GitHub Plugin Guide](../plugins/GitHub.md) - Sync your GitHub data
- [Shopify Plugin Guide](../plugins/Shopify.md) - E-commerce data sync

**Development:**
- [Plugin Development Guide](../DEVELOPMENT.md) - Create your own plugins
- [TypeScript Plugin Guide](../TYPESCRIPT_PLUGIN_GUIDE.md) - TypeScript best practices
- [Contributing Guide](../CONTRIBUTING.md) - Contribute to nself plugins

**Advanced Topics:**
- [Architecture Overview](../architecture/Overview.md) - How plugins work internally
- [Security Best Practices](../Security.md) - Secure your deployments
- [API Reference](../api/REST-API.md) - Complete API documentation

### Install More Plugins

```bash
# GitHub plugin for repository data
nself plugin install github

# Shopify plugin for e-commerce
nself plugin install shopify
```

### Join the Community

- **GitHub**: [github.com/acamarata/nself-plugins](https://github.com/acamarata/nself-plugins)
- **Issues**: [Report bugs or request features](https://github.com/acamarata/nself-plugins/issues)
- **Discussions**: [Ask questions](https://github.com/acamarata/nself-plugins/discussions)

### Production Deployment

When you're ready for production:

1. **Use production API keys** - Switch from `sk_test_` to `sk_live_`
2. **Deploy to a server** - Use a VPS, cloud provider, or container orchestration
3. **Set up monitoring** - Use logs and metrics to track health
4. **Configure backups** - Regular PostgreSQL backups
5. **Use SSL/TLS** - Secure your webhook endpoints
6. **Implement rate limiting** - Protect your API endpoints

See [Deployment Guide](../guides/Deployment.md) for detailed instructions.

---

## Questions?

If you encounter issues not covered here:

1. Check [Troubleshooting Guide](../troubleshooting/Common-Issues.md)
2. Search [existing issues](https://github.com/acamarata/nself-plugins/issues)
3. Open a [new issue](https://github.com/acamarata/nself-plugins/issues/new) with:
   - Plugin name and version
   - Error message (full output)
   - Steps to reproduce
   - Your environment (OS, Node version, PostgreSQL version)

---

**Happy syncing!**

*Last Updated: January 30, 2026*
*For nself v0.4.8+*
