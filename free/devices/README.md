# devices

IoT device enrollment, trust management, and command dispatch service.

## Installation

```bash
nself plugin install devices
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `DEVICES_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `DEVICES_PLUGIN_PORT` | `PORT` or `DEV_PLUGIN_PORT` | Server port | `3603` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `REDIS_URL` | `REDIS_URL` | Redis connection URL | `redis://localhost:6379` |
| `DEV_ENROLLMENT_TOKEN_TTL` | `DEV_ENROLLMENT_TOKEN_TTL` | Enrollment token TTL (seconds) | `3600` |
| `DEV_HEARTBEAT_INTERVAL` | `DEV_HEARTBEAT_INTERVAL` | Device heartbeat interval (seconds) | `60` |
| `DEV_HEARTBEAT_TIMEOUT` | `DEV_HEARTBEAT_TIMEOUT` | Heartbeat timeout (seconds) | `180` |
| `DEV_REALTIME_URL` | `DEV_REALTIME_URL` | Realtime service URL | `http://localhost:3101` |
| `DEV_RECORDING_URL` | `DEV_RECORDING_URL` | Recording service URL | `http://localhost:3602` |
| `DEV_STREAM_GATEWAY_URL` | `DEV_STREAM_GATEWAY_URL` | Stream gateway URL | `http://localhost:3601` |
| `DEV_APP_TV_HEARTBEAT_INTERVAL` | `DEV_APP_TV_HEARTBEAT_INTERVAL` | nTV app heartbeat interval | `30` |

### Configuration Helper Script

You can generate the plugin `.env` file from your backend configuration:

```bash
#!/bin/bash
# generate-devices-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/devices/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$DEVICES_PLUGIN_PORT
REDIS_URL=$REDIS_URL

# Device settings
DEV_ENROLLMENT_TOKEN_TTL=${DEV_ENROLLMENT_TOKEN_TTL:-3600}
DEV_HEARTBEAT_INTERVAL=${DEV_HEARTBEAT_INTERVAL:-60}
DEV_HEARTBEAT_TIMEOUT=${DEV_HEARTBEAT_TIMEOUT:-180}
DEV_COMMAND_DEFAULT_TIMEOUT=${DEV_COMMAND_DEFAULT_TIMEOUT:-300}
DEV_COMMAND_MAX_RETRIES=${DEV_COMMAND_MAX_RETRIES:-3}

# External service URLs
DEV_REALTIME_URL=${DEV_REALTIME_URL:-http://localhost:3101}
DEV_RECORDING_URL=${DEV_RECORDING_URL:-http://localhost:3602}
DEV_STREAM_GATEWAY_URL=${DEV_STREAM_GATEWAY_URL:-http://localhost:3601}

# App-specific overrides
DEV_APP_TV_HEARTBEAT_INTERVAL=${DEV_APP_TV_HEARTBEAT_INTERVAL:-30}
DEV_APP_TV_INGEST_HEARTBEAT_INTERVAL=${DEV_APP_TV_INGEST_HEARTBEAT_INTERVAL:-5}

# Logging
LOG_LEVEL=info
EOF

echo "Created $PLUGIN_ENV"
```

See [CONFIGURATION.md](../../CONFIGURATION.md) for detailed mapping patterns and troubleshooting.

## Usage

See plugin.json for available CLI commands and API endpoints.

## License

See LICENSE file in repository root.
