# Multi-App Support Guide

**Last Updated**: February 10, 2026
**Status**: Production Ready (v1.0.0)

---

## Overview

All nself plugins support **multi-app isolation**, allowing a single nself backend to serve multiple frontend applications with complete data separation. Each app's data is isolated via the `source_account_id` column.

### Key Concepts

- **Single Backend, Multiple Apps**: One PostgreSQL database, one set of plugin containers
- **Data Isolation**: Every row has a `source_account_id` column (defaults to `'primary'`)
- **Per-App Routes**: Each app gets its own subdomain for plugin APIs and webhooks
- **Per-App Credentials**: Each app can use different API keys, tokens, and webhook secrets

---

## Quick Start

### 1. Configure Multiple Apps

In your nself `.env` file:

```bash
# Define your frontend apps
FRONTEND_APP_1_SYSTEM_NAME=admin
FRONTEND_APP_2_SYSTEM_NAME=storefront
FRONTEND_APP_3_SYSTEM_NAME=blog
```

### 2. Configure Plugin Accounts

Each plugin uses CSV environment variables for multi-account support:

#### Stripe Example
```bash
# Multi-account CSV pattern
STRIPE_API_KEYS=sk_live_admin_xxx,sk_live_store_yyy
STRIPE_ACCOUNT_LABELS=admin,storefront
STRIPE_WEBHOOK_SECRETS=whsec_admin_xxx,whsec_store_yyy
```

**Critical**: `STRIPE_ACCOUNT_LABELS` values MUST match your `FRONTEND_APP_N_SYSTEM_NAME` values.

#### GitHub Example
```bash
GITHUB_API_KEYS=ghp_admin_token,ghp_blog_token
GITHUB_ACCOUNT_LABELS=admin,blog
GITHUB_WEBHOOK_SECRETS=github_secret_admin,github_secret_blog
```

#### Shopify Example
```bash
SHOPIFY_ACCESS_TOKENS=shpat_admin_xxx,shpat_store_yyy
SHOPIFY_SHOP_DOMAINS=admin-shop.myshopify.com,store.myshopify.com
SHOPIFY_ACCOUNT_LABELS=admin,storefront
SHOPIFY_WEBHOOK_SECRETS=shopify_secret_admin,shopify_secret_store
```

### 3. Plugin Routes

Each app gets dedicated plugin routes:

```
stripe-admin.example.com     → Admin's Stripe data
stripe-storefront.example.com → Storefront's Stripe data
github-admin.example.com     → Admin's GitHub data
github-blog.example.com      → Blog's GitHub data
```

### 4. Webhook Configuration

Configure webhooks in each external service to point to the app-specific URL:

**Stripe Dashboard (Admin Account)**:
- Webhook URL: `https://stripe-admin.example.com/webhook`
- Webhook Secret: Use the first value from `STRIPE_WEBHOOK_SECRETS`

**Stripe Dashboard (Storefront Account)**:
- Webhook URL: `https://stripe-storefront.example.com/webhook`
- Webhook Secret: Use the second value from `STRIPE_WEBHOOK_SECRETS`

---

## Architecture

### Data Isolation

All plugin tables include a `source_account_id` column:

```sql
CREATE TABLE stripe_customers (
    id VARCHAR(255) PRIMARY KEY,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    email VARCHAR(255),
    name VARCHAR(255),
    ...
);

CREATE INDEX idx_stripe_customers_source_account
  ON stripe_customers(source_account_id);
```

### Query Filtering

Every database query automatically filters by account:

```typescript
// Admin app queries admin data only
GET https://stripe-admin.example.com/api/customers
→ SELECT * FROM stripe_customers WHERE source_account_id = 'admin'

// Storefront app queries storefront data only
GET https://stripe-storefront.example.com/api/customers
→ SELECT * FROM stripe_customers WHERE source_account_id = 'storefront'
```

### Primary Key Strategies

Plugins use two PK strategies:

#### Single PK (Most Plugins)
External IDs are globally unique. One app can't conflict with another.

**Examples**: Stripe, PayPal, Donorbox, Jobs, Notifications, File Processing, Realtime, IDme

```sql
CREATE TABLE stripe_customers (
    id VARCHAR(255) PRIMARY KEY,  -- Stripe's cus_xxx IDs are globally unique
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    ...
);
```

#### Composite PK (GitHub, Shopify)
Two apps may track the SAME external resource. IDs can collide.

**Examples**: GitHub (same org), Shopify (same store)

```sql
CREATE TABLE github_repositories (
    id BIGINT NOT NULL,
    source_account_id VARCHAR(128) NOT NULL DEFAULT 'primary',
    full_name VARCHAR(255),
    ...,
    PRIMARY KEY (id, source_account_id)  -- Composite key prevents conflicts
);
```

**Use Case**:
- Admin app tracks `myorg/repo` (id=12345, source_account_id='admin')
- CI app tracks the SAME `myorg/repo` (id=12345, source_account_id='ci')
- Both rows coexist in the database

---

## Per-Plugin Configuration

### Stripe (Single PK)

```bash
# Required
STRIPE_API_KEY=sk_live_xxx          # Fallback for single-app mode

# Multi-app (CSV)
STRIPE_API_KEYS=sk_live_admin,sk_live_store
STRIPE_ACCOUNT_LABELS=admin,storefront
STRIPE_WEBHOOK_SECRETS=whsec_admin,whsec_store
STRIPE_ACCOUNT_ID=primary           # Fallback account ID
```

**Routes**:
- `stripe-admin.example.com/webhook` → Admin Stripe account
- `stripe-admin.example.com/api/*` → Admin data only

### GitHub (Composite PK)

```bash
# Required
GITHUB_TOKEN=ghp_xxx                # Fallback for single-app mode

# Multi-app (CSV)
GITHUB_API_KEYS=ghp_admin_token,ghp_blog_token
GITHUB_ACCOUNT_LABELS=admin,blog
GITHUB_WEBHOOK_SECRETS=gh_secret_admin,gh_secret_blog
```

**Routes**:
- `github-admin.example.com/webhook` → Admin GitHub repos
- `github-blog.example.com/webhook` → Blog GitHub repos

### Shopify (Composite PK)

```bash
# Required
SHOPIFY_ACCESS_TOKEN=shpat_xxx      # Fallback for single-app mode
SHOPIFY_SHOP_DOMAIN=myshop.myshopify.com

# Multi-app (CSV)
SHOPIFY_ACCESS_TOKENS=shpat_admin,shpat_store
SHOPIFY_SHOP_DOMAINS=admin-shop.myshopify.com,store.myshopify.com
SHOPIFY_ACCOUNT_LABELS=admin,storefront
SHOPIFY_WEBHOOK_SECRETS=shopify_secret_admin,shopify_secret_store
```

**Routes**:
- `shopify-admin.example.com/webhook` → Admin shop data
- `shopify-storefront.example.com/webhook` → Storefront shop data

### PayPal (Single PK)

```bash
PAYPAL_CLIENT_ID=xxx
PAYPAL_CLIENT_SECRET=xxx

# Multi-app (CSV)
PAYPAL_CLIENT_IDS=admin_id,store_id
PAYPAL_CLIENT_SECRETS=admin_secret,store_secret
PAYPAL_ACCOUNT_LABELS=admin,storefront
```

### Other Plugins

All other plugins follow the same CSV pattern:
- `{PLUGIN}_API_KEYS` or `{PLUGIN}_TOKENS`
- `{PLUGIN}_ACCOUNT_LABELS`
- `{PLUGIN}_WEBHOOK_SECRETS` (if webhooks supported)

---

## API Usage

### X-App-Name Header

The nginx reverse proxy sets the `X-App-Name` header based on the subdomain:

```http
GET https://stripe-admin.example.com/api/customers
X-App-Name: admin
```

The plugin server reads this header and scopes all queries:

```typescript
const appContext = getAppContext(request);  // { sourceAccountId: 'admin' }
const scopedDb = db.forSourceAccount(appContext.sourceAccountId);
const customers = await scopedDb.listCustomers();  // Only admin's customers
```

### Direct API Calls (Development)

You can also pass the app name as a query parameter:

```http
GET http://localhost:3001/api/customers?app=admin
```

This is useful for local development without nginx.

---

## Single-App Mode (Backward Compatible)

If you DON'T configure multiple apps, everything defaults to `'primary'`:

```bash
# No FRONTEND_APP_N variables → single-app mode
STRIPE_API_KEY=sk_live_xxx
STRIPE_WEBHOOK_SECRET=whsec_xxx
```

All data uses `source_account_id = 'primary'`. The system works exactly as before.

---

## Data Cleanup

### Remove All Data for an App

If you delete an app, clean up its plugin data:

```sql
-- Remove admin app's Stripe data
DELETE FROM stripe_customers WHERE source_account_id = 'admin';
DELETE FROM stripe_subscriptions WHERE source_account_id = 'admin';
-- ... repeat for all tables
```

Or use the plugin's cleanup API:

```http
POST http://localhost:3001/api/cleanup
Content-Type: application/json

{ "sourceAccountId": "admin" }
```

---

## Troubleshooting

### Issue: Webhook signature verification fails

**Cause**: Webhook secret doesn't match the account.

**Solution**: Ensure the webhook secret in the external service dashboard matches the secret in your CSV env vars at the correct index.

```bash
# If admin is first in STRIPE_ACCOUNT_LABELS, use first secret
STRIPE_ACCOUNT_LABELS=admin,storefront
STRIPE_WEBHOOK_SECRETS=whsec_admin_xxx,whsec_store_yyy
                       ^^^^^^^^^^^^^^^^  ← Use this for admin's webhook
```

### Issue: API returns data from wrong app

**Cause**: `X-App-Name` header not set or incorrect.

**Solution**: Check nginx configuration. Each app subdomain should set the correct header:

```nginx
server {
    server_name stripe-admin.example.com;
    location / {
        proxy_pass http://nself-stripe:3001;
        proxy_set_header X-App-Name admin;  # ← Must match app name
    }
}
```

### Issue: Two apps see each other's data

**Cause**: Plugin doesn't support multi-app (shouldn't happen in v1.0.0+).

**Solution**: Verify the plugin's `plugin.json` has a `multiApp` block:

```json
{
  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "pkStrategy": "single"
  }
}
```

### Issue: Composite PK constraint violation

**Cause**: Trying to insert the same ID for two different apps in a single-PK table.

**Solution**: This shouldn't happen. If it does, the plugin may need composite PKs. File an issue.

---

## Security Considerations

1. **API Keys**: Each app should use DIFFERENT API keys to the external service
2. **Webhook Secrets**: Each app should use DIFFERENT webhook secrets
3. **Database Access**: The `source_account_id` column is the ONLY isolation mechanism. Do NOT share database credentials across untrusted parties.
4. **Network Isolation**: Use nginx to enforce per-app routing. Do NOT expose plugin containers directly.

---

## Migration from Single-App

If you're adding a second app to an existing single-app installation:

1. All existing data has `source_account_id = 'primary'`
2. Add new app's credentials to CSV env vars
3. Restart plugin containers
4. New app's data will have `source_account_id = 'newapp'`
5. Existing data remains `'primary'`

You can optionally rename `'primary'` to a real app name:

```sql
UPDATE stripe_customers SET source_account_id = 'admin'
  WHERE source_account_id = 'primary';
```

---

## Reference

- **Plugin List**: See [Home](../Home.md) for all plugins with multi-app support
- **Development Guide**: See [TYPESCRIPT_PLUGIN_GUIDE.md](../TYPESCRIPT_PLUGIN_GUIDE.md) for building multi-app plugins
- **Source Code**: [GitHub Repository](https://github.com/acamarata/nself-plugins)

---

*For technical details on implementing multi-app support in custom plugins, see the internal development guide.*
