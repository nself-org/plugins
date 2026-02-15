# ddns

Dynamic DNS updater with multi-provider support and external IP monitoring

## Installation

```bash
nself plugin install ddns
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself backend `.env.dev`, map variables as follows:

### Backend -> Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `DDNS_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `DDNS_PLUGIN_PORT` | `PORT` or `DDNS_PLUGIN_PORT` | Server port | `3217` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `DDNS_PROVIDER` | `DDNS_PROVIDER` | DNS provider name | `duckdns` |
| `DDNS_DOMAIN` | `DDNS_DOMAIN` | Domain to update | `myhost.duckdns.org` |
| `DDNS_TOKEN` | `DDNS_TOKEN` | Provider API token | `abc123...` |
| `DDNS_CHECK_INTERVAL` | `DDNS_CHECK_INTERVAL` | IP check interval in seconds | `300` |
| `DDNS_CLOUDFLARE_API_KEY` | `DDNS_CLOUDFLARE_API_KEY` | Cloudflare API key | `cf_key_...` |
| `DDNS_CLOUDFLARE_ZONE_ID` | `DDNS_CLOUDFLARE_ZONE_ID` | Cloudflare zone ID | `zone123...` |

## Supported Providers

- **DuckDNS** - Free dynamic DNS (duckdns.org)
- **Cloudflare** - DNS management via Cloudflare API
- **No-IP** - Dynamic DNS service (noip.com)
- **Dynu** - Free dynamic DNS (dynu.com)

## Usage

See plugin.json for available CLI commands and API endpoints.

## License

See LICENSE file in repository root.
