# Plugin Installation

This guide covers installing and configuring nself plugins.

## Requirements

- nself v0.4.8 or later
- Docker and Docker Compose
- Running nself project (`nself start`)

## Installing Plugins

### From Official Registry

```bash
# List available plugins
nself plugin list

# Install a plugin
nself plugin install stripe

# Install specific version
nself plugin install stripe@1.0.0
```

### From Local Path

For development or custom plugins:

```bash
nself plugin install ./path/to/my-plugin
```

### From Git Repository

```bash
nself plugin install https://github.com/user/nself-plugin-custom.git
```

## Configuration

After installing a plugin, you need to configure it.

### 1. Add Environment Variables

Each plugin requires specific environment variables. Add them to your `.env` file:

```bash
# Stripe Plugin
STRIPE_API_KEY=sk_live_xxx
STRIPE_WEBHOOK_SECRET=whsec_xxx

# Shopify Plugin
SHOPIFY_STORE_URL=mystore.myshopify.com
SHOPIFY_ACCESS_TOKEN=shpat_xxx
```

### 2. Apply Database Schema

The plugin schema is applied automatically during installation. To verify:

```bash
nself plugin stripe status
```

### 3. Initial Data Sync

Sync existing data from the service:

```bash
nself plugin stripe sync
```

## Managing Plugins

### Check Status

```bash
# Status of all plugins
nself plugin status

# Status of specific plugin
nself plugin stripe status
```

### Update Plugins

```bash
# Update all plugins
nself plugin update

# Update specific plugin
nself plugin update stripe
```

### Remove Plugins

```bash
# Remove plugin (keeps data)
nself plugin remove stripe

# Remove plugin and data
nself plugin remove stripe --delete-data
```

## Webhook Configuration

Plugins that support webhooks need endpoint configuration in the external service.

### 1. Get Your Webhook URL

Your webhook endpoint is:
```
https://your-domain.com/webhooks/<plugin-name>
```

For local development:
```
https://local.nself.org/webhooks/stripe
```

### 2. Configure in External Service

For Stripe:
1. Go to [Stripe Dashboard > Webhooks](https://dashboard.stripe.com/webhooks)
2. Click "Add endpoint"
3. Enter your webhook URL
4. Select events to listen for
5. Copy the signing secret to `STRIPE_WEBHOOK_SECRET`

### 3. Verify Webhooks

```bash
# Check webhook status
nself plugin stripe webhook status

# View recent events
nself plugin stripe webhook events
```

## Troubleshooting

### Plugin Not Found

```
Error: Plugin 'xyz' not found in registry
```

Check available plugins: `nself plugin list`

### Database Connection Failed

```
Error: Could not connect to PostgreSQL
```

Ensure your nself project is running: `nself status`

### Missing Environment Variables

```
Error: STRIPE_API_KEY is not set
```

Add the required variable to your `.env` file.

### Webhook Signature Failed

```
Error: Webhook signature verification failed
```

Ensure `STRIPE_WEBHOOK_SECRET` matches the signing secret from Stripe Dashboard.

## Next Steps

- [Stripe Plugin Guide](plugins/Stripe.md)
- [Plugin Development](DEVELOPMENT.md)
- [Planned Plugins](PLANNED.md)
