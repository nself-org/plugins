# recording

Recording orchestration and archive management service.

## Installation

```bash
nself plugin install recording
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `RECORDING_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `true` |
| `RECORDING_PLUGIN_PORT` | `PORT` | Server port | `3602` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `REC_FILE_PROCESSING_URL` | `REC_FILE_PROCESSING_URL` | File processing service URL | `http://media-processing:3019` |
| `REC_DEVICES_URL` | `REC_DEVICES_URL` | Devices service URL | `http://devices:3603` |
| `REC_STORAGE_URL` | `REC_STORAGE_URL` | Storage service URL | `http://localhost:9000` |
| `REC_SPORTS_URL` | `REC_SPORTS_URL` | Sports service URL | `http://sports:3035` |
| `REC_DEFAULT_LEAD_TIME_MINUTES` | `REC_DEFAULT_LEAD_TIME_MINUTES` | Recording lead time (minutes) | `5` |
| `REC_DEFAULT_TRAIL_TIME_MINUTES` | `REC_DEFAULT_TRAIL_TIME_MINUTES` | Recording trail time (minutes) | `5` |
| `REC_AUTO_ENCODE` | `REC_AUTO_ENCODE` | Auto-encode after recording | `true` |
| `REC_AUTO_PUBLISH` | `REC_AUTO_PUBLISH` | Auto-publish recordings | `true` |
| `REC_MAX_CONCURRENT_RECORDINGS` | `REC_MAX_CONCURRENT_RECORDINGS` | Max concurrent recordings | `3` |
| `REC_APP_TV_MAX_CONCURRENT_RECORDINGS` | `REC_APP_TV_MAX_CONCURRENT_RECORDINGS` | nTV max concurrent | `2` |

### Configuration Helper Script

```bash
#!/bin/bash
# generate-recording-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/recording/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$RECORDING_PLUGIN_PORT

# Service URLs
REC_FILE_PROCESSING_URL=${REC_FILE_PROCESSING_URL:-http://localhost:3104}
REC_DEVICES_URL=${REC_DEVICES_URL:-http://localhost:3603}
REC_SPORTS_URL=${REC_SPORTS_URL:-http://localhost:3035}
REC_STORAGE_URL=${REC_STORAGE_URL:-http://localhost:9000}

# Recording settings
REC_DEFAULT_LEAD_TIME_MINUTES=${REC_DEFAULT_LEAD_TIME_MINUTES:-5}
REC_DEFAULT_TRAIL_TIME_MINUTES=${REC_DEFAULT_TRAIL_TIME_MINUTES:-5}
REC_AUTO_ENCODE=${REC_AUTO_ENCODE:-true}
REC_AUTO_PUBLISH=${REC_AUTO_PUBLISH:-true}
REC_MAX_CONCURRENT_RECORDINGS=${REC_MAX_CONCURRENT_RECORDINGS:-3}

# App-specific
REC_APP_TV_MAX_CONCURRENT_RECORDINGS=${REC_APP_TV_MAX_CONCURRENT_RECORDINGS:-2}

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
