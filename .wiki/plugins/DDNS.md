# DDNS Plugin

Dynamic DNS updater with multi-provider support and external IP monitoring. Automatically detects your public IP address changes and updates DNS records across multiple DDNS providers.

| Property | Value |
|----------|-------|
| **Port** | `3217` |
| **Category** | `networking` |
| **Multi-App** | `source_account_id` (UUID) |
| **Min nself** | `0.4.8` |

---

## Quick Start

```bash
nself plugin run ddns init
nself plugin run ddns server
```

---

## Features

- **Multi-Provider Support** - Works with DuckDNS, Cloudflare, No-IP, and Dynu
- **Automatic IP Detection** - Monitors external IP changes automatically
- **Update Scheduling** - Configurable check intervals (default: 5 minutes)
- **Update History** - Tracks all DNS update attempts with success/failure status
- **Provider Flexibility** - Easy to add new DDNS providers
- **Reliable Updates** - Retry logic and error handling for network failures

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DDNS_PLUGIN_PORT` | `3217` | Server port |
| `DDNS_PROVIDER` | - | DNS provider name (`duckdns`, `cloudflare`, `noip`, `dynu`) |
| `DDNS_DOMAIN` | - | Domain/hostname to update |
| `DDNS_TOKEN` | - | Provider API token/key |
| `DDNS_CHECK_INTERVAL` | `300` | IP check interval in seconds (5 minutes) |
| `DDNS_CLOUDFLARE_API_KEY` | - | Cloudflare API key (if using Cloudflare provider) |
| `DDNS_CLOUDFLARE_ZONE_ID` | - | Cloudflare zone ID (if using Cloudflare provider) |

### Supported Providers

| Provider | Type | Configuration Required |
|----------|------|------------------------|
| `duckdns` | Free DDNS | `DDNS_DOMAIN`, `DDNS_TOKEN` |
| `cloudflare` | DNS Service | `DDNS_DOMAIN`, `DDNS_CLOUDFLARE_API_KEY`, `DDNS_CLOUDFLARE_ZONE_ID` |
| `noip` | Free/Paid DDNS | `DDNS_DOMAIN`, `DDNS_TOKEN` |
| `dynu` | Free DDNS | `DDNS_DOMAIN`, `DDNS_TOKEN` |

---

## Installation

```bash
# Install plugin
nself plugin install ddns

# Initialize database
nself plugin run ddns init

# Start server
nself plugin run ddns server
```

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize database schema (2 tables) |
| `server` | Start the HTTP API server (`-p`/`--port`) |
| `status` | Show current IP address and domain status |
| `update` | Force an immediate DNS update |
| `providers` | List available DNS providers |
| `history` | Show DNS update history (`--limit`) |
| `stats` | Show DDNS statistics (total updates, success rate, last update) |

---

## REST API

### Health & Status

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/ready` | Readiness check (DB) |

### DDNS Management

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/ddns/status` | Get current IP and configured domains |
| `POST` | `/api/ddns/update` | Force DNS update (body: `provider?`, `domain?`, `ip?`) |
| `GET` | `/api/ddns/providers` | List available providers with configuration status |
| `GET` | `/api/ddns/history` | Get update history (query: `limit?`, `offset?`, `provider?`, `status?`) |
| `GET` | `/api/ddns/config` | Get current configuration |
| `PUT` | `/api/ddns/config` | Update configuration (body: `provider?`, `domain?`, `token?`, `check_interval?`) |

---

## Webhook Events

| Event | Description |
|-------|-------------|
| `ddns.ip.detected` | External IP address detected |
| `ddns.ip.changed` | IP address changed from previous value |
| `ddns.update.started` | DNS update initiated |
| `ddns.update.success` | DNS update succeeded |
| `ddns.update.failed` | DNS update failed |
| `ddns.config.updated` | Configuration changed |

---

## Database Schema

### `np_ddns_config`

Stores DDNS provider configuration.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Configuration ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `provider` | `VARCHAR(50)` | Provider name (`duckdns`, `cloudflare`, etc.) |
| `domain` | `VARCHAR(255)` | Domain/hostname to update |
| `token_encrypted` | `TEXT` | Encrypted API token/key |
| `check_interval_seconds` | `INTEGER` | IP check interval |
| `last_known_ip` | `VARCHAR(45)` | Last detected IP address |
| `last_update_at` | `TIMESTAMPTZ` | Last successful update timestamp |
| `is_enabled` | `BOOLEAN` | Whether this config is active |
| `metadata` | `JSONB` | Provider-specific configuration |
| `created_at` | `TIMESTAMPTZ` | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | Last update |

### `np_ddns_update_log`

Tracks all DNS update attempts.

| Column | Type | Description |
|--------|------|-------------|
| `id` | `UUID` (PK) | Log entry ID |
| `source_account_id` | `VARCHAR(128)` | Multi-app isolation |
| `config_id` | `UUID` (FK) | References `np_ddns_config` |
| `provider` | `VARCHAR(50)` | Provider name |
| `domain` | `VARCHAR(255)` | Domain updated |
| `old_ip` | `VARCHAR(45)` | Previous IP address |
| `new_ip` | `VARCHAR(45)` | New IP address |
| `status` | `VARCHAR(20)` | `success`, `failed`, `skipped` |
| `error_message` | `TEXT` | Error details if failed |
| `response_time_ms` | `INTEGER` | Provider response time |
| `created_at` | `TIMESTAMPTZ` | Update attempt timestamp |

---

## Usage Examples

### DuckDNS Configuration

```bash
# Configure environment
export DDNS_PROVIDER=duckdns
export DDNS_DOMAIN=myhost.duckdns.org
export DDNS_TOKEN=your-duckdns-token
export DDNS_CHECK_INTERVAL=300

# Initialize and start
nself plugin run ddns init
nself plugin run ddns server
```

### Cloudflare Configuration

```bash
# Configure environment
export DDNS_PROVIDER=cloudflare
export DDNS_DOMAIN=home.example.com
export DDNS_CLOUDFLARE_API_KEY=your-cloudflare-api-key
export DDNS_CLOUDFLARE_ZONE_ID=your-zone-id
export DDNS_CHECK_INTERVAL=600

# Initialize and start
nself plugin run ddns init
nself plugin run ddns server
```

### API Usage

```bash
# Check current status
curl http://localhost:3217/api/ddns/status

# Force immediate update
curl -X POST http://localhost:3217/api/ddns/update

# View update history
curl http://localhost:3217/api/ddns/history?limit=10

# Get provider list
curl http://localhost:3217/api/ddns/providers
```

### CLI Usage

```bash
# Check current status
nself plugin run ddns status

# Force update
nself plugin run ddns update

# View update history (last 20)
nself plugin run ddns history --limit 20

# Show statistics
nself plugin run ddns stats
```

---

## How It Works

1. **IP Detection** - Plugin periodically checks your external IP using public IP detection services
2. **Change Detection** - Compares current IP with last known IP from database
3. **DNS Update** - When IP changes, calls the provider's API to update DNS record
4. **Logging** - Records all update attempts with status, timing, and error details
5. **Scheduling** - Runs check cycle every `DDNS_CHECK_INTERVAL` seconds

### Update Flow

```
[Timer Tick] → [Detect External IP] → [Compare with Last Known IP]
                                              ↓
                                         [IP Changed?]
                                              ↓
                                   [Call Provider API] → [Update DNS Record]
                                              ↓
                                   [Log Update Result] → [Store New IP]
```

---

## Provider Setup Guides

### DuckDNS

1. Create account at [duckdns.org](https://www.duckdns.org)
2. Create a subdomain (e.g., `myhost.duckdns.org`)
3. Copy your token from the DuckDNS dashboard
4. Configure plugin:
   ```bash
   DDNS_PROVIDER=duckdns
   DDNS_DOMAIN=myhost.duckdns.org
   DDNS_TOKEN=your-token-here
   ```

### Cloudflare

1. Log in to Cloudflare dashboard
2. Get your API key from profile settings
3. Find your Zone ID for the domain
4. Configure plugin:
   ```bash
   DDNS_PROVIDER=cloudflare
   DDNS_DOMAIN=home.example.com
   DDNS_CLOUDFLARE_API_KEY=your-api-key
   DDNS_CLOUDFLARE_ZONE_ID=your-zone-id
   ```

### No-IP

1. Create account at [noip.com](https://www.noip.com)
2. Create a hostname
3. Generate an update token
4. Configure plugin:
   ```bash
   DDNS_PROVIDER=noip
   DDNS_DOMAIN=myhost.ddns.net
   DDNS_TOKEN=your-token
   ```

### Dynu

1. Create account at [dynu.com](https://www.dynu.com)
2. Create a DDNS hostname
3. Get API credentials
4. Configure plugin:
   ```bash
   DDNS_PROVIDER=dynu
   DDNS_DOMAIN=myhost.dynu.net
   DDNS_TOKEN=your-token
   ```

---

## Troubleshooting

**"Provider not configured"** -- Verify `DDNS_PROVIDER` is set to a supported provider (`duckdns`, `cloudflare`, `noip`, `dynu`).

**"Token not found"** -- Ensure the appropriate token environment variable is set. DuckDNS/No-IP/Dynu use `DDNS_TOKEN`, Cloudflare uses `DDNS_CLOUDFLARE_API_KEY`.

**"Update failed" in logs** -- Check the `error_message` in the update log table. Common issues: invalid token, domain not found, rate limiting, network errors.

**IP not updating** -- Verify `DDNS_CHECK_INTERVAL` is set (default 300 seconds). Check server logs for errors. Ensure your network allows outbound HTTPS to the provider API.

**Updates too frequent** -- Increase `DDNS_CHECK_INTERVAL`. Default is 300 seconds (5 minutes). Most providers recommend 5-10 minute intervals.

**Cloudflare updates not working** -- Verify both `DDNS_CLOUDFLARE_API_KEY` and `DDNS_CLOUDFLARE_ZONE_ID` are set correctly. The zone ID can be found in the Cloudflare dashboard for your domain.

**Multiple domains** -- Currently one configuration per account. For multiple domains, configure separate accounts or manually call the API with different domains using `POST /api/ddns/update`.

---

## Security Notes

- API tokens are encrypted at rest in the database
- Use environment variables for secrets, never hardcode tokens
- Cloudflare API keys should have minimal permissions (Zone:DNS:Edit only)
- Monitor update logs for suspicious activity
- Consider using Cloudflare API tokens (more secure than global API keys)

---

## Performance

- Minimal resource usage (IP checks are lightweight HTTP requests)
- Database writes only on IP changes or configuration updates
- Configurable check intervals to balance responsiveness vs. API quota usage
- Provider response times typically 100-500ms

---

## Advanced Configuration

### Using with nself Backend

Add to your `.env.dev`:

```bash
# Enable DDNS plugin
DDNS_PLUGIN_ENABLED=true
DDNS_PLUGIN_PORT=3217

# Configure provider
DDNS_PROVIDER=duckdns
DDNS_DOMAIN=myhost.duckdns.org
DDNS_TOKEN=your-token
DDNS_CHECK_INTERVAL=300
```

### Custom Check Intervals

```bash
# Check every minute (fast, for testing)
DDNS_CHECK_INTERVAL=60

# Check every 10 minutes (recommended for most use cases)
DDNS_CHECK_INTERVAL=600

# Check every hour (for stable IPs)
DDNS_CHECK_INTERVAL=3600
```

### Multiple Provider Failover

While the plugin configuration supports one provider at a time, you can use the API to manually update multiple providers:

```bash
# Update DuckDNS
curl -X POST http://localhost:3217/api/ddns/update \
  -H "Content-Type: application/json" \
  -d '{"provider":"duckdns","domain":"myhost.duckdns.org"}'

# Update Cloudflare
curl -X POST http://localhost:3217/api/ddns/update \
  -H "Content-Type: application/json" \
  -d '{"provider":"cloudflare","domain":"home.example.com"}'
```
