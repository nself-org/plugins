# observability

Unified observability service with health probes, watchdog timers, service auto-discovery, and systemd integration

## Installation

```bash
nself plugin install observability
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself backend `.env.dev`, map variables as follows:

### Backend -> Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `OBSERVABILITY_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `OBSERVABILITY_PLUGIN_PORT` | `PORT` or `OBSERVABILITY_PLUGIN_PORT` | Server port | `3215` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `OBSERVABILITY_CHECK_INTERVAL` | `OBSERVABILITY_CHECK_INTERVAL` | Health check interval in seconds | `30` |
| `OBSERVABILITY_DOCKER_SOCKET` | `OBSERVABILITY_DOCKER_SOCKET` | Docker socket path | `/var/run/docker.sock` |
| `OBSERVABILITY_WATCHDOG_ENABLED` | `OBSERVABILITY_WATCHDOG_ENABLED` | Enable watchdog timers | `true` |

## Usage

See plugin.json for available CLI commands and API endpoints.

## License

See LICENSE file in repository root.
