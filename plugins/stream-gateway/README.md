# stream-gateway

Stream admission and governance service.

## Installation

```bash
nself plugin install stream-gateway
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `STREAM_GATEWAY_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `false` |
| `STREAM_GATEWAY_PLUGIN_PORT` | `PORT` | Server port | `3601` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `REDIS_URL` | `REDIS_URL` | Redis connection URL | `redis://localhost:6379` |
| `SG_HEARTBEAT_INTERVAL` | `SG_HEARTBEAT_INTERVAL` | Heartbeat interval (seconds) | `30` |
| `SG_HEARTBEAT_TIMEOUT` | `SG_HEARTBEAT_TIMEOUT` | Heartbeat timeout (seconds) | `90` |
| `SG_DEFAULT_MAX_CONCURRENT` | `SG_DEFAULT_MAX_CONCURRENT` | Default max concurrent viewers | `100` |
| `SG_DEFAULT_MAX_DEVICE_STREAMS` | `SG_DEFAULT_MAX_DEVICE_STREAMS` | Max streams per device | `3` |
| `SG_SESSION_MAX_DURATION_HOURS` | `SG_SESSION_MAX_DURATION_HOURS` | Max session duration | `24` |
| `SG_REALTIME_URL` | `SG_REALTIME_URL` | Realtime service URL | `http://localhost:3101` |
| `SG_APP_TV_MAX_CONCURRENT` | `SG_APP_TV_MAX_CONCURRENT` | nTV max concurrent | `50` |
| `SG_APP_TV_MAX_DEVICE_STREAMS` | `SG_APP_TV_MAX_DEVICE_STREAMS` | nTV max device streams | `2` |

### Configuration Helper Script

```bash
#!/bin/bash
# generate-stream-gateway-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/stream-gateway/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$STREAM_GATEWAY_PLUGIN_PORT
REDIS_URL=$REDIS_URL

# Stream gateway settings
SG_HEARTBEAT_INTERVAL=${SG_HEARTBEAT_INTERVAL:-30}
SG_HEARTBEAT_TIMEOUT=${SG_HEARTBEAT_TIMEOUT:-90}
SG_DEFAULT_MAX_CONCURRENT=${SG_DEFAULT_MAX_CONCURRENT:-100}
SG_DEFAULT_MAX_DEVICE_STREAMS=${SG_DEFAULT_MAX_DEVICE_STREAMS:-3}
SG_SESSION_MAX_DURATION_HOURS=${SG_SESSION_MAX_DURATION_HOURS:-24}

# External services
SG_REALTIME_URL=${SG_REALTIME_URL:-http://localhost:3101}

# App-specific
SG_APP_TV_MAX_CONCURRENT=${SG_APP_TV_MAX_CONCURRENT:-50}
SG_APP_TV_MAX_DEVICE_STREAMS=${SG_APP_TV_MAX_DEVICE_STREAMS:-2}

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
