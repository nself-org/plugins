# torrent-manager

Torrent downloading with Transmission/qBittorrent integration, multi-source search, seeding policies, and VPN enforcement

## Installation

```bash
nself plugin install torrent-manager
```

## Configuration

See plugin.json for environment variables and configuration options.

## Configuration Mapping

When using nself-tv backend `.env.dev`, map variables as follows:

### Backend → Plugin Variable Mapping

| Backend Variable | Plugin Variable | Description | Example |
|------------------|-----------------|-------------|---------|
| `TORRENT_MANAGER_PLUGIN_ENABLED` | - | Enable plugin (backend only) | `false` |
| `TORRENT_MANAGER_PLUGIN_PORT` | `PORT` | Server port | `3201` |
| `DATABASE_URL` | `DATABASE_URL` | PostgreSQL connection URL | `postgresql://...` |
| `TORRENT_CLIENT` | `TORRENT_CLIENT` | Torrent client type | `transmission` |
| `TORRENT_SEED_RATIO` | `TORRENT_SEED_RATIO` | Target seed ratio | `2.0` |
| `TORRENT_MAX_ACTIVE` | `TORRENT_MAX_ACTIVE` | Max active torrents | `3` |
| `TORRENT_DOWNLOAD_DIR` | `TORRENT_DOWNLOAD_DIR` | Download directory | `/media/torrents/incomplete` |
| `TORRENT_COMPLETE_DIR` | `TORRENT_COMPLETE_DIR` | Completed directory | `/media/torrents/complete` |
| `TORRENT_WATCH_DIR` | `TORRENT_WATCH_DIR` | Watch directory | `/media/torrents/watch` |

### Configuration Helper Script

```bash
#!/bin/bash
# generate-torrent-manager-env.sh

BACKEND_ENV="$HOME/Sites/nself-tv/backend/.env.dev"
PLUGIN_ENV="$HOME/.nself/plugins/torrent-manager/ts/.env"

# Source backend variables
source "$BACKEND_ENV"

# Create plugin .env
cat > "$PLUGIN_ENV" <<EOF
# Auto-generated from backend .env.dev
DATABASE_URL=$DATABASE_URL
PORT=$TORRENT_MANAGER_PLUGIN_PORT

# Torrent client settings
TORRENT_CLIENT=${TORRENT_CLIENT:-transmission}
TORRENT_SEED_RATIO=${TORRENT_SEED_RATIO:-2.0}
TORRENT_MAX_ACTIVE=${TORRENT_MAX_ACTIVE:-3}

# Directories
TORRENT_DOWNLOAD_DIR=${TORRENT_DOWNLOAD_DIR:-/media/torrents/incomplete}
TORRENT_COMPLETE_DIR=${TORRENT_COMPLETE_DIR:-/media/torrents/complete}
TORRENT_WATCH_DIR=${TORRENT_WATCH_DIR:-/media/torrents/watch}

# Bandwidth scheduling (optional)
TORRENT_BANDWIDTH_SCHEDULE_ENABLED=${TORRENT_BANDWIDTH_SCHEDULE_ENABLED:-false}

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
