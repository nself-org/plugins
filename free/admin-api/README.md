# admin-api

Admin API service providing aggregated metrics, system health, session counts, storage breakdown, and real-time dashboard endpoints

## Installation

```bash
nself plugin install admin-api
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using an nself backend `.env.dev`, map variables as follows:

### Backend -> Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `ADMIN_API_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `ADMIN_API_PLUGIN_PORT` | `PORT` or `ADMIN_API_PLUGIN_PORT` | Server port | `3212` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `PROMETHEUS_URL` | `PROMETHEUS_URL` | Prometheus server URL | `http://localhost:9090` |
| `ADMIN_API_CACHE_TTL` | `ADMIN_API_CACHE_TTL` | Cache TTL in seconds | `30` |
| `ADMIN_API_WS_ENABLED` | `ADMIN_API_WS_ENABLED` | Enable WebSocket support | `true` |

## Usage

See plugin.json for available CLI commands and API endpoints.

## License

See LICENSE file in repository root.
