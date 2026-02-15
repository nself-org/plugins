# mdns

mDNS/Bonjour service discovery for zero-config LAN advertising

## Installation

```bash
nself plugin install mdns
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself backend `.env.dev`, map variables as follows:

### Backend -> Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `MDNS_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `MDNS_PLUGIN_PORT` | `PORT` or `MDNS_PLUGIN_PORT` | Server port | `3216` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `MDNS_SERVICE_TYPE` | `MDNS_SERVICE_TYPE` | Default mDNS service type | `_ntv._tcp` |
| `MDNS_INSTANCE_NAME` | `MDNS_INSTANCE_NAME` | Instance name for advertising | `nself-server` |
| `MDNS_DOMAIN` | `MDNS_DOMAIN` | mDNS domain | `local` |

## Usage

See plugin.json for available CLI commands and API endpoints.

## License

See LICENSE file in repository root.
